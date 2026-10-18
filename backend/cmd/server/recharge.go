package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sub-adds/backend/internal/recharge"
	"time"
)

func (a *app) rechargeRoute(w http.ResponseWriter, r *http.Request) {
	if a.recharge == nil {
		failText(w, 503, "充值服务尚未配置")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/recharge/")
	if p == "notify" {
		a.recharge.Notify(w, r)
		return
	}
	if p == "packages" && r.Method == "GET" {
		success(w, a.recharge.Packages())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	uid, ok := a.userID(w, r)
	if !ok {
		return
	}
	timeout := 15 * time.Second
	if strings.HasSuffix(p, "/checkout") {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	result := func(v any, err error) {
		if err != nil {
			log.Printf("recharge request: %v", err)
			if errors.Is(err, recharge.ErrPendingLimit) {
				failText(w, 429, err.Error())
			} else if errors.Is(err, recharge.ErrThrottled) {
				failText(w, 429, err.Error())
			} else if errors.Is(err, recharge.ErrRefundBalance) || errors.Is(err, recharge.ErrConflict) {
				failText(w, 409, err.Error())
			} else if errors.Is(err, recharge.ErrQRCodeUnavailable) {
				failText(w, 502, err.Error())
			} else if errors.Is(err, recharge.ErrGatewayOrderMissing) {
				failText(w, 502, "支付平台暂未查到该订单，未增加余额，请稍后重试")
			} else if errors.Is(err, recharge.ErrQuery) {
				failText(w, 502, "支付平台暂时无法查单，后台将继续重试")
			} else if errors.Is(err, recharge.ErrPaymentMismatch) || errors.Is(err, recharge.ErrTradeConflict) || errors.Is(err, recharge.ErrPaymentFields) {
				failText(w, 409, "支付信息存在差异，已转入异常订单，请联系管理员核对")
			} else if errors.Is(err, sql.ErrNoRows) {
				failText(w, 404, "订单不存在")
			} else {
				failText(w, 500, "充值服务暂时不可用，请稍后重试")
			}
			return
		}
		success(w, v)
	}
	if strings.HasPrefix(p, "admin/") {
		if !a.recharge.IsAdmin(ctx, uid) {
			failText(w, 403, "需要管理员权限")
			return
		}
		a.rechargeAdmin(w, r, ctx, uid, strings.TrimPrefix(p, "admin/"), result)
		return
	}
	switch {
	case p == "access" && r.Method == "GET":
		success(w, map[string]bool{"is_admin": a.recharge.IsAdmin(ctx, uid)})
	case p == "orders" && r.Method == "POST":
		var in struct {
			PackageID   string `json:"package_id"`
			PaymentType string `json:"payment_type"`
			RequestID   string `json:"request_id"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			failText(w, 400, "请求格式错误")
			return
		}
		valid := false
		for _, x := range a.recharge.Packages() {
			if x.ID == in.PackageID {
				valid = true
			}
		}
		if !valid || (in.PaymentType != "alipay" && in.PaymentType != "wxpay") {
			failText(w, 400, "套餐或支付方式无效")
			return
		}
		o, err := a.recharge.CreateWithKey(ctx, uid, in.PackageID, in.PaymentType, in.RequestID)
		if err != nil {
			result(nil, err)
			return
		}
		success(w, map[string]any{"order": o})
	case p == "orders" && r.Method == "GET":
		v, e := a.recharge.List(ctx, uid)
		result(v, e)
	case p == "logs" && r.Method == "GET":
		v, e := a.recharge.Logs(ctx, uid)
		result(v, e)
	case strings.HasPrefix(p, "orders/") && strings.HasSuffix(p, "/checkout") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(p, "orders/"), "/checkout")
		c, e := a.recharge.Checkout(ctx, uid, id)
		if errors.Is(e, recharge.ErrCheckoutPending) {
			current, readErr := a.recharge.Payment(ctx, uid, id)
			if readErr != nil {
				result(nil, readErr)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			success(w, current)
			return
		}
		result(c, e)
	case strings.HasPrefix(p, "orders/") && strings.HasSuffix(p, "/recheck") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(p, "orders/"), "/recheck")
		o, e := a.recharge.Get(ctx, uid, id)
		if e != nil {
			result(nil, e)
			return
		}
		if o.ReversedAt != nil {
			failText(w, 409, "订单已登记退款冲正")
			return
		}
		e = a.recharge.Reconcile(ctx, id, uid, "用户申请查询付款结果")
		if e != nil && !errors.Is(e, recharge.ErrUnpaid) {
			result(nil, e)
			return
		}
		o, e = a.recharge.Get(ctx, uid, id)
		result(o, e)
	case strings.HasPrefix(p, "orders/") && r.Method == "GET":
		c, e := a.recharge.Payment(ctx, uid, strings.TrimPrefix(p, "orders/"))
		result(c, e)
	default:
		method(w)
	}
}

func (a *app) rechargeAdmin(w http.ResponseWriter, r *http.Request, ctx context.Context, uid int64, p string, result func(any, error)) {
	if r.Method == "GET" {
		switch p {
		case "orders":
			v, e := a.recharge.AdminOrders(ctx)
			result(v, e)
		case "issues":
			v, e := a.recharge.Issues(ctx)
			result(v, e)
		default:
			http.NotFound(w, r)
		}
		return
	}
	if r.Method != "POST" {
		method(w)
		return
	}
	var in struct {
		Reason    string `json:"reason"`
		Reference string `json:"reference"`
		Confirmed bool   `json:"confirmed"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(strings.TrimSpace(in.Reason)) < 3 || len(in.Reason) > 1000 {
		failText(w, 400, "请填写有效的操作原因（3—1000 字节）")
		return
	}
	parts := strings.Split(p, "/")
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	switch {
	case parts[0] == "orders" && parts[2] == "recheck":
		e := a.recharge.Reconcile(ctx, parts[1], uid, in.Reason)
		if errors.Is(e, recharge.ErrUnpaid) {
			success(w, map[string]string{"message": "平台尚未确认支付，未增加余额"})
			return
		}
		result(map[string]bool{"checked": e == nil}, e)
	case parts[0] == "orders" && parts[2] == "refund":
		if !in.Confirmed {
			failText(w, 400, "必须确认已在支付平台完成全额退款")
			return
		}
		e := a.recharge.RegisterRefund(ctx, parts[1], uid, strings.TrimSpace(in.Reference), in.Reason)
		result(map[string]bool{"registered": e == nil}, e)
	case parts[0] == "issues" && parts[2] == "resolve":
		id, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id <= 0 {
			failText(w, 400, "异常编号无效")
			return
		}
		e = a.recharge.ResolveIssue(ctx, id, uid, in.Reason)
		result(map[string]bool{"resolved": e == nil}, e)
	default:
		http.NotFound(w, r)
	}
}
