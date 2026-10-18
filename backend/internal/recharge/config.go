package recharge

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Package struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AmountCents int64  `json:"amount_cents"`
	Credit      string `json:"credit"`
}
type Config struct {
	Gateway, PID, Key, PublicURL, DatabaseURL, RedisURL string
	Packages                                            []Package
	AlertWebhook, AlertToken                            string
}

var decimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,10})(\.[0-9]{1,8})?$`)

func LoadConfig() (Config, error) {
	c := Config{Gateway: os.Getenv("RECHARGE_GATEWAY"), PID: os.Getenv("RECHARGE_PID"), Key: os.Getenv("RECHARGE_KEY"), PublicURL: strings.TrimRight(os.Getenv("RECHARGE_PUBLIC_URL"), "/"), DatabaseURL: os.Getenv("SUB2API_DATABASE_URL"), RedisURL: os.Getenv("SUB2API_REDIS_URL")}
	c.AlertWebhook = os.Getenv("RECHARGE_ALERT_WEBHOOK_URL")
	c.AlertToken = os.Getenv("RECHARGE_ALERT_TOKEN")
	if c.Gateway == "" {
		c.Gateway = "https://pay.1kexiu.xyz"
	}
	if err := json.Unmarshal([]byte(os.Getenv("RECHARGE_PACKAGES")), &c.Packages); err != nil && os.Getenv("RECHARGE_PACKAGES") != "" {
		return c, fmt.Errorf("invalid RECHARGE_PACKAGES: %w", err)
	}
	if len(c.Packages) == 0 {
		for _, n := range []int64{1, 5, 10, 30, 50, 100, 300, 500} {
			s := strconv.FormatInt(n, 10)
			c.Packages = append(c.Packages, Package{s, s + " 刀额度", n * 100, s})
		}
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	for _, s := range []string{c.Gateway, c.PublicURL} {
		u, e := url.Parse(s)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("recharge gateway/public URL must be an HTTPS URL without query or fragment")
		}
	}
	if c.PID == "" || c.Key == "" || c.DatabaseURL == "" || c.RedisURL == "" {
		return fmt.Errorf("recharge PID, key, Sub2API database and Redis URLs are required")
	}
	if c.AlertWebhook != "" {
		u, e := url.Parse(c.AlertWebhook)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("recharge alert webhook must use HTTPS")
		}
	}
	seen := map[string]bool{}
	for _, p := range c.Packages {
		if p.ID == "" || p.Name == "" || seen[p.ID] || p.AmountCents <= 0 || p.AmountCents > 100000000 || !decimal.MatchString(p.Credit) || strings.Trim(p.Credit, "0.") == "" {
			return fmt.Errorf("invalid or duplicate recharge package")
		}
		seen[p.ID] = true
	}
	if len(c.Packages) == 0 {
		return fmt.Errorf("recharge packages required")
	}
	return nil
}

// Sign follows the supplied gateway specification: sort raw values, omit empty/sign fields.
func Sign(v url.Values, key string) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		if k != "sign" && k != "sign_type" && k != "a" && k != "c" && k != "m" && k != "s" && v.Get(k) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+v.Get(k))
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + key))
	return hex.EncodeToString(sum[:])
}
func validSignature(v url.Values, key string) bool {
	for _, values := range v {
		if len(values) != 1 {
			return false
		}
	}
	if t := v.Get("sign_type"); t != "" && t != "MD5" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(Sign(v, key)), []byte(strings.ToLower(v.Get("sign")))) == 1
}
func money(c int64) string { return fmt.Sprintf("%d.%02d", c/100, c%100) }
func parseMoney(s string) (int64, error) {
	if !regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,2})?$`).MatchString(s) {
		return 0, fmt.Errorf("invalid amount")
	}
	p := strings.SplitN(s, ".", 2)
	n, _ := strconv.ParseInt(p[0], 10, 64)
	var f int64
	if len(p) == 2 {
		v := p[1]
		if len(v) == 1 {
			v += "0"
		}
		f, _ = strconv.ParseInt(v, 10, 64)
	}
	return n*100 + f, nil
}
