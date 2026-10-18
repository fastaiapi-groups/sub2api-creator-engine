package recharge

import (
	"net/url"
	"testing"
)

func TestSign(t *testing.T) {
	v := url.Values{"b": {"hello world"}, "item": {"1"}, "empty": {""}, "sign": {"ignored"}, "sign_type": {"MD5"}}
	// MD5 of the raw, sorted string b=hello world&item=1secret.
	if got := Sign(v, "secret"); got != "dacefcb257ba43694552273ce04fd25a" {
		t.Fatalf("sign = %s", got)
	}
	v.Set("sign", Sign(v, "secret"))
	if !validSignature(v, "secret") {
		t.Fatal("valid signature rejected")
	}
	v["item"] = []string{"1", "2"}
	if validSignature(v, "secret") {
		t.Fatal("duplicate parameter accepted")
	}
}
func TestMoney(t *testing.T) {
	for s, n := range map[string]int64{"5": 500, "5.0": 500, "5.00": 500, "0.01": 1, "1000.99": 100099} {
		got, e := parseMoney(s)
		if e != nil || got != n {
			t.Fatalf("%s = %d %v", s, got, e)
		}
	}
	for _, s := range []string{"-1", "1.001", "1e2", "NaN", " 5", "+5", ""} {
		if _, e := parseMoney(s); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	if money(501) != "5.01" {
		t.Fatal("format")
	}
}
func TestValidate(t *testing.T) {
	c := Config{Gateway: "https://pay.example", PublicURL: "https://app.example", PID: "merchant", Key: "secret", DatabaseURL: "db", RedisURL: "redis", Packages: []Package{{"5", "5 USD", 500, "5"}}}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, credit := range []string{"-1", "0", "0.00000000", "1e3", "1.123456789"} {
		bad := c
		bad.Packages = []Package{{"5", "5 USD", 500, credit}}
		if bad.Validate() == nil {
			t.Fatal("accepted", credit)
		}
	}
	c.Packages = append(c.Packages, c.Packages[0])
	if c.Validate() == nil {
		t.Fatal("duplicate package")
	}
}

// Vector published by the gateway, including Chinese text and raw URL ampersands.
func TestGatewaySignatureVector(t *testing.T) {
	v := url.Values{"pid": {"M1234567890"}, "type": {"alipay"}, "out_trade_no": {"ORDER202608130001"}, "notify_url": {"https://shop.example.com/pay/notify?channel=alipay&version=2"}, "return_url": {"https://shop.example.com/pay/return?order=ORDER202608130001&source=pay"}, "name": {"测试订单"}, "money": {"1.00"}}
	if got := Sign(v, "test_pay_key_20260813"); got != "beddd35092c3afea54c5218de0872502" {
		t.Fatal(got)
	}
	v.Set("a", "ignored-route")
	v.Set("c", "ignored-route")
	v.Set("m", "ignored-route")
	v.Set("s", "ignored-route")
	if got := Sign(v, "test_pay_key_20260813"); got != "beddd35092c3afea54c5218de0872502" {
		t.Fatal("route parameter not excluded", got)
	}
}
