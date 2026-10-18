package recharge

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Reconcile claims a short database lease, so user refreshes and multiple instances
// cannot flood the gateway. Manual actions still require fresh platform evidence.
func (s *Service) Reconcile(ctx context.Context, id string, actor int64, reason string) error {
	var claimed string
	err := s.db.QueryRowContext(ctx, `UPDATE sub_adds_recharge_orders SET last_checked_at=now(),
 next_check_at=now()+CASE WHEN created_at>now()-interval '1 hour' THEN interval '1 minute'
 WHEN created_at>now()-interval '1 day' THEN interval '5 minutes'
 WHEN created_at>now()-interval '7 days' THEN interval '1 hour' ELSE interval '1 day' END
 WHERE id=$1 AND reversed_at IS NULL AND (last_checked_at IS NULL OR last_checked_at<now()-interval '15 seconds') RETURNING id`, id).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrThrottled
	}
	if err != nil {
		return err
	}
	o, err := scanOrder(s.db.QueryRowContext(ctx, `SELECT `+fields+` FROM sub_adds_recharge_orders WHERE id=$1`, id))
	if err != nil {
		return err
	}
	if actor > 0 {
		detail, _ := json.Marshal(map[string]any{"actor_id": actor, "reason": reason})
		if _, err = s.db.ExecContext(ctx, `INSERT INTO sub_adds_recharge_logs(order_id,event,detail) VALUES($1,'recheck_requested',$2)`, id, string(detail)); err != nil {
			return err
		}
	}
	return s.verifyAndCredit(ctx, o, "")
}
func (s *Service) reconcileDue(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sub_adds_recharge_orders WHERE next_check_at<=now() AND reversed_at IS NULL AND (status='pending' OR paid_at>now()-interval '7 days') ORDER BY next_check_at LIMIT 5`)
	if err != nil {
		log.Printf("recharge reconciliation queue unavailable")
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
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		err = s.Reconcile(ctx, id, 0, "")
		if err != nil && !errors.Is(err, ErrUnpaid) && !errors.Is(err, ErrGatewayOrderMissing) && !errors.Is(err, ErrThrottled) {
			log.Printf("recharge reconciliation pending for %s", id)
		}
	}
}
func (s *Service) recordIssue(ctx context.Context, id, kind, detail string) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("recharge issue persistence unavailable for %s", id)
		return
	}
	defer tx.Rollback()
	var count int64
	err = tx.QueryRowContext(ctx, `INSERT INTO sub_adds_recharge_issues(order_id,kind,detail) VALUES($1,$2,$3)
 ON CONFLICT(order_id,kind) DO UPDATE SET detail=EXCLUDED.detail,updated_at=now(),
 occurrences=CASE WHEN sub_adds_recharge_issues.resolved_at IS NULL THEN sub_adds_recharge_issues.occurrences+1 ELSE 1 END,
 notified_at=CASE WHEN sub_adds_recharge_issues.resolved_at IS NULL THEN sub_adds_recharge_issues.notified_at ELSE NULL END,
 notify_after=CASE WHEN sub_adds_recharge_issues.resolved_at IS NULL THEN sub_adds_recharge_issues.notify_after ELSE now() END,
 notify_attempts=CASE WHEN sub_adds_recharge_issues.resolved_at IS NULL THEN sub_adds_recharge_issues.notify_attempts ELSE 0 END,
 alert_generation=CASE WHEN sub_adds_recharge_issues.resolved_at IS NULL THEN sub_adds_recharge_issues.alert_generation ELSE sub_adds_recharge_issues.alert_generation+1 END,
 resolved_at=NULL,resolved_by=NULL,resolution='' RETURNING occurrences`, id, kind, detail).Scan(&count)
	if err == nil && count == 1 {
		_, err = tx.ExecContext(ctx, `INSERT INTO sub_adds_recharge_logs(order_id,event,detail) VALUES($1,'exception',$2)`, id, detail)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		log.Printf("recharge issue persistence unavailable for %s", id)
	}
}

type Issue struct {
	ID             int64      `json:"id"`
	OrderID        string     `json:"order_id"`
	Kind           string     `json:"kind"`
	Detail         string     `json:"detail"`
	Occurrences    int64      `json:"occurrences"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	Resolution     string     `json:"resolution"`
	NotifiedAt     *time.Time `json:"notified_at"`
	NotifyAttempts int        `json:"notify_attempts"`
}

