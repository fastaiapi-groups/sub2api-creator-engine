package clientupdates

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
)

var rendererPath = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
var windowsReserved = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)`)

func rendererNameValid(name string) bool {
	if !rendererPath.MatchString(name) || path.Clean(name) != name || strings.HasPrefix(name, "/") || len(name) > 240 {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || strings.HasSuffix(part, ".") || windowsReserved.MatchString(part) {
			return false
		}
	}
	return name == "index.html" || name == "renderer-update.json" || name == "THIRD_PARTY_LICENSES.txt" || strings.HasPrefix(name, "assets/")
}

func inspectRendererBundle(filename, version string) (string, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return "", err
	}
	if info.Size() > 64<<20 {
		return "", errors.New("页面更新包超过 64 MB")
	}
	z, err := zip.OpenReader(filename)
	if err != nil {
		return "", errors.New("页面更新包不是有效 ZIP")
	}
	defer z.Close()
	if len(z.File) == 0 || len(z.File) > 4096 {
		return "", errors.New("页面更新包文件数量无效")
	}
	type fileDigest struct {
		Size   int64  `json:"size_bytes"`
		SHA256 string `json:"sha256"`
	}
	actual := map[string]fileDigest{}
	seen := map[string]bool{}
	var metadata []byte
	var total uint64
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		folded := strings.ToLower(f.Name)
		if !rendererNameValid(f.Name) || seen[folded] || f.Mode().Type() != 0 || f.Flags&1 != 0 || f.UncompressedSize64 > 64<<20 {
			return "", errors.New("页面更新包包含非法路径、链接、重复文件或超大文件")
		}
		seen[folded] = true
		total += f.UncompressedSize64
		if total > 256<<20 {
			return "", errors.New("页面更新包解压后超过 256 MB")
		}
		r, err := f.Open()
		if err != nil {
			return "", err
		}
		if f.Name == "renderer-update.json" {
			metadata, err = io.ReadAll(io.LimitReader(r, (1<<20)+1))
			r.Close()
			if err != nil || len(metadata) > 1<<20 {
				return "", errors.New("页面清单无效或过大")
			}
			continue
		}
		h := sha256.New()
		n, copyErr := io.Copy(h, io.LimitReader(r, (64<<20)+1))
		r.Close()
		if copyErr != nil || n != int64(f.UncompressedSize64) {
			return "", errors.New("页面更新包内容损坏")
		}
		actual[f.Name] = fileDigest{Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}
	}
	var bundle struct {
		Schema      int                   `json:"schema_version"`
		AppID       string                `json:"app_id"`
		Version     string                `json:"version"`
		RuntimeHash string                `json:"runtime_sha256"`
		Files       map[string]fileDigest `json:"files"`
	}
	if json.Unmarshal(metadata, &bundle) != nil || bundle.Schema != 1 || bundle.AppID != ManjuAppID || bundle.Version != version || len(bundle.RuntimeHash) != 64 || len(bundle.Files) != len(actual) {
		return "", errors.New("页面更新包的产品、版本或清单不匹配")
	}
	if _, err := hex.DecodeString(bundle.RuntimeHash); err != nil {
		return "", errors.New("页面更新包的桌面摘要无效")
	}
	if _, ok := actual["index.html"]; !ok {
		return "", errors.New("页面更新包缺少 index.html")
	}
	for name, digest := range actual {
		if bundle.Files[name] != digest {
			return "", errors.New("页面更新包文件摘要不匹配")
		}
	}
	return bundle.RuntimeHash, nil
}
