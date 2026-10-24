package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sub-adds/backend/internal/clientupdates"
	"time"
)

func (a *app) clientUpdates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if a.updates == nil {
		failText(w, 503, "客户端更新服务尚未配置")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/client-updates/")
	if p == "v1/check" && r.Method == "GET" {
		if id := r.URL.Query().Get("product_id"); id != "" && id != clientupdates.Product {
			failText(w, 400, "产品标识无效")
			return
		}
		if ch := r.URL.Query().Get("channel"); ch != "" && ch != "stable" {
			failText(w, 400, "不支持的更新渠道")
			return
		}
		release, e := a.updates.Current(r.Context())
		if e == sql.ErrNoRows {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if e != nil {
			failText(w, 503, "版本检查暂不可用")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release.Envelope)
		return
	}
	if p == "public" && r.Method == "GET" {
		release, e := a.updates.Current(r.Context())
		if e == sql.ErrNoRows {
			success(w, map[string]any{"release": nil})
			return
		}
		if e != nil {
			failText(w, 503, "下载列表暂不可用")
			return
		}
		success(w, map[string]any{"release": release})
		return
	}
	uid, ok := a.userID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	admin := a.updates.IsAdmin(ctx, uid)
	if p == "access" && r.Method == "GET" {
		success(w, map[string]bool{"is_admin": admin})
		return
	}
	if !strings.HasPrefix(p, "admin/releases") {
		http.NotFound(w, r)
		return
	}
	if !admin {
		failText(w, 403, "需要有效管理员权限")
		return
	}
	result := func(v any, e error) {
		if e != nil {
			failText(w, 400, "发布操作失败："+e.Error())
			return
		}
		success(w, v)
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 2 {
		switch r.Method {
		case "GET":
			v, e := a.updates.List(ctx)
			result(v, e)
		case "POST":
			var in struct {
				Version  string   `json:"version"`
				Notes    string   `json:"notes"`
				Required []string `json:"required_targets"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 32768)
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				failText(w, 400, "版本信息无效")
				return
			}
			v, e := a.updates.Create(ctx, uid, in.Version, in.Notes, in.Required)
			result(v, e)
		default:
			w.WriteHeader(405)
		}
		return
	}
	if len(parts) != 4 {
		http.NotFound(w, r)
		return
	}
	id, e := strconv.ParseInt(parts[2], 10, 64)
	if e != nil || id <= 0 {
		failText(w, 400, "版本编号无效")
		return
	}
	switch parts[3] {
	case "assets":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
		v, e := a.updates.Upload(ctx, id, uid, r.URL.Query().Get("platform"), r.URL.Query().Get("arch"), r.URL.Query().Get("name"), r.Body)
		result(v, e)
	case "manifest":
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		v, e := a.updates.Get(ctx, id)
		if e != nil {
			result(nil, e)
			return
		}
		m := clientupdates.DraftManifest(v)
		if e = m.Validate(); e != nil {
			result(nil, e)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="client-release-manifest.json"`)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	case "validate":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var env clientupdates.Envelope
		r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
		if json.NewDecoder(r.Body).Decode(&env) != nil {
			failText(w, 400, "签名清单无效")
			return
		}
		v, e := a.updates.Validate(ctx, id, uid, env)
		result(v, e)
	case "publish":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		e := a.updates.Publish(ctx, id, uid)
		result(map[string]bool{"published": e == nil}, e)
	case "withdraw":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			failText(w, 400, "请填写撤回原因")
			return
		}
		e := a.updates.Withdraw(ctx, id, uid, in.Reason)
		result(map[string]bool{"withdrawn": e == nil}, e)
	default:
		http.NotFound(w, r)
	}
}
func (a *app) clientDownload(w http.ResponseWriter, r *http.Request) {
	if a.updates == nil {
		failText(w, 503, "下载服务尚未配置")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/downloads/codexplus/"), "/")
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	file, e := a.updates.Download(r.Context(), parts[0], parts[1], parts[2])
	if e != nil {
		http.NotFound(w, r)
		return
	}
	f, e := os.Open(file)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		http.NotFound(w, r)
		return
	}
	// 每次请求先查发布状态，撤回不会被代理长缓存继续分发。
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", `"`+parts[1]+`"`)
	w.Header().Set("Content-Disposition", `attachment; filename="`+parts[2]+`"`)
	http.ServeContent(w, r, parts[2], stat.ModTime(), f)
}
