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
const ManjuProduct = "xiaoxiao-manju"
const ManjuAppID = "com.fastaiapi.xiaoxiao.manju"
const ManjuKeyID = "xiaoxiao-manju-release-v1"
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
	AppID       string  `json:"app_id,omitempty"`
	Mandatory   bool    `json:"mandatory,omitempty"`
	RuntimeHash string  `json:"runtime_sha256,omitempty"`
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
	return ProductAssetURL(Product, version, hash, name)
}
func ProductAssetURL(product, version, hash, name string) string {
	prefix := DownloadPrefix
	origin := PublicBase
	if product == ManjuProduct {
		origin = PublicBase
		prefix = "/fastaiapi/manju-updates/downloads/manju/"
	}
	return origin + prefix + version + "/" + hash + "/" + name
}
func (m Manifest) Validate() error {
	manju := m.Product == ManjuProduct && m.Schema == 2 && m.AppID == ManjuAppID
	if !(m.Schema == 1 && m.Product == Product) && !manju {
		return errors.New("更新产品或协议版本不匹配")
	}
	if !manju && (m.AppID != "" || m.Mandatory || m.RuntimeHash != "") {
		return errors.New("当前产品不支持这些更新选项")
	}
	if m.Channel != "stable" || !VersionValid(m.Version) || m.Sequence <= 0 || len(m.Assets) == 0 || len(m.Assets) > 3 || len(m.Notes) > 16000 {
		return errors.New("版本清单字段无效")
	}
	seen := map[string]bool{}
	for _, a := range m.Assets {
		target := a.Platform + "/" + a.Arch
		renderer := manju && target == "renderer/any"
		if seen[target] || !(target == "windows/x64" || target == "macos/universal" || target == "macos/arm64" || target == "macos/x64" || renderer) {
			return errors.New("平台重复或不受支持")
		}
		seen[target] = true
		if renderer {
			if len(m.RuntimeHash) != 64 || a.Size > 64<<20 {
				return errors.New("页面更新包缺少桌面兼容摘要或超过 64 MB")
			}
			if _, e := hex.DecodeString(m.RuntimeHash); e != nil {
				return errors.New("桌面兼容摘要无效")
			}
		}
		suffix := strings.ToLower(path.Ext(a.Name))
		if !filenamePattern.MatchString(a.Name) || strings.Contains(a.Name, "..") || a.Size <= 0 || a.Size > 2<<30 || len(a.SHA256) != 64 {
			return errors.New("安装包名称、大小或摘要无效")
		}
		if _, e := hex.DecodeString(a.SHA256); e != nil {
			return e
		}
		if (a.Platform == "macos" && suffix != ".dmg") || (a.Platform == "windows" && suffix != ".exe" && suffix != ".msi") || (renderer && suffix != ".zip") {
			return errors.New("安装包类型不匹配")
		}
		u, e := url.Parse(a.URL)
		if e != nil || u.String() != ProductAssetURL(m.Product, m.Version, a.SHA256, a.Name) {
			return errors.New("安装包必须使用本站不可变下载链接")
		}
	}
	return nil
}
func Verify(e Envelope, public ed25519.PublicKey) (Manifest, error) {
	return VerifyProduct(e, public, Product)
}
func VerifyProduct(e Envelope, public ed25519.PublicKey, product string) (Manifest, error) {
	var m Manifest
	keyID := "fastaiapi-release-v1"
	if product == ManjuProduct {
		keyID = ManjuKeyID
	}
	if e.KeyID != keyID {
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
	if m.Product != product {
		return m, errors.New("签名清单的产品不匹配")
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
