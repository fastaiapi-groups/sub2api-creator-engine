package main

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"sub-adds/backend/internal/recharge"
	"time"
)

type dailyUsage struct {
	Date        string  `json:"date"`
	ImageCount  float64 `json:"image_count"`
	TotalAmount float64 `json:"total_amount"`
	TokenAmount float64 `json:"token_amount"`
}

type rechargeStats struct {
	Daily          []recharge.DailyRecharge `json:"daily"`
	YesterdayTotal float64                  `json:"yesterday_total"`
	TodayTotal     float64                  `json:"today_total"`
	Usage          []dailyUsage             `json:"usage"`
}

func (a *app) enrichAdminOrders(ctx context.Context, orders []recharge.AdminOrder) error {
	if a.sub2db == nil {
		return nil
	}
	for i := range orders {
		if err := a.sub2db.QueryRowContext(ctx, `SELECT COALESCE(email,'') FROM users WHERE id=$1`, orders[i].UserID).Scan(&orders[i].OwnerEmail); err != nil && err != sql.ErrNoRows {
			return err
		}
	}
	return nil
}

func (a *app) rechargeStats(ctx context.Context) (rechargeStats, error) {
	daily, err := a.recharge.DailyRecharge(ctx)
	if err != nil {
		return rechargeStats{}, err
	}
	stats := rechargeStats{Daily: daily, Usage: make([]dailyUsage, 0, 30)}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	yesterday := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	for _, item := range daily {
		if item.Date == today {
			stats.TodayTotal = item.Total
		}
		if item.Date == yesterday {
			stats.YesterdayTotal = item.Total
		}
	}
	if a.sub2db == nil {
		return stats, nil
	}
	quotaPerUnit := 1.0
	var quotaPerUnitText string
	if err := a.sub2db.QueryRowContext(ctx, `SELECT value FROM options WHERE key='QuotaPerUnit' LIMIT 1`).Scan(&quotaPerUnitText); err == nil {
		if parsed, parseErr := strconv.ParseFloat(quotaPerUnitText, 64); parseErr == nil && parsed > 0 {
			quotaPerUnit = parsed
		}
	}
	rows, err := a.sub2db.QueryContext(ctx, `SELECT to_char(to_timestamp(created_at) AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'), COALESCE(sum(quota),0), COALESCE(sum(prompt_tokens)+sum(completion_tokens),0), COALESCE(sum(CASE WHEN COALESCE(NULLIF(other::jsonb->>'image_count',''),'0') ~ '^[0-9]+(\\.[0-9]+)?$' THEN (other::jsonb->>'image_count')::numeric ELSE 0 END),0) FROM logs WHERE type=2 AND created_at >= extract(epoch FROM now()-interval '30 days') GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return stats, fmt.Errorf("usage statistics unavailable: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item dailyUsage
		if err := rows.Scan(&item.Date, &item.TotalAmount, &item.TokenAmount, &item.ImageCount); err != nil {
			return stats, err
		}
		item.TotalAmount /= quotaPerUnit
		item.TokenAmount /= quotaPerUnit
		stats.Usage = append(stats.Usage, item)
	}
	return stats, rows.Err()
}
