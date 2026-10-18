package recharge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func testActualSettlement(t *testing.T, s *Service) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	oldTransport, oldPackages := s.client.Transport, s.cfg.Packages
	defer func() { s.client.Transport = oldTransport; s.cfg.Packages = oldPackages }()
	s.cfg.Packages = append(s.cfg.Packages, Package{ID: "one", Name: "1 USD", AmountCents: 100, Credit: "1"}, Package{ID: "rate", Name: "custom", AmountCents: 700, Credit: "10"})
	if _, e := s.db.Exec(`INSERT INTO users(id) VALUES(77),(78)`); e != nil {
		t.Fatal(e)
	}
	defer func() {
		s.db.Exec(`DELETE FROM sub_adds_recharge_issues WHERE order_id IN (SELECT id FROM sub_adds_recharge_orders WHERE user_id IN (77,78))`)
		s.db.Exec(`DELETE FROM sub_adds_recharge_logs WHERE order_id IN (SELECT id FROM sub_adds_recharge_orders WHERE user_id IN (77,78))`)
		s.db.Exec(`DELETE FROM redeem_codes WHERE used_by IN (77,78)`)
		s.db.Exec(`DELETE FROM sub_adds_recharge_orders WHERE user_id IN (77,78)`)
		s.db.Exec(`DELETE FROM users WHERE id IN (77,78)`)
	}()
	price, status, tradeOverride, moneyOverride := "0.01", "1", "", ""
	s.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.ParseForm()
		id := r.PostForm.Get("out_trade_no")
		nominal := "1.00"
		if strings.Contains(r.URL.Path, "unexpected") {
			t.Error("unexpected endpoint")
		}
		var amount int64
		s.db.QueryRow(`SELECT amount_cents FROM sub_adds_recharge_orders WHERE id=$1`, id).Scan(&amount)
		nominal = money(amount)
		if moneyOverride != "" {
			nominal = moneyOverride
		}
		trade := "verified-" + id
		if tradeOverride != "" {
			trade = tradeOverride
		}
		b, _ := json.Marshal(map[string]any{"code": 1, "out_trade_no": id, "trade_no": trade, "type": "alipay", "money": nominal, "price": price, "status": status})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
	})
	create := func(uid int64, pkg string) Order {
		o, e := s.Create(ctx, uid, pkg, "alipay")
		if e != nil {
			t.Fatal(e)
		}
		return o
	}
	notice := func(o Order) url.Values {
		v := url.Values{"pid": {s.cfg.PID}, "out_trade_no": {o.ID}, "trade_no": {"verified-" + o.ID}, "money": {money(o.AmountCents)}, "type": {"alipay"}, "trade_status": {"TRADE_SUCCESS"}, "sign_type": {"MD5"}}
		v.Set("sign", Sign(v, s.cfg.Key))
		return v
	}
	// 1 yuan order with authenticated 0.01 yuan receipt must credit 0.01 only.
	o := create(77, "one")
	v := notice(o)
	send := func(v url.Values) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/notify", strings.NewReader(v.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		s.Notify(w, r)
		return w
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := send(v)
			if r.Code != 200 || r.Body.String() != "success" {
				t.Error("valid repeated callback failed", r.Code)
			}
		}()
	}
	wg.Wait()
	var balance, actual string
	var receipt int64
	var count int
	s.db.QueryRow(`SELECT balance::text FROM users WHERE id=77`).Scan(&balance)
	s.db.QueryRow(`SELECT paid_amount_cents,credited_credit::text FROM sub_adds_recharge_orders WHERE id=$1`, o.ID).Scan(&receipt, &actual)
	s.db.QueryRow(`SELECT count(*) FROM sub_adds_recharge_logs WHERE order_id=$1 AND event='credited'`, o.ID).Scan(&count)
	if balance != "0.01000000" || actual != "0.01000000" || receipt != 1 || count != 1 {
		t.Fatal("partial settlement duplicated or overcredited", balance, actual, receipt, count)
	}
	// Replaying a captured successful callback can never transfer it to another user/order.
	other := create(78, "one")
	forged := notice(o)
	forged.Set("out_trade_no", other.ID)
	if send(forged).Code != 400 {
		t.Fatal("modified callback signature accepted")
	}
	forged = notice(other)
	forged.Del("sign")
	if send(forged).Code != 400 {
		t.Fatal("unsigned success accepted")
	}
	forged = notice(other)
	forged.Set("money", "999999.00")
	if send(forged).Code != 400 {
		t.Fatal("altered amount accepted")
	}
	// Even an authenticated provider response reusing a transaction rolls back atomically.
	tradeOverride = "verified-" + o.ID
	if e := s.verifyAndCredit(ctx, other, tradeOverride); e == nil {
		t.Fatal("reused provider transaction accepted")
	}
	s.db.QueryRow(`SELECT balance::text FROM users WHERE id=78`).Scan(&balance)
	if balance != "0.00000000" {
		t.Fatal("cross-user replay credited", balance)
	}
	tradeOverride = ""
	// A signed success claim cannot credit an unpaid gateway order.
	status = "0"
	unpaid := create(78, "one")
	if send(notice(unpaid)).Code != 503 {
		t.Fatal("client success overrode unpaid gateway")
	}
	s.db.QueryRow(`SELECT balance::text FROM users WHERE id=78`).Scan(&balance)
	if balance != "0.00000000" {
		t.Fatal("unpaid order credited")
	}
	status = "1"
	// Immutable settlement: repeated queries with a changed price never add another credit.
	price = "100.00"
	if e := s.verifyAndCredit(ctx, o, ""); e == nil {
		t.Fatal("changed settled amount accepted")
	}
	s.db.QueryRow(`SELECT balance::text FROM users WHERE id=77`).Scan(&balance)
	if balance != "0.01000000" {
		t.Fatal("settlement changed balance")
	}
	// Full refund deducts only the actual 0.01 credit, not the quoted 1 credit.
	price = "0.01"
	if e := s.RegisterRefund(ctx, o.ID, 2, "PARTIAL-REFUND-TEST", "已核对外部实付全额退款"); e != nil {
		t.Fatal(e)
	}
	s.db.QueryRow(`SELECT balance::text FROM users WHERE id=77`).Scan(&balance)
	if balance != "0.00000000" {
		t.Fatal("partial refund deducted quote", balance)
	}
	if r := send(v); r.Code != 200 {
		t.Fatal("refunded duplicate notification not acknowledged")
	}
	s.db.QueryRow(`SELECT balance::text FROM users WHERE id=77`).Scan(&balance)
	if balance != "0.00000000" {
		t.Fatal("refunded callback credited again")
	}
	// Some channel adapters echo the paid value in both amount fields.
	moneyOverride = "0.01"
	small := create(78, "one")
	if e := s.verifyAndCredit(ctx, small, ""); e != nil {
		t.Fatal("valid actual money echo rejected", e)
	}
	s.db.QueryRow(`SELECT balance::text FROM users WHERE id=78`).Scan(&balance)
	if balance != "0.01000000" {
		t.Fatal("actual amount echo overcredited", balance)
	}
	moneyOverride = ""
	for _, tc := range []struct {
		pkg, price, status, want string
		ok                       bool
	}{{"one", "1.25", "1", "1.25000000", true}, {"rate", "0.01", "1", "0.01428571", true}, {"one", "0.01", "0", "", false}, {"one", "0", "1", "", false}, {"one", "", "1", "", false}, {"one", "-1", "1", "", false}} {
		price, status = tc.price, tc.status
		x := create(77, tc.pkg)
		e := s.verifyAndCredit(ctx, x, "")
		if (e == nil) != tc.ok {
			t.Fatal("settlement validation", tc, e)
		}
		var state string
		s.db.QueryRow(`SELECT status FROM sub_adds_recharge_orders WHERE id=$1`, x.ID).Scan(&state)
		if tc.ok {
			s.db.QueryRow(`SELECT credited_credit::text FROM sub_adds_recharge_orders WHERE id=$1`, x.ID).Scan(&actual)
			if actual != tc.want || state != "paid" {
				t.Fatal("incorrect actual credit", tc, actual, state)
			}
		} else if state != "pending" {
			t.Fatal("unconfirmed settlement credited", tc)
		}
	}
}
