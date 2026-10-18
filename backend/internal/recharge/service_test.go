package recharge

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

// Uses a disposable database only: never point RECHARGE_TEST_DATABASE_URL at production.
func TestPaymentIntegration(t *testing.T) {
	dsn := os.Getenv("RECHARGE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("temporary PostgreSQL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`CREATE TABLE users(id bigint PRIMARY KEY,balance numeric(20,8) NOT NULL DEFAULT 0,updated_at timestamptz,status text DEFAULT 'active',role text DEFAULT 'user',deleted_at timestamptz);CREATE TABLE api_keys(key text,user_id bigint,deleted_at timestamptz);CREATE TABLE redeem_codes(code varchar(32) UNIQUE,type text,value numeric(20,8),status text,used_by bigint,used_at timestamptz,created_at timestamptz);INSERT INTO users(id) VALUES(1),(2);INSERT INTO api_keys VALUES('test-key',1,NULL)`)
	if e != nil {
		t.Fatal(e)
	}
	c := Config{Gateway: "https://pay.example", PublicURL: "https://app.example", PID: "merchant", Key: "secret", DatabaseURL: dsn, RedisURL: os.Getenv("RECHARGE_TEST_REDIS_URL"), Packages: []Package{{"5", "5 USD", 500, "5"}}}
	s, e := New(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.ParseForm()
		payload := gatewayOrder{Code: "1", OutTradeNo: r.Form.Get("out_trade_no"), TradeNo: "trade3", Money: "5.00", Price: "5.00", Type: "alipay", Status: "1"}
		b, _ := json.Marshal(payload)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
	})
	create := func() Order {
		o, e := s.Create(ctx, 1, "5", "alipay")
		if e != nil {
			t.Fatal(e)
		}
		return o
	}
	callback := func(o Order, trade string) url.Values {
		return url.Values{"pid": {"merchant"}, "out_trade_no": {o.ID}, "money": {"5.00"}, "type": {"alipay"}, "trade_no": {trade}, "trade_status": {"TRADE_SUCCESS"}}
	}
	balance := func() string {
		var b string
		if e := db.QueryRow(`SELECT balance::text FROM users WHERE id=1`).Scan(&b); e != nil {
			t.Fatal(e)
		}
		return b
	}
	o := create()
	v := callback(o, "trade1")
	if _, e = s.Get(ctx, 2, o.ID); e != sql.ErrNoRows {
		t.Fatal("order ownership broken", e)
	}
	if !validSignature(s.paymentParams(o), c.Key) {
		t.Fatal("payment URL signature")
	}
	bad := callback(o, "trade1")
	bad.Set("money", "0.01")
	if e = s.credit(ctx, bad); e == nil {
		t.Fatal("wrong amount accepted")
	}
	bad = callback(o, "trade1")
	bad.Set("pid", "other")
	if e = s.credit(ctx, bad); e == nil {
		t.Fatal("wrong merchant accepted")
	}
	bad = callback(o, "trade1")
	bad.Set("type", "wxpay")
	if e = s.credit(ctx, bad); e == nil {
		t.Fatal("wrong payment type accepted")
	}
	rec := httptest.NewRecorder()
	s.Notify(rec, httptest.NewRequest("GET", "/notify?"+v.Encode(), nil))
	if rec.Code != 400 || balance() != "0.00000000" {
		t.Fatal("unsigned callback accepted")
	}
	// Concurrent duplicate deliveries must credit once.
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.credit(ctx, v) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if balance() != "5.00000000" {
		t.Fatal("duplicate credits", balance())
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM sub_adds_recharge_logs WHERE event='credited'`).Scan(&n)
	if n != 1 {
		t.Fatal("ledger count", n)
	}
	// Reused gateway trade number rolls back both balance and official history.
	o2 := create()
	if e = s.credit(ctx, callback(o2, "trade1")); e == nil {
		t.Fatal("reused trade accepted")
	}
	if balance() != "5.00000000" {
		t.Fatal("transaction did not roll back")
	}
	db.QueryRow(`SELECT count(*) FROM redeem_codes`).Scan(&n)
	if n != 1 {
		t.Fatal("history did not roll back")
	}
	// Refresh official Redis cache and L1 invalidation channel.
	h := "62af8704764faf8ea82fc61ce9c4c3908b6cb97d463a634e9e587d7c885db0ef"
	s.redis.Set(ctx, "billing:balance:1", "0", 0)
	s.redis.Set(ctx, "apikey:auth:"+h, "stale", 0)
	pub := s.redis.Subscribe(ctx, "auth:cache:invalidate")
	defer pub.Close()
	if _, e = pub.Receive(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.syncOrder(ctx, o.ID); e != nil {
		t.Fatal(e)
	}
	if n, e := s.redis.Exists(ctx, "billing:balance:1", "apikey:auth:"+h).Result(); e != nil || n != 0 {
		t.Fatal("stale cache", n, e)
	}
	msg, e := pub.ReceiveMessage(ctx)
	if e != nil || msg.Payload != h {
		t.Fatal("L1 invalidate", msg, e)
	}
	v.Set("sign", Sign(v, c.Key))
	rec = httptest.NewRecorder()
	s.Notify(rec, httptest.NewRequest("POST", "/notify", strings.NewReader(v.Encode())))
	if rec.Code != 400 {
		t.Fatal("POST without form content type accepted")
	}
	// POST requires form content type.
	req := httptest.NewRequest("POST", "/notify", strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Notify(rec, req)
	if rec.Code != 200 || rec.Body.String() != "success" {
		t.Fatal("POST callback", rec.Code, rec.Body.String())
	}
	// Redis failure is durably retried without crediting again.
	o3 := create()
	v3 := callback(o3, "trade3")
	v3.Set("sign", Sign(v3, c.Key))
	rec = httptest.NewRecorder()
	s.Notify(rec, httptest.NewRequest("GET", "/notify?"+v3.Encode(), nil))
	if rec.Code != 200 || rec.Body.String() != "success" {
		t.Fatal("GET callback", rec.Code, rec.Body.String())
	}
	// Mark as pending to simulate a process stopping before Redis refresh.
	if _, e = db.Exec(`UPDATE sub_adds_recharge_orders SET cache_synced=false WHERE id=$1`, o3.ID); e != nil {
		t.Fatal(e)
	}
	good := s.redis
	s.redis = redisUnavailable()
	if e = s.syncOrder(ctx, o3.ID); e == nil {
		t.Fatal("expected Redis failure")
	}
	s.redis.Close()
	s.redis = good
	s.syncPending(ctx)
	var synced bool
	db.QueryRow(`SELECT cache_synced FROM sub_adds_recharge_orders WHERE id=$1`, o3.ID).Scan(&synced)
	if !synced || balance() != "10.00000000" {
		t.Fatal("retry failed", synced, balance())
	}
	t.Log("concurrent callbacks, ownership, atomic rollback, cache invalidation and recovery verified")
	t.Run("checkout", func(t *testing.T) { testCheckout(t, s) })
	t.Run("operations", func(t *testing.T) { testOperations(t, s) })
}

func redisUnavailable() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
