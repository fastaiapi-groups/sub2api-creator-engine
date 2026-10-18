package recharge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed schema.sql
var schema string

var ErrPendingLimit = errors.New("待支付订单过多，请稍后再试")

type Service struct {
	db     *sql.DB
	redis  *redis.Client
	cfg    Config
	client *http.Client
}
type Order struct {
	PaidAmountCents *int64         `json:"paid_amount_cents"`
	CreditedCredit  *string        `json:"credited_credit"`
	ID              string         `json:"id"`
	UserID          int64          `json:"-"`
	PackageID       string         `json:"package_id"`
	AmountCents     int64          `json:"amount_cents"`
	Credit          string         `json:"credit"`
	PaymentType     string         `json:"payment_type"`
	Status          string         `json:"status"`
	TradeNo         sql.NullString `json:"-"`
	CreatedAt       time.Time      `json:"created_at"`
	PaidAt          *time.Time     `json:"paid_at"`
	LastCheckedAt   *time.Time     `json:"last_checked_at"`
	GatewayStatus   string         `json:"gateway_status"`
	ReversedAt      *time.Time     `json:"reversed_at"`
}

func New(ctx context.Context, c Config) (*Service, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	db, err := sql.Open("pgx", c.DatabaseURL)
	if err != nil {
		return nil, err
	}
	o, err := redis.ParseURL(c.RedisURL)
	if err != nil {
		db.Close()
		return nil, err
	}
	r := redis.NewClient(o)
	s := &Service{db: db, redis: r, cfg: c, client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	if err = db.PingContext(ctx); err == nil {
		err = r.Ping(ctx).Err()
	}
	if err == nil {
		_, err = db.ExecContext(ctx, schema)
	}
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (o Order) settledCredit() string {
	if o.CreditedCredit != nil {
		return *o.CreditedCredit
	}
	return o.Credit
}
func (s *Service) Close()              { s.db.Close(); s.redis.Close() }
func (s *Service) Packages() []Package { return s.cfg.Packages }
func orderTargets(o *Order) []any {
	return []any{&o.ID, &o.UserID, &o.PackageID, &o.AmountCents, &o.Credit, &o.PaymentType, &o.Status, &o.TradeNo, &o.CreatedAt, &o.PaidAt, &o.LastCheckedAt, &o.GatewayStatus, &o.ReversedAt, &o.PaidAmountCents, &o.CreditedCredit}
}
func scanOrder(row interface{ Scan(...any) error }) (o Order, err error) {
	err = row.Scan(orderTargets(&o)...)
	return
}

const fields = `id,user_id,package_id,amount_cents,credit::text,payment_type,status,trade_no,created_at,paid_at,last_checked_at,gateway_status,reversed_at,paid_amount_cents,credited_credit::text`

func (s *Service) Create(ctx context.Context, uid int64, pid, pt string) (Order, error) {
	return s.CreateWithKey(ctx, uid, pid, pt, "")
}
func (s *Service) CreateWithKey(ctx context.Context, uid int64, pid, pt, key string) (Order, error) {
	if key != "" && !requestKeyPattern.MatchString(key) {
		return Order{}, ErrConflict
	}
	if pt != "alipay" && pt != "wxpay" {
		return Order{}, fmt.Errorf("请选择支付宝或微信支付")
	}
	var p Package
	for _, x := range s.cfg.Packages {
		if x.ID == pid {
			p = x
			break
		}
	}
	if p.ID == "" {
		return Order{}, fmt.Errorf("套餐不存在")
	}
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		return Order{}, err
	}
	id := "RA" + hex.EncodeToString(b)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback()
	// Bound pending orders per user; lock serializes concurrent order creation.
	var exists int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL AND status='active' FOR UPDATE`, uid).Scan(&exists); err != nil {
		return Order{}, err
	}
	if key != "" {
		existing, e := scanOrder(tx.QueryRowContext(ctx, `SELECT `+fields+` FROM sub_adds_recharge_orders WHERE user_id=$1 AND request_key=$2`, uid, key))
		if e == nil {
			if existing.PackageID != pid || existing.PaymentType != pt {
				return Order{}, ErrConflict
			}
			return existing, tx.Commit()
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return Order{}, e
		}
	}
	var count int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sub_adds_recharge_orders WHERE user_id=$1 AND status='pending' AND created_at>now()-interval '1 hour'`, uid).Scan(&count)
	if err != nil {
		return Order{}, err
	}
	if count >= 20 {
		return Order{}, ErrPendingLimit
	}
	o, err := scanOrder(tx.QueryRowContext(ctx, `INSERT INTO sub_adds_recharge_orders(id,user_id,package_id,amount_cents,credit,payment_type,request_key,payment_state) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),'new') RETURNING `+fields, id, uid, p.ID, p.AmountCents, p.Credit, pt, key))
	if err != nil {
		return o, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sub_adds_recharge_logs(order_id,event,detail) VALUES($1,'created','订单已创建')`, id)
	if err != nil {
		return o, err
	}
	return o, tx.Commit()
}
func (s *Service) Get(ctx context.Context, uid int64, id string) (Order, error) {
	return scanOrder(s.db.QueryRowContext(ctx, `SELECT `+fields+` FROM sub_adds_recharge_orders WHERE id=$1 AND user_id=$2`, id, uid))
}
func (s *Service) List(ctx context.Context, uid int64) ([]Order, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+fields+` FROM sub_adds_recharge_orders WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		o, e := scanOrder(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (s *Service) Logs(ctx context.Context, uid int64) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT l.order_id,l.event,l.detail,l.created_at FROM sub_adds_recharge_logs l JOIN sub_adds_recharge_orders o ON o.id=l.order_id WHERE o.user_id=$1 ORDER BY l.id DESC LIMIT 100`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, event, detail string
		var t time.Time
		if err = rows.Scan(&id, &event, &detail, &t); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"order_id": id, "event": event, "detail": detail, "created_at": t})
	}
	return out, rows.Err()
}

// credit receives only server-verified settlement values from verifyAndCredit.
// Never pass a raw HTTP notification or browser payload to this method.
func (s *Service) credit(ctx context.Context, v url.Values) error {
	cents, err := parseMoney(v.Get("money"))
	if err != nil {
		return err
	}
	if v.Get("pid") != s.cfg.PID || v.Get("trade_status") != "TRADE_SUCCESS" || v.Get("trade_no") == "" || len(v.Get("trade_no")) > 128 {
		return ErrPaymentFields
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := scanOrder(tx.QueryRowContext(ctx, `SELECT `+fields+` FROM sub_adds_recharge_orders WHERE id=$1 FOR UPDATE`, v.Get("out_trade_no")))
	if err != nil {
		return err
	}
	if o.AmountCents != cents || o.PaymentType != v.Get("type") {
		return ErrPaymentMismatch
	}
	if o.TradeNo.Valid && o.TradeNo.String != v.Get("trade_no") {
		return ErrTradeConflict
	}
	paidCents := cents
	if v.Get("paid_money") != "" {
		paidCents, err = parseMoney(v.Get("paid_money"))
		if err != nil || paidCents <= 0 {
			return ErrPaymentMismatch
		}
	}
	if o.Status == "paid" {
		if o.PaidAmountCents != nil && *o.PaidAmountCents != paidCents {
			return ErrPaymentMismatch
		}
		return nil
	}
	var credited string
	// PostgreSQL numeric arithmetic avoids float rounding and snapshots the quote's rate.
	if err = tx.QueryRowContext(ctx, `SELECT trunc($1::numeric*$2::numeric/$3::numeric,8)::text`, o.Credit, paidCents, o.AmountCents).Scan(&credited); err != nil {
		return err
	}
	if strings.Trim(credited, "0.") == "" {
		return ErrPaymentMismatch
	}
	var balance string
	err = tx.QueryRowContext(ctx, `UPDATE users SET balance=balance+$1::numeric,updated_at=now() WHERE id=$2 AND deleted_at IS NULL RETURNING balance::text`, credited, o.UserID).Scan(&balance)
	if err != nil {
		return err
	}
	// Populate the official recharge history in the same transaction.
	_, err = tx.ExecContext(ctx, `INSERT INTO redeem_codes(code,type,value,status,used_by,used_at,created_at) VALUES($1,'balance',$2,'used',$3,now(),now())`, o.ID, credited, o.UserID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET status='paid',trade_no=$2,paid_at=now(),cache_version=cache_version+1,paid_amount_cents=$3,credited_credit=$4 WHERE id=$1`, o.ID, v.Get("trade_no"), paidCents, credited)
	if err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]any{"credit": credited, "balance_after": balance, "trade_no": v.Get("trade_no"), "paid_amount_cents": paidCents, "quoted_amount_cents": o.AmountCents, "quoted_credit": o.Credit})
	_, err = tx.ExecContext(ctx, `INSERT INTO sub_adds_recharge_logs(order_id,event,detail) VALUES($1,'credited',$2)`, o.ID, string(detail))
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.resolvePaymentIssues(ctx, o.ID)
}
func (s *Service) Notify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "fail", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil || !validSignature(r.Form, s.cfg.Key) {
		http.Error(w, "fail", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	// Validate the callback against the saved order before issuing an authenticated gateway query.
	o, err := scanOrder(s.db.QueryRowContext(ctx, `SELECT `+fields+` FROM sub_adds_recharge_orders WHERE id=$1`, r.Form.Get("out_trade_no")))
	if err != nil {
		http.Error(w, "fail", 400)
		return
	}
	if err = s.validateNotification(o, r.Form); err != nil {
		s.recordIssue(ctx, o.ID, "callback_mismatch", "支付通知的商户、金额、方式或流水与订单不一致")
		http.Error(w, "fail", 400)
		return
	}
	// Already committed deliveries can be acknowledged during gateway outages; never credit again.
	if o.Status == "paid" && o.TradeNo.String == r.Form.Get("trade_no") {
		if err = s.syncOrder(ctx, o.ID); err != nil {
			log.Printf("recharge cache refresh pending")
		}
		fmt.Fprint(w, "success")
		return
	}
	if err = s.verifyAndCredit(ctx, o, r.Form.Get("trade_no")); err != nil {
		log.Printf("recharge callback verification pending for %s", o.ID)
		http.Error(w, "fail", 503)
		return
	}
	// The durable worker also retries after Redis failures or process restarts.
	if err := s.syncOrder(ctx, r.Form.Get("out_trade_no")); err != nil {
		s.recordIssue(ctx, r.Form.Get("out_trade_no"), "cache_sync", "余额已入账，缓存刷新失败，后台将继续重试")
		log.Printf("recharge cache refresh pending")
	}
	fmt.Fprint(w, "success")
}
func (s *Service) syncOrder(ctx context.Context, id string) error {
	var uid, version int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id,cache_version FROM sub_adds_recharge_orders WHERE id=$1 AND status='paid' AND NOT cache_synced`, id).Scan(&uid, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key FROM api_keys WHERE user_id=$1 AND deleted_at IS NULL`, uid)
	if err != nil {
		return err
	}
	hashes := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		h := sha256.Sum256([]byte(key))
		hashes = append(hashes, hex.EncodeToString(h[:]))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	pipe := s.redis.Pipeline()
	pipe.Del(ctx, fmt.Sprintf("billing:balance:%d", uid))
	for _, h := range hashes {
		pipe.Del(ctx, "apikey:auth:"+h)
		pipe.Publish(ctx, "auth:cache:invalidate", h)
	}
	if _, err = pipe.Exec(ctx); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET cache_synced=true WHERE id=$1 AND cache_version=$2`, id, version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 1 {
		_, err = s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_issues SET resolved_at=now(),resolution='缓存刷新恢复' WHERE order_id=$1 AND kind='cache_sync' AND resolved_at IS NULL`, id)
	}
	return err
}
func (s *Service) Run(ctx context.Context) {
	go s.runReconciliation(ctx)
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		s.syncPending(ctx)
		s.dispatchAlerts(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *Service) syncPending(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sub_adds_recharge_orders WHERE status='paid' AND NOT cache_synced ORDER BY paid_at LIMIT 100`)
	if err != nil {
		log.Printf("recharge cache queue: %v", err)
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if e := rows.Err(); e != nil {
		err = e
	}
	rows.Close()
	if err != nil {
		log.Printf("recharge cache queue: %v", err)
		return
	}
	for _, id := range ids {
		if err = s.syncOrder(ctx, id); err != nil {
			s.recordIssue(ctx, id, "cache_sync", "余额已入账，缓存刷新失败，后台将继续重试")
			log.Printf("recharge cache retry pending for %s", id)
			if ctx.Err() != nil {
				return
			}
		}
	}
}