func (s *Service) Issues(ctx context.Context) ([]Issue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,order_id,kind,detail,occurrences,created_at,updated_at,resolved_at,resolution,notified_at,notify_attempts FROM sub_adds_recharge_issues ORDER BY (resolved_at IS NULL) DESC,updated_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Issue{}
	for rows.Next() {
		var v Issue
		if err = rows.Scan(&v.ID, &v.OrderID, &v.Kind, &v.Detail, &v.Occurrences, &v.CreatedAt, &v.UpdatedAt, &v.ResolvedAt, &v.Resolution, &v.NotifiedAt, &v.NotifyAttempts); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) IsAdmin(ctx context.Context, uid int64) bool {
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT role='admin' AND status='active' AND deleted_at IS NULL FROM users WHERE id=$1`, uid).Scan(&ok)
	return err == nil && ok
}

type AdminOrder struct {
	Order
	OwnerID     int64 `json:"user_id"`
	CacheSynced bool  `json:"cache_synced"`
	OpenIssues  int   `json:"open_issues"`
}

func (s *Service) AdminOrders(ctx context.Context) ([]AdminOrder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+fields+`,cache_synced,(SELECT count(*) FROM sub_adds_recharge_issues i WHERE i.order_id=o.id AND i.resolved_at IS NULL) FROM sub_adds_recharge_orders o ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminOrder{}
	for rows.Next() {
		var a AdminOrder
		dest := append(orderTargets(&a.Order), &a.CacheSynced, &a.OpenIssues)
		if err = rows.Scan(dest...); err != nil {
			return nil, err
		}
		a.OwnerID = a.UserID
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Service) runReconciliation(ctx context.Context) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		s.reconcileDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func validNote(v string) bool { return len(strings.TrimSpace(v)) >= 3 && len(v) <= 1000 }
func (s *Service) ResolveIssue(ctx context.Context, id, actor int64, note string) error {
	if !validNote(note) {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oid string
	err = tx.QueryRowContext(ctx, `UPDATE sub_adds_recharge_issues SET resolved_at=now(),resolved_by=$2,resolution=$3 WHERE id=$1 AND resolved_at IS NULL RETURNING order_id`, id, actor, note).Scan(&oid)
	if err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]any{"issue_id": id, "actor_id": actor, "reason": note})
	_, err = tx.ExecContext(ctx, `INSERT INTO sub_adds_recharge_logs(order_id,event,detail) VALUES($1,'issue_resolved',$2)`, oid, string(detail))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RegisterRefund records an already completed FULL external refund. It does not
// call a gateway refund API. A unique reference and row lock prevent double debit.
func (s *Service) RegisterRefund(ctx context.Context, id string, actor int64, reference, note string) error {
	if len(strings.TrimSpace(reference)) < 3 || len(reference) > 128 || !validNote(note) {
		return ErrConflict
	}
	// Keep the request audit outside the money transaction, so a rejected or rolled-back
	// refund attempt still records its administrator, reference and reason.
	requestDetail, _ := json.Marshal(map[string]any{"actor_id": actor, "refund_reference": reference, "reason": note})
	audit, err := s.db.ExecContext(ctx, `INSERT INTO sub_adds_recharge_logs(order_id,event,detail) SELECT id,'refund_requested',$2 FROM sub_adds_recharge_orders WHERE id=$1`, id, string(requestDetail))
	if err != nil {
		return err
	}
	n, err := audit.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := scanOrder(tx.QueryRowContext(ctx, `SELECT `+fields+` FROM sub_adds_recharge_orders WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if o.ReversedAt != nil {
		var old string
		err = tx.QueryRowContext(ctx, `SELECT refund_reference FROM sub_adds_recharge_orders WHERE id=$1`, id).Scan(&old)
		if err != nil {
			return err
		}
		if old == reference {
			return nil
		}
		return ErrConflict
	}
	if o.Status != "paid" {
		return ErrConflict
	}
	var balance string
	err = tx.QueryRowContext(ctx, `UPDATE users SET balance=balance-$1::numeric,updated_at=now() WHERE id=$2 AND deleted_at IS NULL AND balance>=$1::numeric RETURNING balance::text`, o.settledCredit(), o.UserID).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		s.recordIssue(ctx, id, "refund_balance", "管理员登记退款时可用余额不足或用户已删除，未扣余额；需要人工处理已消费额度")
		return ErrRefundBalance
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO redeem_codes(code,type,value,status,used_by,used_at,created_at) VALUES($1,'admin_balance',-$2::numeric,'used',$3,now(),now())`, "RV"+id[2:], o.settledCredit(), o.UserID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET reversed_at=now(),refund_reference=$2,cache_synced=false,cache_version=cache_version+1 WHERE id=$1`, id, reference)
	if err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]any{"actor_id": actor, "refund_reference": reference, "reason": note, "debit": o.settledCredit(), "balance_after": balance})
	_, err = tx.ExecContext(ctx, `INSERT INTO sub_adds_recharge_logs(order_id,event,detail) VALUES($1,'refund_registered',$2)`, id, string(detail))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE sub_adds_recharge_issues SET resolved_at=now(),resolved_by=$2,resolution='管理员确认外部全额退款并完成余额冲正' WHERE order_id=$1 AND resolved_at IS NULL`, id, actor)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err = s.syncOrder(ctx, id); err != nil {
		s.recordIssue(ctx, id, "cache_sync", "退款冲正已完成，缓存刷新失败，后台将继续重试")
	}
	return nil
}
func (s *Service) dispatchAlerts(parent context.Context) {
	if s.cfg.AlertWebhook == "" {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	// Claim with a lease; at-least-once delivery, receiver can deduplicate by issue_id.
	var id int64
	var oid, kind, detail string
	var attempt int
	var generation int64
	err := s.db.QueryRowContext(ctx, `UPDATE sub_adds_recharge_issues SET notify_after=now()+interval '1 minute',notify_attempts=notify_attempts+1 WHERE id=(SELECT id FROM sub_adds_recharge_issues WHERE resolved_at IS NULL AND notified_at IS NULL AND notify_after<=now() ORDER BY notify_after LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id,order_id,kind,detail,notify_attempts,alert_generation`).Scan(&id, &oid, &kind, &detail, &attempt, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		log.Print("recharge alert queue unavailable")
		return
	}
	body, _ := json.Marshal(map[string]any{"event": "recharge_exception", "issue_id": id, "generation": generation, "order_id": oid, "kind": kind, "detail": detail})
	req, err := http.NewRequestWithContext(ctx, "POST", s.cfg.AlertWebhook, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.AlertToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.AlertToken)
	}
	resp, err := s.client.Do(req)
	ok := err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300
	if resp != nil {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
	}
	if ok {
		_, err = s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_issues SET notified_at=now() WHERE id=$1 AND alert_generation=$2 AND resolved_at IS NULL`, id, generation)
		if err != nil {
			log.Print("recharge alert acknowledgement pending")
		}
		return
	}
	delay := 60 << min(attempt-1, 6)
	if delay > 3600 {
		delay = 3600
	}
	_, err = s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_issues SET notify_after=now()+$2*interval '1 second' WHERE id=$1 AND alert_generation=$3`, id, delay, generation)
	if err != nil {
		log.Print("recharge alert retry scheduling pending")
	}
	log.Printf("recharge alert delivery pending for issue %d", id)
}
