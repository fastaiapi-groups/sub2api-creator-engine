package recharge

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testCheckout(t *testing.T, s *Service) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	old := s.client.Transport
	defer func() { s.client.Transport = old }()
	var mapi atomic.Int64
	mode := "cashier"
	var imageBytes bytes.Buffer
	png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 64, 64)))
	encoded := "data:image/jpg;base64," + base64.StdEncoding.EncodeToString(imageBytes.Bytes())
	s.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		response := func(b string) *http.Response {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(b))}
		}
		if r.URL.Path == "/mapi.php" {
			mapi.Add(1)
			r.ParseForm()
			if r.Method != "POST" || !validSignature(r.PostForm, s.cfg.Key) || r.PostForm.Get("money") != "5.00" {
				t.Error("invalid signed mapi request")
			}
			if mode == "timeout" {
				return nil, io.ErrUnexpectedEOF
			}
			reply := map[string]any{"code": 1, "trade_no": "QR-" + r.PostForm.Get("out_trade_no"), "money": "5.00", "price": "5.00", "payurl": "https://pay.example/gateway/Submit/pay?trade_no=provider"}
			if mode == "native" {
				reply["qrcode"] = "wxp://example-native-code"
			}
			if mode == "mismatch" {
				reply["price"] = "4.99"
			}
			if mode == "external" {
				reply["payurl"] = "http://127.0.0.1/private"
			}
			b, _ := json.Marshal(reply)
			return response(string(b)), nil
		}
		if r.URL.Path == "/api.php" {
			r.ParseForm()
			b, _ := json.Marshal(map[string]any{"code": 1, "trade_no": "QR-" + r.PostForm.Get("out_trade_no"), "out_trade_no": r.PostForm.Get("out_trade_no"), "money": "5.00", "price": "5.00", "status": 0, "type": "alipay"})
			return response(string(b)), nil
		}
		if r.URL.Host != "pay.example" {
			t.Error("attempted external asset fetch", r.URL.Host)
		}
		return response(`<html><script>location.href='https://evil.example'</script><img src="/logo.png"><div id="qrarea"><img class="qr" src="` + encoded + `"></div></html>`), nil
	})
	var createdIDs []string
	defer func() {
		for _, id := range createdIDs {
			s.db.Exec(`DELETE FROM sub_adds_recharge_issues WHERE order_id=$1`, id)
			s.db.Exec(`DELETE FROM sub_adds_recharge_logs WHERE order_id=$1`, id)
			s.db.Exec(`DELETE FROM sub_adds_recharge_orders WHERE id=$1`, id)
		}
	}()
	create := func(key string) Order {
		o, e := s.CreateWithKey(ctx, 1, "5", "alipay", key)
		if e != nil {
			t.Fatal(e)
		}
		createdIDs = append(createdIDs, o.ID)
		return o
	}
	o := create("same-request-key-123456")
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x, e := s.CreateWithKey(ctx, 1, "5", "alipay", "same-request-key-123456")
			if e != nil {
				t.Error(e)
				return
			}
			ids <- x.ID
		}()
	}
	wg.Wait()
	close(ids)
	for id := range ids {
		if id != o.ID {
			t.Fatal("request id did not deduplicate local orders")
		}
	}
	if _, e := s.CreateWithKey(ctx, 1, "5", "wxpay", "same-request-key-123456"); !errors.Is(e, ErrConflict) {
		t.Fatal("request id reused across payment methods")
	}
	var count int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM sub_adds_recharge_orders WHERE request_key='same-request-key-123456'`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate local orders", count)
	}
	// Only one instance may execute the upstream non-idempotent creation request.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.Checkout(ctx, 1, o.ID)
			if e != nil && !errors.Is(e, ErrCheckoutPending) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if mapi.Load() != 1 {
		t.Fatal("duplicate upstream orders", mapi.Load())
	}
	c, e := s.Payment(ctx, 1, o.ID)
	if e != nil || c.State != "ready" || c.QRKind != "image" || !strings.HasPrefix(c.QRCode, "data:image/png;base64,") {
		t.Fatal("cashier image not extracted", c.State, c.QRKind, e)
	}
	if strings.Contains(c.QRCode, "script") || strings.Contains(c.QRCode, "pay.example") {
		t.Fatal("cashier page leaked to client")
	}
	if _, e = s.Checkout(ctx, 2, o.ID); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("another user's QR accessible", e)
	}
	if mapi.Load() != 1 {
		t.Fatal("unauthorized request called gateway")
	}
	bad := s.paymentParams(o)
	bad.Set("trade_no", "different-trade")
	bad.Set("trade_status", "TRADE_SUCCESS")
	if e = s.credit(ctx, bad); !errors.Is(e, ErrTradeConflict) {
		t.Fatal("pending provider trade number not enforced", e)
	}
	mode = "native"
	native := create("")
	c, e = s.Checkout(ctx, 1, native.ID)
	if e != nil || c.QRKind != "text" || c.QRCode != "wxp://example-native-code" {
		t.Fatal("native QR", e, c.QRKind)
	}
	mode = "mismatch"
	wrong := create("")
	if _, e = s.Checkout(ctx, 1, wrong.ID); !errors.Is(e, ErrPaymentMismatch) {
		t.Fatal("provider amount mismatch accepted", e)
	}
	n := mapi.Load()
	if _, e = s.Checkout(ctx, 1, wrong.ID); !errors.Is(e, ErrPaymentMismatch) || mapi.Load() != n {
		t.Fatal("amount block bypassed on retry", e)
	}
	mode = "external"
	external := create("")
	if _, e = s.Checkout(ctx, 1, external.ID); !errors.Is(e, ErrQRCodeUnavailable) {
		t.Fatal("external cashier accepted", e)
	}
	mode = "timeout"
	unknown := create("")
	if _, e = s.Checkout(ctx, 1, unknown.ID); !errors.Is(e, ErrCheckoutPending) {
		t.Fatal(e)
	}
	n = mapi.Load()
	s.Checkout(ctx, 1, unknown.ID)
	if mapi.Load() != n {
		t.Fatal("timeout retried mapi")
	}
	s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET payment_requested_at=now()-interval '20 seconds' WHERE id=$1`, unknown.ID)
	c, e = s.Checkout(ctx, 1, unknown.ID)
	if e != nil || c.State != "ready" || mapi.Load() != n {
		t.Fatal("same-order recovery failed", c.State, e)
	}
	// Legacy redirect orders are recovered by query/GET only as well.
	legacy := create("")
	s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET payment_state='legacy' WHERE id=$1`, legacy.ID)
	if c, e = s.Checkout(ctx, 1, legacy.ID); e != nil || c.State != "ready" || mapi.Load() != n {
		t.Fatal("legacy order was re-created", e)
	}
}
func TestUnsafeQRCodeSources(t *testing.T) {
	s := &Service{cfg: Config{Gateway: "https://pay.example"}, client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unsafe URL fetched"); return nil, io.EOF })}}
	for _, raw := range []string{"javascript:alert(1)", "file:///etc/passwd", "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=", "http://127.0.0.1/a", "https://other.example/private"} {
		if _, _, e := s.resolveQRCode(context.Background(), raw, "cashier"); e == nil {
			t.Fatal("unsafe source accepted", raw)
		}
	}
}
