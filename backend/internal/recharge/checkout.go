package recharge

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var (
	requestKeyPattern    = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,64}$`)
	ErrCheckoutPending   = errors.New("渠道订单正在确认，请稍后重试获取二维码")
	ErrQRCodeUnavailable = errors.New("暂时无法获取支付二维码，请稍后重试或联系管理员")
)

type Checkout struct {
	Order  Order  `json:"order"`
	State  string `json:"state"`
	QRCode string `json:"qr_code,omitempty"`
	QRKind string `json:"qr_kind,omitempty"`
}
type paymentRecord struct{ state, source, sourceKind, qr, qrKind string }

func (s *Service) paymentRecord(ctx context.Context, id string) (v paymentRecord, e error) {
	e = s.db.QueryRowContext(ctx, `SELECT payment_state,payment_source,payment_source_kind,payment_qr,payment_qr_kind FROM sub_adds_recharge_orders WHERE id=$1`, id).Scan(&v.state, &v.source, &v.sourceKind, &v.qr, &v.qrKind)
	return
}

// Payment only reads cached data; GET requests never create upstream orders.
func (s *Service) Payment(ctx context.Context, uid int64, id string) (Checkout, error) {
	o, e := s.Get(ctx, uid, id)
	if e != nil {
		return Checkout{}, e
	}
	c := Checkout{Order: o, State: o.Status}
	if o.Status == "paid" || o.ReversedAt != nil {
		return c, nil
	}
	if o.GatewayStatus == "expired" {
		c.State = "expired"
		return c, nil
	}
	p, e := s.paymentRecord(ctx, id)
	if e != nil {
		return c, e
	}
	c.State = p.state
	if p.state == "ready" {
		c.QRCode = p.qr
		c.QRKind = p.qrKind
	}
	return c, nil
}
func (s *Service) paymentParams(o Order) url.Values {
	v := url.Values{"pid": {s.cfg.PID}, "type": {o.PaymentType}, "out_trade_no": {o.ID}, "name": {"API 余额充值"}, "money": {money(o.AmountCents)}, "notify_url": {s.cfg.PublicURL + "/fastaiapi/api/recharge/notify"}, "return_url": {s.cfg.PublicURL + "/fastaiui/recharge?order=" + url.QueryEscape(o.ID)}, "device": {"pc"}}
	v.Set("sign", Sign(v, s.cfg.Key))
	v.Set("sign_type", "MD5")
	return v
}

type createReply struct {
	Code    scalar `json:"code"`
	TradeNo scalar `json:"trade_no"`
	QRCode  string `json:"qrcode"`
	PayURL  string `json:"payurl"`
	Money   scalar `json:"money"`
	Price   scalar `json:"price"`
}

// Checkout reserves the request durably before making the non-idempotent upstream call.
// An ambiguous response is recovered by querying the SAME order, never by resubmitting mapi.
func (s *Service) Checkout(ctx context.Context, uid int64, id string) (Checkout, error) {
	c, e := s.Payment(ctx, uid, id)
	if e != nil {
		return c, e
	}
	if c.Order.Status == "paid" || c.Order.ReversedAt != nil || c.State == "ready" {
		return c, nil
	}
	if c.State == "expired" {
		return c, ErrConflict
	}
	p, e := s.paymentRecord(ctx, id)
	if e != nil {
		return c, e
	}
	if p.state == "blocked" {
		return c, ErrPaymentMismatch
	}
	if p.source != "" {
		return s.renderPayment(ctx, c.Order, p.source, p.sourceKind)
	}
	if p.state != "new" {
		// A crash, timeout or old redirect order may already exist at the gateway.
		// Avoid even a recovery query while an in-flight mapi request can still finish.
		var fresh bool
		e = s.db.QueryRowContext(ctx, `SELECT payment_requested_at IS NOT NULL AND payment_requested_at>now()-interval '15 seconds' FROM sub_adds_recharge_orders WHERE id=$1`, id).Scan(&fresh)
		if e != nil {
			return c, e
		}
		if fresh {
			return c, ErrCheckoutPending
		}
		return s.recoverPayment(ctx, c.Order)
	}
	var reserved string
	e = s.db.QueryRowContext(ctx, `UPDATE sub_adds_recharge_orders SET payment_state='requesting',payment_requested_at=now() WHERE id=$1 AND user_id=$2 AND status='pending' AND payment_state='new' RETURNING id`, id, uid).Scan(&reserved)
	if errors.Is(e, sql.ErrNoRows) {
		return c, ErrCheckoutPending
	}
	if e != nil {
		return c, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.cfg.Gateway, "/")+"/mapi.php", strings.NewReader(s.paymentParams(c.Order).Encode()))
	if e != nil {
		return c, e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := s.client.Do(req)
	var reply createReply
	if e == nil {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			e = ErrQuery
		} else {
			b, re := io.ReadAll(io.LimitReader(resp.Body, (128<<10)+1))
			if re != nil || len(b) > 128<<10 {
				e = ErrQuery
			} else if json.Unmarshal(b, &reply) != nil || reply.Code != "1" {
				e = ErrQuery
			}
		}
	}
	if e != nil {
		s.checkoutUncertain(id)
		return c, ErrCheckoutPending
	}
	if reply.TradeNo == "" || len(reply.TradeNo) > 128 {
		s.checkoutUncertain(id)
		return c, ErrCheckoutPending
	}
	for _, value := range []scalar{reply.Money} {
		if value != "" {
			n, e := parseMoney(string(value))
			if e != nil || n != c.Order.AmountCents {
				s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET payment_state='blocked' WHERE id=$1`, id)
				s.recordIssue(ctx, id, "payment_mismatch", "渠道创建订单时返回的应付金额无效或与本地报价不符，暂停展示二维码")
				return c, ErrPaymentMismatch
			}
		}
	}

	// Persist the provider's trade number and source before fetching the cashier image.
	source, kind := reply.QRCode, "content"
	if source == "" {
		source, kind = reply.PayURL, "cashier"
	}
	result, e := s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET trade_no=$2,payment_source=$3,payment_source_kind=$4 WHERE id=$1 AND (trade_no IS NULL OR trade_no=$2)`, id, string(reply.TradeNo), source, kind)
	if e != nil {
		s.checkoutUncertain(id)
		return c, e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return c, ErrTradeConflict
	}
	if source == "" {
		s.recordIssue(ctx, id, "qr_unavailable", "渠道已创建订单但没有返回二维码或收银台地址，请核对通道配置")
		return c, ErrQRCodeUnavailable
	}
	return s.renderPayment(ctx, c.Order, source, kind)
}
func (s *Service) checkoutUncertain(id string) {
	// Request cancellation must not erase the durable marker or cause a fresh mapi request.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET payment_state='uncertain' WHERE id=$1 AND payment_state='requesting'`, id)
	s.recordIssue(ctx, id, "checkout_uncertain", "渠道下单响应未确认，将沿用原订单查单；不会自动重复向渠道下单")
}
func (s *Service) recoverPayment(ctx context.Context, o Order) (Checkout, error) {
	c := Checkout{Order: o, State: "uncertain"}
	g, e := s.queryOrder(ctx, o.ID)
	if e != nil {
		return c, ErrCheckoutPending
	}
	n, ne := parseMoney(string(g.Money))
	if ne != nil || n <= 0 || g.Type != o.PaymentType {
		return c, ErrPaymentMismatch
	}
	if g.Status == "1" {
		if e = s.verifyAndCredit(ctx, o, ""); e != nil {
			return c, e
		}
		return s.Payment(ctx, o.UserID, o.ID)
	}
	if g.Status == "2" {
		s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET gateway_status='expired' WHERE id=$1`, o.ID)
		c.State = "expired"
		return c, ErrConflict
	}
	if g.Status != "0" || g.TradeNo == "" || len(g.TradeNo) > 128 {
		return c, ErrPaymentFields
	}
	cashier := strings.TrimRight(s.cfg.Gateway, "/") + "/gateway/Submit/pay?" + url.Values{"trade_no": {string(g.TradeNo)}}.Encode()
	result, e := s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET trade_no=$2,payment_source=$3,payment_source_kind='cashier' WHERE id=$1 AND (trade_no IS NULL OR trade_no=$2)`, o.ID, string(g.TradeNo), cashier)
	if e != nil {
		return c, e
	}
	nRows, _ := result.RowsAffected()
	if nRows != 1 {
		return c, ErrTradeConflict
	}
	return s.renderPayment(ctx, o, cashier, "cashier")
}
func (s *Service) renderPayment(ctx context.Context, o Order, source, kind string) (Checkout, error) {
	qr, qrKind, e := s.resolveQRCode(ctx, source, kind)
	if e != nil {
		s.recordIssue(ctx, o.ID, "qr_unavailable", "渠道订单已保留，但暂未获取到可用二维码；可重试获取，系统不会重复下单")
		return Checkout{Order: o, State: "uncertain"}, ErrQRCodeUnavailable
	}
	_, e = s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_orders SET payment_state='ready',payment_qr=$2,payment_qr_kind=$3 WHERE id=$1 AND status='pending'`, o.ID, qr, qrKind)
	if e != nil {
		return Checkout{}, e
	}
	_, e = s.db.ExecContext(ctx, `UPDATE sub_adds_recharge_issues SET resolved_at=now(),resolution='已获取原渠道订单二维码' WHERE order_id=$1 AND kind IN ('qr_unavailable','checkout_uncertain') AND resolved_at IS NULL`, o.ID)
	if e != nil {
		return Checkout{}, e
	}
	return s.Payment(ctx, o.UserID, o.ID)
}
func (s *Service) sameGateway(u *url.URL) bool {
	base, e := url.Parse(s.cfg.Gateway)
	return e == nil && u.Scheme == "https" && u.Host == base.Host && u.User == nil
}
func (s *Service) fetchPaymentAsset(ctx context.Context, raw string) ([]byte, error) {
	base, _ := url.Parse(s.cfg.Gateway)
	u, e := url.Parse(raw)
	if e != nil {
		return nil, ErrQRCodeUnavailable
	}
	u = base.ResolveReference(u)
	if !s.sameGateway(u) {
		return nil, ErrQRCodeUnavailable
	}
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, e
	}
	client := *s.client
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !s.sameGateway(r.URL) {
			return http.ErrUseLastResponse
		}
		return nil
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, ErrQRCodeUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, ErrQRCodeUnavailable
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if e != nil || len(b) > 2<<20 {
		return nil, ErrQRCodeUnavailable
	}
	return b, nil
}
func imageData(b []byte) (string, error) {
	if len(b) == 0 || len(b) > 1<<20 {
		return "", ErrQRCodeUnavailable
	}
	cfg, format, e := image.DecodeConfig(strings.NewReader(string(b)))
	if e != nil || cfg.Width < 32 || cfg.Height < 32 || cfg.Width > 4096 || cfg.Height > 4096 {
		return "", ErrQRCodeUnavailable
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return "", ErrQRCodeUnavailable
	}
	return "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}
func dataImage(raw string) (string, error) {
	if len(raw) > 1500000 || !strings.HasPrefix(raw, "data:image/") {
		return "", ErrQRCodeUnavailable
	}
	parts := strings.SplitN(raw, ",", 2)
	if len(parts) != 2 || !strings.HasSuffix(parts[0], ";base64") {
		return "", ErrQRCodeUnavailable
	}
	b, e := base64.StdEncoding.DecodeString(parts[1])
	if e != nil {
		return "", ErrQRCodeUnavailable
	}
	return imageData(b)
}

// Never embed the provider page or execute its scripts. Read only the QR image inside #qrarea.
func (s *Service) resolveQRCode(ctx context.Context, raw, kind string) (string, string, error) {
	if strings.HasPrefix(raw, "data:image/") {
		v, e := dataImage(raw)
		return v, "image", e
	}
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || len(raw) > 8192 {
		return "", "", ErrQRCodeUnavailable
	}
	if kind == "content" && !s.sameGateway(u) {
		switch u.Scheme {
		case "https", "http", "wxp", "weixin", "alipay", "alipays":
			return raw, "text", nil
		}
		return "", "", ErrQRCodeUnavailable
	}
	if !s.sameGateway(u) {
		if u.Scheme == "wxp" || u.Scheme == "weixin" || u.Scheme == "alipay" || u.Scheme == "alipays" || u.Hostname() == "qr.alipay.com" {
			return raw, "text", nil
		}
		return "", "", ErrQRCodeUnavailable
	}
	b, e := s.fetchPaymentAsset(ctx, raw)
	if e != nil {
		return "", "", e
	}
	if v, e := imageData(b); e == nil {
		return v, "image", nil
	}
	doc, e := html.Parse(strings.NewReader(string(b)))
	if e != nil {
		return "", "", ErrQRCodeUnavailable
	}
	var find func(*html.Node, bool) string
	find = func(n *html.Node, inside bool) string {
		for _, a := range n.Attr {
			if a.Key == "id" && a.Val == "qrarea" {
				inside = true
			}
		}
		if inside && n.Type == html.ElementNode && n.Data == "img" {
			for _, a := range n.Attr {
				if a.Key == "src" {
					return a.Val
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if v := find(c, inside); v != "" {
				return v
			}
		}
		return ""
	}
	src := find(doc, false)
	if src == "" {
		return "", "", ErrQRCodeUnavailable
	}
	if strings.HasPrefix(src, "data:image/") {
		v, e := dataImage(src)
		return v, "image", e
	}
	page, _ := url.Parse(raw)
	imgURL, e := url.Parse(src)
	if e != nil {
		return "", "", ErrQRCodeUnavailable
	}
	imgURL = page.ResolveReference(imgURL)
	b, e = s.fetchPaymentAsset(ctx, imgURL.String())
	if e != nil {
		return "", "", e
	}
	v, e := imageData(b)
	return v, "image", e
}
