package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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

type usageSourceStats struct {
	Usage        []dailyUsage `json:"usage"`
	RequestCount int64        `json:"request_count"`
	ImageCount   float64      `json:"image_count"`
	TotalAmount  float64      `json:"total_amount"`
}

type rechargeStats struct {
	Daily           []recharge.DailyRecharge `json:"daily"`
	YesterdayTotal  float64                  `json:"yesterday_total"`
	TodayTotal      float64                  `json:"today_total"`
	Usage           []dailyUsage             `json:"usage"`
	NewAPI          usageSourceStats         `json:"new_api"`
	SubAPI          usageSourceStats         `json:"sub_api"`
	NewAPIDaily     []recharge.DailyRecharge `json:"new_api_daily"`
	NewAPIYesterday float64                  `json:"new_api_yesterday_total"`
	NewAPIToday     float64                  `json:"new_api_today_total"`
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

func (a *app) rechargeStats(ctx context.Context, authorization string) (rechargeStats, error) {
	daily, err := a.recharge.DailyRecharge(ctx)
	if err != nil {
		return rechargeStats{}, err
	}
	stats := rechargeStats{Daily: daily, Usage: make([]dailyUsage, 0, 30)}
	stats.NewAPIDaily, stats.NewAPIYesterday, stats.NewAPIToday = a.newAPIRecharge(ctx)
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
	quotaPerUnit := 500000.0
	var quotaPerUnitText string
	optionDB := a.newapiDB
	if optionDB == nil {
		optionDB = a.sub2db
	}
	if err := optionDB.QueryRowContext(ctx, `SELECT value FROM options WHERE key='QuotaPerUnit' LIMIT 1`).Scan(&quotaPerUnitText); err == nil {
		if parsed, parseErr := strconv.ParseFloat(quotaPerUnitText, 64); parseErr == nil && parsed > 0 {
			quotaPerUnit = parsed
		}
	}
	logDB := a.newapiDB
	if logDB == nil {
		logDB = a.sub2db
	}
	rows, err := logDB.QueryContext(ctx, `SELECT to_char(to_timestamp(created_at) AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'), COALESCE(sum(CASE WHEN other LIKE '%Image count%' THEN quota ELSE 0 END),0), COALESCE(sum(prompt_tokens)+sum(completion_tokens),0) FROM logs WHERE type=2 AND quota > 0 AND created_at >= extract(epoch FROM now()-interval '30 days') GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return a.remoteUsageStats(ctx, authorization, stats, quotaPerUnit)
	}
	defer rows.Close()
	usageByDate := make(map[string]*dailyUsage)
	for rows.Next() {
		var item dailyUsage
		if err := rows.Scan(&item.Date, &item.TotalAmount, &item.TokenAmount); err != nil {
			return stats, err
		}
		item.TotalAmount /= quotaPerUnit
		item.TokenAmount /= quotaPerUnit
		usageByDate[item.Date] = &item
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	imageRows, err := logDB.QueryContext(ctx, `SELECT to_char(to_timestamp(created_at) AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'), COALESCE(sum((substring(other from '"params":\{"count":([0-9]+(\.[0-9]+)?)')::numeric)),0) FROM logs WHERE type=2 AND quota > 0 AND created_at >= extract(epoch FROM now()-interval '30 days') AND other LIKE '%Image count%' GROUP BY 1`)
	if err != nil {
		return a.remoteUsageStats(ctx, authorization, stats, quotaPerUnit)
	}
	for imageRows.Next() {
		var date string
		var count float64
		if err := imageRows.Scan(&date, &count); err != nil {
			imageRows.Close()
			return stats, err
		}
		item := usageByDate[date]
		if item == nil {
			item = &dailyUsage{Date: date}
			usageByDate[date] = item
		}
		item.ImageCount = count
	}
	if err := imageRows.Err(); err != nil {
		imageRows.Close()
		return stats, err
	}
	imageRows.Close()
	for _, item := range usageByDate {
		stats.Usage = append(stats.Usage, *item)
	}
	sort.Slice(stats.Usage, func(i, j int) bool { return stats.Usage[i].Date < stats.Usage[j].Date })
	stats.NewAPI = usageSourceStats{Usage: stats.Usage}
	if err := logDB.QueryRowContext(ctx, `SELECT count(*) FROM logs WHERE type=2 AND quota > 0 AND created_at >= extract(epoch FROM now()-interval '30 days')`).Scan(&stats.NewAPI.RequestCount); err == nil {
		for _, item := range stats.NewAPI.Usage {
			stats.NewAPI.ImageCount += item.ImageCount
			stats.NewAPI.TotalAmount += item.TotalAmount
		}
	}
	stats.SubAPI, err = a.subAPIUsageStats(ctx)
	if err != nil {
		return stats, err
	}
	return stats, nil
}

func (a *app) subAPIUsageStats(ctx context.Context) (usageSourceStats, error) {
	result := usageSourceStats{Usage: make([]dailyUsage, 0, 30)}
	if a.sub2db == nil {
		return result, nil
	}
	rows, err := a.sub2db.QueryContext(ctx, `SELECT to_char(created_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'), count(*), COALESCE(sum(image_count),0), COALESCE(sum(CASE WHEN image_count > 0 THEN actual_cost ELSE 0 END),0), COALESCE(sum(CASE WHEN image_count > 0 THEN 0 ELSE actual_cost END),0) FROM usage_logs WHERE actual_cost > 0 AND created_at >= now()-interval '30 days' GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item dailyUsage
		var requests int64
		if err := rows.Scan(&item.Date, &requests, &item.ImageCount, &item.TotalAmount, &item.TokenAmount); err != nil {
			return result, err
		}
		result.Usage = append(result.Usage, item)
		result.RequestCount += requests
		result.ImageCount += item.ImageCount
		result.TotalAmount += item.TotalAmount
	}
	return result, rows.Err()
}

func (a *app) newAPIRecharge(ctx context.Context) ([]recharge.DailyRecharge, float64, float64) {
	items := make([]recharge.DailyRecharge, 0, 30)
	if a.newapiDB == nil {
		return items, 0, 0
	}
	rows, err := a.newapiDB.QueryContext(ctx, `SELECT to_char(to_timestamp(created_at) AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'), COALESCE(other::jsonb->'admin_info'->>'payment_method',''), COALESCE(sum((substring(content from 'payment amount: ([0-9]+(\\.[0-9]+)?)')::double precision)),0) FROM logs WHERE type=1 AND content LIKE 'Online top-up succeeded%' AND created_at >= extract(epoch FROM now()-interval '30 days') GROUP BY 1,2 ORDER BY 1`)
	if err != nil {
		return items, 0, 0
	}
	defer rows.Close()
	byDate := map[string]*recharge.DailyRecharge{}
	for rows.Next() {
		var date, paymentType string
		var amount float64
		if rows.Scan(&date, &paymentType, &amount) != nil {
			continue
		}
		item := byDate[date]
		if item == nil {
			item = &recharge.DailyRecharge{Date: date}
			byDate[date] = item
		}
		if paymentType == "wxpay" {
			item.Wxpay += amount
		} else {
			item.Alipay += amount
		}
		item.Total += amount
	}
	for _, item := range byDate {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Date < items[j].Date })
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	yesterday := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	var yesterdayTotal, todayTotal float64
	for _, item := range items {
		if item.Date == yesterday {
			yesterdayTotal = item.Total
		}
		if item.Date == today {
			todayTotal = item.Total
		}
	}
	return items, yesterdayTotal, todayTotal
}

type remoteLog struct {
	CreatedAt        int64  `json:"created_at"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	Other            string `json:"other"`
}

func (a *app) remoteUsageStats(ctx context.Context, authorization string, stats rechargeStats, quotaPerUnit float64) (rechargeStats, error) {
	if authorization == "" {
		return stats, fmt.Errorf("usage statistics unavailable")
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	start := time.Now().In(loc).AddDate(0, 0, -29).Truncate(24 * time.Hour).Unix()
	byDate := map[string]*dailyUsage{}
	for page := 1; page <= 100; page++ {
		u := a.base + "/fastaiapi/api/v1/log/?" + url.Values{"p": {strconv.Itoa(page)}, "page_size": {"100"}, "type": {"2"}, "start_timestamp": {strconv.FormatInt(start, 10)}}.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return stats, err
		}
		req.Header.Set("Authorization", authorization)
		resp, err := a.client.Do(req)
		if err != nil {
			return stats, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if readErr != nil {
			return stats, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return stats, fmt.Errorf("usage API returned %d", resp.StatusCode)
		}
		var envelope struct {
			Data struct {
				Items []remoteLog `json:"items"`
				Total int         `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			return stats, err
		}
		items := envelope.Data.Items
		for _, logItem := range items {
			date := time.Unix(logItem.CreatedAt, 0).In(loc).Format("2006-01-02")
			v := byDate[date]
			if v == nil {
				v = &dailyUsage{Date: date}
				byDate[date] = v
			}
			v.TotalAmount += float64(logItem.Quota) / quotaPerUnit
			v.TokenAmount += float64(logItem.PromptTokens+logItem.CompletionTokens) / quotaPerUnit
			var other map[string]any
			if json.Unmarshal([]byte(logItem.Other), &other) == nil {
				if n, ok := other["image_count"].(float64); ok {
					v.ImageCount += n
				}
			}
		}
		if len(items) == 0 || page*100 >= envelope.Data.Total {
			break
		}
	}
	for _, v := range byDate {
		stats.Usage = append(stats.Usage, *v)
	}
	return stats, nil
}
