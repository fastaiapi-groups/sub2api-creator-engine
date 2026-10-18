package recharge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var (
	ErrPaymentFields       = errors.New("invalid payment fields")
	ErrPaymentMismatch     = errors.New("payment amount/type mismatch")
	ErrTradeConflict       = errors.New("trade number mismatch")
	ErrGatewayOrderMissing = errors.New("gateway order not found")
	ErrUnpaid              = errors.New("gateway payment not confirmed")
	ErrQuery               = errors.New("gateway query unavailable")
	ErrThrottled           = errors.New("查询过于频繁，请稍后重试")
	ErrRefundBalance       = errors.New("用户可用余额不足，无法冲正，请先人工处理已消费额度")
	ErrConflict            = errors.New("订单状态或凭证冲突")
)

// scalar accepts both number and string fields without floating-point conversion.
type scalar string

func (s *scalar) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = scalar(v)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*s = scalar(n.String())
	return nil
}

type gatewayOrder struct {
	Message    string `json:"msg"`
	Code       scalar `json:"code"`
	TradeNo    scalar `json:"trade_no"`
	OutTradeNo string `json:"out_trade_no"`
	Type       string `json:"type"`
	Money      scalar `json:"money"`
	Price      scalar `json:"price"`
	Status     scalar `json:"status"`
}

