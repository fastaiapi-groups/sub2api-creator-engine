package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"sub-adds/backend/internal/recharge"
)

func TestRechargeAuthorization(t *testing.T) {
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
	if _, e = db.ExecContext(ctx, `CREATE SCHEMA recharge_routes_test`); e != nil {
		t.Fatal(e)
	}
	defer db.Exec(`DROP SCHEMA recharge_routes_test CASCADE`)
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", "recharge_routes_test")
	u.RawQuery = q.Encode()
	scoped, e := sql.Open("pgx", u.String())
	if e != nil {
		t.Fatal(e)
	}
	defer scoped.Close()
	_, e = scoped.ExecContext(ctx, `CREATE TABLE users(id bigint PRIMARY KEY,balance numeric(20,8) DEFAULT 0,updated_at timestamptz,role text DEFAULT 'user',status text DEFAULT 'active',deleted_at timestamptz);
 CREATE TABLE api_keys(key text,user_id bigint,deleted_at timestamptz);
 CREATE TABLE redeem_codes(code varchar(32) UNIQUE,type text,value numeric(20,8),status text,used_by bigint,used_at timestamptz,created_at timestamptz);
 INSERT INTO users(id,role) VALUES(1,'user'),(2,'admin');
 CREATE TABLE sub_adds_recharge_orders(id varchar(32) PRIMARY KEY,user_id bigint NOT NULL,package_id text NOT NULL,amount_cents bigint NOT NULL,credit numeric(20,8) NOT NULL,payment_type text NOT NULL,status text DEFAULT 'pending',trade_no text UNIQUE,created_at timestamptz DEFAULT now(),paid_at timestamptz,cache_synced boolean DEFAULT false);
 UPDATE users SET balance=5 WHERE id=1;
 INSERT INTO sub_adds_recharge_orders(id,user_id,package_id,amount_cents,credit,payment_type,status,trade_no,paid_at) VALUES('legacy-paid-before-upgrade',1,'5',500,5,'alipay','paid','legacy-trade',now());
 INSERT INTO redeem_codes(code,type,value,status,used_by) VALUES('legacy-paid-before-upgrade','balance',5,'used',1);`)
	if e != nil {
		t.Fatal(e)
	}
	cfg := recharge.Config{Gateway: "https://unused.example", PublicURL: "https://app.example", PID: "test", Key: "secret", DatabaseURL: u.String(), RedisURL: os.Getenv("RECHARGE_TEST_REDIS_URL"), Packages: []recharge.Package{{ID: "5", Name: "5 USD", AmountCents: 500, Credit: "5"}}}
	svc, e := recharge.New(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer svc.Close()
	var legacyCents int64
	var legacyCredit, legacyBalance string
	if e = scoped.QueryRowContext(ctx, `SELECT paid_amount_cents,credited_credit::text,(SELECT balance::text FROM users WHERE id=1) FROM sub_adds_recharge_orders WHERE id='legacy-paid-before-upgrade'`).Scan(&legacyCents, &legacyCredit, &legacyBalance); e != nil || legacyCents != 500 || legacyCredit != "5.00000000" || legacyBalance != "5.00000000" {
		t.Fatal("legacy migration changed balance or settlement", e, legacyCents, legacyCredit, legacyBalance)
	}

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id int
		if r.Header.Get("Authorization") == "Bearer user" {
			id = 1
		} else if r.Header.Get("Authorization") == "Bearer admin" {
			id = 2
		} else {
			w.WriteHeader(401)
			return
		}
		// A claimed admin role in the auth response must not override the database role.
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "role": "admin"}})
	}))
	defer auth.Close()
	a := &app{base: auth.URL, client: auth.Client(), recharge: svc}
	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/recharge/"+path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.rechargeRoute(w, r)
		return w
	}
	for _, tc := range []struct {
		method, path, token, body string
		want                      int
	}{
		{"GET", "admin/orders", "", "", 401},
		{"GET", "admin/issues", "invalid", "", 401},
		{"GET", "admin/orders", "user", "", 403},
		{"GET", "admin/issues", "user", "", 403},
		{"POST", "admin/orders/fake/refund", "user", `{"reason":"测试","reference":"ref","confirmed":true}`, 403},
		{"GET", "admin/orders", "admin", "", 200},
		{"GET", "admin/issues", "admin", "", 200},
		{"POST", "admin/orders/fake/refund", "admin", `{"reason":"已核对退款"}`, 400},
		{"POST", "admin/issues/1/resolve", "admin", `{"reason":""}`, 400},
	} {
		t.Run(tc.method+tc.path+tc.token, func(t *testing.T) {
			r := request(tc.method, tc.path, tc.token, tc.body)
			if r.Code != tc.want {
				t.Fatalf("got %d want %d: %s", r.Code, tc.want, r.Body.String())
			}
		})
	}
	if r := request("GET", "access", "user", ""); r.Code != 200 || !strings.Contains(r.Body.String(), `"is_admin":false`) {
		t.Fatal("untrusted admin role exposed in access", r.Body.String())
	}
	other, e := svc.Create(ctx, 2, "5", "alipay")
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"orders/" + other.ID, "orders/" + other.ID + "/recheck", "orders/" + other.ID + "/checkout"} {
		method := "GET"
		if strings.HasSuffix(path, "/recheck") || strings.HasSuffix(path, "/checkout") {
			method = "POST"
		}
		if r := request(method, path, "user", ""); r.Code != 404 {
			t.Fatal("other user's order exposed", r.Code)
		}
	}
	if r := request("GET", "logs", "user", ""); strings.Contains(r.Body.String(), other.ID) {
		t.Fatal("other user's logs exposed")
	}
	if r := request("GET", "orders", "user", ""); strings.Contains(r.Body.String(), other.ID) {
		t.Fatal("other user's orders exposed")
	}
	created := request("POST", "orders", "user", `{"package_id":"5","payment_type":"alipay","user_id":2,"amount_cents":1,"credit":"999999","paid_amount_cents":999999,"credited_credit":"999999"}`)
	if created.Code != 200 {
		t.Fatal(created.Code, created.Body.String())
	}
	var result struct {
		Data struct {
			Order recharge.Order `json:"order"`
		} `json:"data"`
	}
	if e = json.Unmarshal(created.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	var uid, amount int64
	var credit string
	if e = scoped.QueryRowContext(ctx, `SELECT user_id,amount_cents,credit::text FROM sub_adds_recharge_orders WHERE id=$1`, result.Data.Order.ID).Scan(&uid, &amount, &credit); e != nil {
		t.Fatal(e)
	}
	if uid != 1 || amount != 500 || credit != "5.00000000" {
		t.Fatal("client-controlled order fields accepted", uid, amount, credit)
	}
	var clean bool
	scoped.QueryRowContext(ctx, `SELECT paid_amount_cents IS NULL AND credited_credit IS NULL FROM sub_adds_recharge_orders WHERE id=$1`, result.Data.Order.ID).Scan(&clean)
	if !clean {
		t.Fatal("client injected settlement fields")
	}
	scoped.ExecContext(ctx, `UPDATE users SET status='disabled' WHERE id=2`)
	if r := request("GET", "admin/orders", "admin", ""); r.Code != 403 {
		t.Fatal("disabled admin accepted", r.Code)
	}
}
