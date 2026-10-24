package clientupdates

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const Product = "fastaiapi-codexplus"
const PublicBase = "https://fastaiapi.cloud"
const DownloadPrefix = "/fastaiapi/downloads/codexplus/"

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var filenamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,179}$`)

type Asset struct {
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Size     int64  `json:"size_bytes"`
	SHA256   string `json:"sha256"`
}
type Manifest struct {
	Schema      int     `json:"schema_version"`
	Product     string  `json:"product_id"`
	Version     string  `json:"version"`
	Channel     string  `json:"channel"`
	Sequence    int64   `json:"release_sequence"`
	PublishedAt string  `json:"published_at"`
	Notes       string  `json:"notes"`
	Assets      []Asset `json:"assets"`
}
type Envelope struct {
	Manifest  string `json:"manifest_base64"`
	Signature string `json:"signature"`
	KeyID     string `json:"key_id"`
}

func VersionValid(v string) bool { return versionPattern.MatchString(v) }
func Newer(a, b string) bool {
	if !VersionValid(a) || !VersionValid(b) {
		return false
	}
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, e := strconv.ParseUint(aa[i], 10, 64)
		if e != nil {
			return false
		}
		y, e := strconv.ParseUint(bb[i], 10, 64)
		if e != nil {
			return false
		}
		if x != y {
			return x > y
		}
	}
	return false
}
func AssetURL(version, hash, name string) string {
	return PublicBase + DownloadPrefix + version + "/" + hash + "/" + name
}
func (m Manifest) Validate() error {
	if m.Schema != 1 || m.Product != Product || m.Channel != "stable" || !VersionValid(m.Version) || m.Sequence <= 0 || len(m.Assets) == 0 || len(m.Assets) > 3 || len(m.Notes) > 16000 {
		return errors.New("版本清单字段无效")
	}
	seen := map[string]bool{}
	for _, a := range m.Assets {
		target := a.Platform + "/" + a.Arch
		if seen[target] || !(target == "windows/x64" || target == "macos/universal" || target == "macos/arm64" || target == "macos/x64") {
			return errors.New("平台重复或不受支持")
		}
		seen[target] = true
		suffix := strings.ToLower(path.Ext(a.Name))
		if !filenamePattern.MatchString(a.Name) || strings.Contains(a.Name, "..") || a.Size <= 0 || a.Size > 2<<30 || len(a.SHA256) != 64 {
			return errors.New("安装包名称、大小或摘要无效")
		}
		if _, e := hex.DecodeString(a.SHA256); e != nil {
			return e
		}
		if (a.Platform == "macos" && suffix != ".dmg") || (a.Platform == "windows" && suffix != ".exe" && suffix != ".msi") {
			return errors.New("安装包类型不匹配")
		}
		u, e := url.Parse(a.URL)
		if e != nil || u.String() != AssetURL(m.Version, a.SHA256, a.Name) {
			return errors.New("安装包必须使用本站不可变下载链接")
		}
	}
	return nil
}
func Verify(e Envelope, public ed25519.PublicKey) (Manifest, error) {
	var m Manifest
	if e.KeyID != "fastaiapi-release-v1" {
		return m, errors.New("签名密钥不受信任")
	}
	data, err := base64.StdEncoding.DecodeString(e.Manifest)
	if err != nil || len(data) > 128*1024 {
		return m, errors.New("签名清单编码无效")
	}
	sig, err := base64.StdEncoding.DecodeString(e.Signature)
	if err != nil || !ed25519.Verify(public, data, sig) {
		return m, errors.New("发布签名无效")
	}
	if err = json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, m.Validate()
}
func ParsePublic(s string) (ed25519.PublicKey, error) {
	b, e := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if e != nil || len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("发布公钥无效")
	}
	return ed25519.PublicKey(b), nil
}
