package recharge

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/redis/go-redis/v9"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testOperations(t *testing.T, s *Service) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	create := func() Order {
		o, e := s.Create(ctx, 1, "5", "alipay")
		if e != nil {
			t.Fatal(e)
		}
		return o
	}
	getBalance := func() string {
		var b string
		if e := s.db.QueryRowContext(ctx, `SELECT balance::text FROM users WHERE id=1`).Scan(&b); e != nil {
			t.Fatal(e)
		}
		return b
	}
	g := gatewayOrder{Code: "1", Money: "5.00", Price: "5.00", Type: "alipay", Status: "1"}
	var requests atomic.Int64
	var alertCalls int
	alertStatus := 500
	s.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "alerts.example" {
			alertCalls++
			if r.Header.Get("Authorization") != "Bearer test-alert-token" {
				t.Error("alert authentication")
			}
			var payload map[string]any
			if e := json.NewDecoder(r.Body).Decode(&payload); e != nil || payload["issue_id"] == nil || payload["generation"] == nil {
				t.Error("alert payload", e)
			}
			return &http.Response{StatusCode: alertStatus, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
		}
		requests.Add(1)
		if r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Error("unsafe query transport")
		}
		r.ParseForm()
		if r.PostForm.Get("key") != s.cfg.Key || r.PostForm.Get("pid") != s.cfg.PID || r.PostForm.Get("act") != "order" {
			t.Error("query credentials")
		}
		v := g
		if v.OutTradeNo == "" {
			v.OutTradeNo = r.PostForm.Get("out_trade_no")
		}
		if v.TradeNo == "" {
			v.TradeNo = scalar("gateway-" + r.PostForm.Get("out_trade_no"))
		}
		b, _ := json.Marshal(v)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
	})
	// Credentials must never be forwarded to a redirected query endpoint.
	originalTransport := s.client.Transport
	var redirected atomic.Int64
	s.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		redirected.Add(1)
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": {"https://other.example/collect"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if _, e := s.queryOrder(ctx, "id"); !errors.Is(e, ErrQuery) || redirected.Load() != 1 {
		t.Fatal("query followed a credential-bearing redirect", e, redirected.Load())
	}
	s.client.Transport = originalTransport
	// A missing callback is recovered from the authenticated query alone.
	if _, e := s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET next_check_at=now()+interval '1 day'`); e != nil {
		t.Fatal(e)
	}
	o := create()
	s.reconcileDue(ctx)
	if getBalance() != "15.00000000" {
		t.Fatal("reconciliation did not credit", getBalance())
	}
	before := requests.Load()
	if e := s.Reconcile(ctx, o.ID, 2, "再次查询"); !errors.Is(e, ErrThrottled) {
		t.Fatal("not throttled", e)
	}
	if requests.Load() != before {
		t.Fatal("throttled request reached gateway")
	}
	// Underpayment in price cannot be hidden by a matching order money field.
	under := create()
	g.Price = "4.99"
	if e := s.Reconcile(ctx, under.ID, 0, ""); !errors.Is(e, ErrPaymentMismatch) {
		t.Fatal("underpaid accepted", e)
	}
	if getBalance() != "15.00000000" {
		t.Fatal("underpaid credited")
	}
	var issueID int64
	var occurrences int
	if e := s.db.QueryRowContext(ctx, `SELECT id,occurrences FROM sub_adds_recharge_issues WHERE order_id=$1 AND kind='payment_mismatch' AND resolved_at IS NULL`, under.ID).Scan(&issueID, &occurrences); e != nil {
		t.Fatal(e)
	}
	if e := s.verifyAndCredit(ctx, under, ""); !errors.Is(e, ErrPaymentMismatch) {
		t.Fatal(e)
	}
	s.db.QueryRowContext(ctx, `SELECT occurrences FROM sub_adds_recharge_issues WHERE id=$1`, issueID).Scan(&occurrences)
	if occurrences != 2 {
		t.Fatal("issues not deduplicated", occurrences)
	}
	// Alert failures are persisted, retried and deduplicated within one open episode.
	s.cfg.AlertWebhook = "https://alerts.example/webhook"
	s.cfg.AlertToken = "test-alert-token"
	s.dispatchAlerts(ctx)
	var notified bool
	s.db.QueryRowContext(ctx, `SELECT notified_at IS NOT NULL FROM sub_adds_recharge_issues WHERE id=$1`, issueID).Scan(&notified)
	if notified || alertCalls != 1 {
		t.Fatal("failed alert acknowledged")
	}
	s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_issues SET notify_after=now() WHERE id=$1`, issueID)
	alertStatus = 204
	s.dispatchAlerts(ctx)
	s.dispatchAlerts(ctx)
	if alertCalls != 2 {
		t.Fatal("alert duplicated", alertCalls)
	}
	s.db.QueryRowContext(ctx, `SELECT notified_at IS NOT NULL FROM sub_adds_recharge_issues WHERE id=$1`, issueID).Scan(&notified)
	if !notified {
		t.Fatal("alert success missing")
	}
	if e := s.ResolveIssue(ctx, issueID, 2, "已联系商户人工核对"); e != nil {
		t.Fatal(e)
	}
	s.recordIssue(ctx, under.ID, "payment_mismatch", "金额仍不一致，异常重新打开")
	s.dispatchAlerts(ctx)
	if alertCalls != 3 {
		t.Fatal("reopened issue not alerted")
	}
	s.cfg.AlertWebhook = ""
	// Overpayments, order amount mismatch, missing price and type mismatch also stop credit.
	for _, tc := range []struct {
		name         string
		money, price scalar
		pt           string
	}{{"overpaid", "5.00", "5.01", "alipay"}, {"order_amount", "4.99", "5.00", "alipay"}, {"missing_price", "5.00", "", "alipay"}, {"payment_type", "5.00", "5.00", "wxpay"}} {
		t.Run(tc.name, func(t *testing.T) {
			g.Money = tc.money
			g.Price = tc.price
			g.Type = tc.pt
			if e := s.verifyAndCredit(ctx, under, ""); !errors.Is(e, ErrPaymentMismatch) {
				t.Fatal("mismatch accepted", e)
			}
		})
	}
	g.Money = "5.00"
	g.Price = "5.00"
	g.Type = "alipay"
	g.OutTradeNo = "other-order"
	if e := s.verifyAndCredit(ctx, under, ""); !errors.Is(e, ErrPaymentFields) {
		t.Fatal("wrong order accepted", e)
	}
	g.OutTradeNo = ""
	g.Status = "0"
	if e := s.verifyAndCredit(ctx, under, ""); !errors.Is(e, ErrUnpaid) {
		t.Fatal("unpaid accepted", e)
	}
	g.Status = "2"
	if e := s.verifyAndCredit(ctx, o, ""); !errors.Is(e, ErrUnpaid) {
		t.Fatal("expiry accepted", e)
	}
	if getBalance() != "15.00000000" {
		t.Fatal("expiry changed credited balance")
	}
	g.Status = "9"
	if e := s.verifyAndCredit(ctx, under, ""); !errors.Is(e, ErrPaymentFields) {
		t.Fatal("unknown status accepted", e)
	}
	g.Status = "1"
	if e := s.verifyAndCredit(ctx, under, "different-trade"); !errors.Is(e, ErrTradeConflict) {
		t.Fatal("trade mismatch accepted", e)
	}
	// A later verified payment resolves the anomaly without bypassing any checks.
	s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET last_checked_at=NULL WHERE id=$1`, under.ID)
	if e := s.Reconcile(ctx, under.ID, 2, "支付平台核对金额已一致，重新补单"); e != nil {
		t.Fatal(e)
	}
	if getBalance() != "20.00000000" {
		t.Fatal("corrected payment not credited")
	}
	var open int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM sub_adds_recharge_issues WHERE order_id=$1 AND resolved_at IS NULL`, under.ID).Scan(&open)
	if open != 0 {
		t.Fatal("corrected issues unresolved", open)
	}
	// A refresh started before another balance change must not acknowledge the newer version.
	s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET cache_synced=false WHERE id=$1`, o.ID)
	hook := &pausePipeline{entered: make(chan struct{}), resume: make(chan struct{})}
	hook.enabled.Store(true)
	s.redis.AddHook(hook)
	syncDone := make(chan error, 1)
	go func() { syncDone <- s.syncOrder(ctx, o.ID) }()
	select {
	case <-hook.entered:
	case <-ctx.Done():
		t.Fatal("cache hook timeout")
	}
	s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET cache_synced=false,cache_version=cache_version+1 WHERE id=$1`, o.ID)
	close(hook.resume)
	if e := <-syncDone; e != nil {
		t.Fatal(e)
	}
	var synced bool
	s.db.QueryRowContext(ctx, `SELECT cache_synced FROM sub_adds_recharge_orders WHERE id=$1`, o.ID).Scan(&synced)
	if synced {
		t.Fatal("stale cache task acknowledged a newer balance change")
	}
	if e := s.syncOrder(ctx, o.ID); e != nil {
		t.Fatal(e)
	}
	// Refund registration debits once and also creates an official negative adjustment record.
	if e := s.RegisterRefund(ctx, o.ID, 2, "REFUND-EXTERNAL-001", "已核对商户后台全额退款凭证"); e != nil {
		t.Fatal(e)
	}
	if e := s.RegisterRefund(ctx, o.ID, 2, "REFUND-EXTERNAL-001", "重复提交同一凭证"); e != nil {
		t.Fatal(e)
	}
	if getBalance() != "15.00000000" {
		t.Fatal("refund double debited", getBalance())
	}
	if e := s.RegisterRefund(ctx, o.ID, 2, "OTHER-REF", "不同凭证"); !errors.Is(e, ErrConflict) {
		t.Fatal("refund conflict accepted", e)
	}
	if e := s.RegisterRefund(ctx, under.ID, 2, "REFUND-EXTERNAL-001", "同一退款凭证不能用于多个订单"); e == nil {
		t.Fatal("reused refund reference accepted")
	}
	if getBalance() != "15.00000000" {
		t.Fatal("refund collision did not roll back")
	}
	// A late callback after reversal cannot restore the refunded credit.
	paid, _ := s.Get(ctx, 1, o.ID)
	v := url.Values{"pid": {s.cfg.PID}, "out_trade_no": {o.ID}, "trade_no": {paid.TradeNo.String}, "money": {"5.00"}, "type": {"alipay"}, "trade_status": {"TRADE_SUCCESS"}}
	v.Set("sign", Sign(v, s.cfg.Key))
	rec := httptest.NewRecorder()
	s.Notify(rec, httptest.NewRequest("GET", "/notify?"+v.Encode(), nil))
	if rec.Body.String() != "success" || getBalance() != "15.00000000" {
		t.Fatal("late callback after refund")
	}
	pending := create()
	if e := s.RegisterRefund(ctx, pending.ID, 2, "REFUND-PENDING", "未支付订单不能退款冲正"); !errors.Is(e, ErrConflict) {
		t.Fatal("unpaid refunded", e)
	}
	s.db.ExecContext(ctx, `UPDATE users SET balance=0 WHERE id=1`)
	if e := s.RegisterRefund(ctx, under.ID, 2, "REFUND-INSUFFICIENT", "可用余额不足测试"); !errors.Is(e, ErrRefundBalance) {
		t.Fatal("spent credit debited", e)
	}
	if getBalance() != "0.00000000" {
		t.Fatal("negative balance introduced")
	}
	var negatives int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM redeem_codes WHERE value<0`).Scan(&negatives)
	if negatives != 1 {
		t.Fatal("refund history rollback", negatives)
	}
	var failedAudit string
	if e := s.db.QueryRowContext(ctx, `SELECT detail FROM sub_adds_recharge_logs WHERE order_id=$1 AND event='refund_requested' ORDER BY id DESC LIMIT 1`, under.ID).Scan(&failedAudit); e != nil {
		t.Fatal(e)
	}
	var recorded map[string]any
	if e := json.Unmarshal([]byte(failedAudit), &recorded); e != nil || recorded["actor_id"] != float64(2) || recorded["reason"] != "可用余额不足测试" {
		t.Fatal("failed refund lost its administrator audit", e)
	}
	// Missing gateway orders remain pending; repeated outages create an issue and a successful read clears it.
	unseen := create()
	g.Code = "-1"
	g.Message = "订单不存在"
	g.Status = "0"
	if e := s.verifyAndCredit(ctx, unseen, ""); !errors.Is(e, ErrGatewayOrderMissing) {
		t.Fatal(e)
	}
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM sub_adds_recharge_issues WHERE order_id=$1 AND resolved_at IS NULL`, unseen.ID).Scan(&open)
	if open != 0 {
		t.Fatal("an uncreated gateway order produced a false payment alarm")
	}
	if e := s.verifyAndCredit(ctx, unseen, "notified-trade"); !errors.Is(e, ErrGatewayOrderMissing) {
		t.Fatal(e)
	}
	g.Message = "商户鉴权失败，secret-never-log"
	for i := 0; i < 3; i++ {
		if e := s.verifyAndCredit(ctx, unseen, ""); !errors.Is(e, ErrQuery) {
			t.Fatal(e)
		}
	}
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM sub_adds_recharge_issues WHERE order_id=$1 AND kind='query_unavailable' AND resolved_at IS NULL`, unseen.ID).Scan(&open)
	if open != 1 {
		t.Fatal("continuous query failure not recorded")
	}
	g.Code = "1"
	g.Message = ""
	g.Status = "0"
	if e := s.verifyAndCredit(ctx, unseen, ""); !errors.Is(e, ErrUnpaid) {
		t.Fatal(e)
	}
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM sub_adds_recharge_issues WHERE order_id=$1 AND resolved_at IS NULL`, unseen.ID).Scan(&open)
	if open != 0 {
		t.Fatal("gateway recovery did not resolve query failures")
	}
	// Access is based on the official role, including disabled/deleted administrators.
	s.db.ExecContext(ctx, `UPDATE users SET role='admin' WHERE id=2`)
	if s.IsAdmin(ctx, 1) || !s.IsAdmin(ctx, 2) {
		t.Fatal("admin roles")
	}
	s.db.ExecContext(ctx, `UPDATE users SET status='disabled' WHERE id=2`)
	if s.IsAdmin(ctx, 2) {
		t.Fatal("disabled admin accepted")
	}
	if _, e := s.AdminOrders(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Issues(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestGatewayQueryFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{{"http_error", `{}`, 502}, {"bad_json", `not-json`, 200}, {"business_error", `{"code":-1,"key":"secret-never-log"}`, 200}, {"oversized", strings.Repeat(" ", 65537), 200}} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{cfg: Config{Gateway: "https://pay.example", PID: "merchant", Key: "secret"}, client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}}
			if _, e := s.queryOrder(context.Background(), "id"); !errors.Is(e, ErrQuery) {
				t.Fatal(e)
			}
		})
	}
}

type pausePipeline struct {
	enabled         atomic.Bool
	entered, resume chan struct{}
}

func (h *pausePipeline) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}
func (h *pausePipeline) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }
func (h *pausePipeline) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if h.enabled.CompareAndSwap(true, false) {
			close(h.entered)
			select {
			case <-h.resume:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return next(ctx, cmds)
	}
}

func TestGatewayNumericPrecision(t *testing.T) {
	s := &Service{cfg: Config{Gateway: "https://pay.example", PID: "merchant", Key: "secret"}, client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":1,"trade_no":5090058146920009728,"out_trade_no":"id","money":5.00,"price":"5.00","status":1,"type":"alipay"}`)), Header: make(http.Header)}, nil
	})}}
	g, e := s.queryOrder(context.Background(), "id")
	if e != nil || g.TradeNo != "5090058146920009728" || g.Money != "5.00" || g.Status != "1" {
		t.Fatal("numeric precision lost", g, e)
	}
}