func (s *Service) queryOrder(ctx context.Context, id string) (gatewayOrder, error) {
	form := url.Values{"act": {"order"}, "pid": {s.cfg.PID}, "key": {s.cfg.Key}, "out_trade_no": {id}}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.cfg.Gateway, "/")+"/api.php", strings.NewReader(form.Encode()))
	if err != nil {
		return gatewayOrder{}, ErrQuery
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return gatewayOrder{}, ErrQuery
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return gatewayOrder{}, ErrQuery
	}
	var o gatewayOrder
	// Do not retain, return or log the raw gateway response: it may contain merchant secrets.
	data, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return o, ErrQuery
	}
	if err = json.Unmarshal(data, &o); err != nil {
		return o, ErrQuery
	}
	if o.Code != "1" {
		if strings.Contains(o.Message, "订单不存在") || strings.Contains(o.Message, "订单号不存在") {
			return o, ErrGatewayOrderMissing
		}
		return o, ErrQuery
	}
	if o.OutTradeNo != id || o.TradeNo == "" {
		return o, ErrPaymentFields
	}
	return o, nil
}
func (s *Service) validateNotification(o Order, v url.Values) error {
	n, e := parseMoney(v.Get("money"))
	if e != nil {
		return ErrPaymentMismatch
	}
	if v.Get("pid") != s.cfg.PID || v.Get("trade_status") != "TRADE_SUCCESS" || v.Get("trade_no") == "" || len(v.Get("trade_no")) > 128 {
		return ErrPaymentFields
	}
	if n != o.AmountCents || v.Get("type") != o.PaymentType {
		return ErrPaymentMismatch
	}
	if o.Status == "paid" && o.TradeNo.String != v.Get("trade_no") {
		return ErrTradeConflict
	}
	return nil
}
func (s *Service) verifyAndCredit(ctx context.Context, o Order, expectedTrade string) error {
	g, err := s.queryOrder(ctx, o.ID)
	if err != nil {
		if errors.Is(err, ErrGatewayOrderMissing) {
			_, e := s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET gateway_status='not_found',query_failures=0 WHERE id=$1`, o.ID)
			if e != nil {
				return e
			}
			if expectedTrade != "" || o.Status == "paid" {
				s.recordIssue(ctx, o.ID, "query_missing", "已经收到支付通知或本地已经入账，但平台暂时无法查到订单，请核对商户和平台流水")
			}
			return err
		}
		if errors.Is(err, ErrPaymentFields) {
			s.recordIssue(ctx, o.ID, "query_mismatch", "支付平台返回的订单号或流水无效")
		} else {
			s.queryFailed(ctx, o.ID)
		}
		return err
	}
	// Status 2 is expiry, NOT a refund. Never infer a refund from an unpaid response.
	status := map[scalar]string{"0": "pending", "1": "paid", "2": "expired"}[g.Status]
	if status == "" {
		s.recordIssue(ctx, o.ID, "unknown_status", "支付平台返回未知状态，需要人工核对")
		return ErrPaymentFields
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET gateway_status=$2,query_failures=0 WHERE id=$1`, o.ID, status); err != nil {
		return err
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_issues SET resolved_at=now(),resolution='平台查单已恢复，订单信息可正常读取' WHERE order_id=$1 AND resolved_at IS NULL AND kind IN ('query_unavailable','query_mismatch','query_missing','unknown_status')`, o.ID); err != nil {
		return err
	}
	if status != "paid" {
		if o.Status == "paid" && o.ReversedAt == nil {
			s.recordIssue(ctx, o.ID, "payment_state_changed", "本地已经入账，但支付平台返回未支付或超时；请人工核对，系统不会自动扣款")
		}
		return ErrUnpaid
	}
	n, e := parseMoney(string(g.Money))
	price, pe := parseMoney(string(g.Price))
	if e != nil || pe != nil || n != o.AmountCents || price != o.AmountCents || g.Type != o.PaymentType {
		s.recordIssue(ctx, o.ID, "payment_mismatch", fmt.Sprintf("应付 %s 元，平台订单金额 %s 元，支付金额 %s 元；金额或支付方式不一致，暂停入账", money(o.AmountCents), safeMoney(g.Money), safeMoney(g.Price)))
		return ErrPaymentMismatch
	}
	if (expectedTrade != "" && string(g.TradeNo) != expectedTrade) || (o.TradeNo.Valid && string(g.TradeNo) != o.TradeNo.String) || len(g.TradeNo) > 128 {
		s.recordIssue(ctx, o.ID, "trade_conflict", "支付通知、本地订单与平台流水不一致，暂停入账")
		return ErrTradeConflict
	}
	v := url.Values{"pid": {s.cfg.PID}, "out_trade_no": {o.ID}, "trade_no": {string(g.TradeNo)}, "money": {string(g.Money)}, "type": {g.Type}, "trade_status": {"TRADE_SUCCESS"}}
	if err = s.credit(ctx, v); err != nil {
		s.recordIssue(ctx, o.ID, "credit_failed", "平台确认收款，但本地入账失败；请检查数据库、用户状态或流水唯一性，后台将继续重试")
		return err
	}
	if err = s.syncOrder(ctx, o.ID); err != nil {
		s.recordIssue(ctx, o.ID, "cache_sync", "余额已入账，缓存刷新失败，后台将继续重试")
	}
	return s.resolvePaymentIssues(ctx, o.ID)
}
func safeMoney(s scalar) string {
	n, e := parseMoney(string(s))
	if e != nil {
		return "无效"
	}
	return money(n)
}
func (s *Service) resolvePaymentIssues(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_issues SET resolved_at=now(),resolution='平台查单核对一致，已确认入账' WHERE order_id=$1 AND resolved_at IS NULL AND kind IN ('payment_mismatch','callback_mismatch','query_mismatch','query_missing','query_unavailable','trade_conflict','credit_failed','unknown_status','payment_state_changed')`, id)
	return err
}
func (s *Service) queryFailed(ctx context.Context, id string) {
	var n int
	if err := s.db.QueryRowContext(ctx, `UPDATE sub_adds_recharge_orders SET query_failures=query_failures+1 WHERE id=$1 RETURNING query_failures`, id).Scan(&n); err == nil && n >= 3 {
		s.recordIssue(ctx, id, "query_unavailable", "支付平台查单连续失败至少三次；请检查网络、商户配置或平台订单是否存在，后台将继续重试")
	}
}
