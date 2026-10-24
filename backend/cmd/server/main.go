package main

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sub-adds/backend/internal/chatimage"
	"sub-adds/backend/internal/chatimage/gallery"
	"sub-adds/backend/internal/clientupdates"
	"sub-adds/backend/internal/recharge"
	"time"
)

const authMePath = "/api/v1/auth/me"

type app struct {
	db            *sql.DB
	base          string
	client        *http.Client
	conversations *chatimage.SQLRepository
	recharge      *recharge.Service
	gallery       *gallery.SQLRepository
	updates       *clientupdates.Service
}

func main() {
	db, e := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if e != nil {
		log.Fatal(e)
	}
	base := strings.TrimRight(os.Getenv("FASTAIAPI_BASE_URL"), "/")
	if base == "" {
		log.Fatal("FASTAIAPI_BASE_URL is required")
	}
	a := &app{db: db, base: base, client: &http.Client{Timeout: 10 * time.Minute}, conversations: chatimage.NewSQLRepository(db), gallery: gallery.NewSQLRepository(db)}
	if os.Getenv("RECHARGE_ENABLED") == "true" {
		cfg, err := recharge.LoadConfig()
		if err != nil {
			log.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		a.recharge, err = recharge.New(ctx, cfg)
		cancel()
		if err != nil {
			log.Fatalf("recharge initialization: %v", err)
		}
		defer a.recharge.Close()
		go a.recharge.Run(context.Background())
	}
	if os.Getenv("CLIENT_UPDATES_ENABLED") == "true" {
		public, err := clientupdates.ParsePublic(os.Getenv("CLIENT_UPDATES_PUBLIC_KEY"))
		if err != nil {
			log.Fatal(err)
		}
		admin, err := sql.Open("pgx", os.Getenv("SUB2API_DATABASE_URL"))
		if err != nil {
			log.Fatal(err)
		}
		defer admin.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		a.updates, err = clientupdates.New(ctx, db, admin, os.Getenv("CLIENT_UPDATES_DIR"), public)
		cancel()
		if err != nil {
			log.Fatal(err)
		}
	}
	m := http.NewServeMux()
	m.HandleFunc("/api/client-updates/", a.clientUpdates)
	m.HandleFunc("/downloads/codexplus/", a.clientDownload)
	m.HandleFunc("/api/recharge/", a.rechargeRoute)
	m.HandleFunc("/health", a.health)
	m.HandleFunc("/api/proxy/", a.proxy)
	m.HandleFunc("/api/chat-image", a.chatImage)
	m.HandleFunc("/api/chat-image/", a.chatImage)
	log.Fatal(http.ListenAndServe(":8080", cors(m)))
}
func cors(n http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		n.ServeHTTP(w, r)
	})
}
func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	if e := a.db.Ping(); e != nil {
		fail(w, 503, e)
		return
	}
	success(w, map[string]string{"status": "ok"})
}
func (a *app) proxy(w http.ResponseWriter, r *http.Request) {
	targets := map[string]string{"/api/proxy/me": authMePath, "/api/proxy/models": "/v1/models", "/api/proxy/chat": "/v1/chat/completions", "/api/proxy/images": "/v1/images/generations", "/api/proxy/image-edits": "/v1/images/edits"}
	t, ok := targets[r.URL.Path]
	if !ok && r.URL.Path == "/api/proxy/groups/available" {
		t, ok = "/api/v1/groups/available", true
	}
	if !ok && (r.URL.Path == "/api/proxy/keys" || strings.HasPrefix(r.URL.Path, "/api/proxy/keys/")) {
		t, ok = "/api/v1"+strings.TrimPrefix(r.URL.Path, "/api/proxy"), true
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.URL.RawQuery != "" {
		t += "?" + r.URL.RawQuery
	}
	a.forward(w, r, t)
}
func (a *app) forward(w http.ResponseWriter, r *http.Request, t string) {
	q, e := http.NewRequestWithContext(r.Context(), r.Method, a.base+t, r.Body)
	if e != nil {
		fail(w, 500, e)
		return
	}
	q.Header.Set("Authorization", r.Header.Get("Authorization"))
	q.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	p, e := a.client.Do(q)
	if e != nil {
		fail(w, 502, e)
		return
	}
	defer p.Body.Close()
	for k, v := range p.Header {
		for _, x := range v {
			w.Header().Add(k, x)
		}
	}
	w.WriteHeader(p.StatusCode)
	buffer := make([]byte, 16*1024)
	flusher, canFlush := w.(http.Flusher)
	for {
		n, readErr := p.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}
func (a *app) userID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	t := r.Header.Get("Authorization")
	if t == "" {
		failText(w, 401, "authorization required")
		return 0, false
	}
	q, _ := http.NewRequestWithContext(r.Context(), "GET", a.base+authMePath, nil)
	q.Header.Set("Authorization", t)
	p, e := a.client.Do(q)
	if e != nil {
		fail(w, 502, e)
		return 0, false
	}
	defer p.Body.Close()
	if p.StatusCode != 200 {
		w.WriteHeader(p.StatusCode)
		io.Copy(w, p.Body)
		return 0, false
	}
	var b map[string]any
	if json.NewDecoder(p.Body).Decode(&b) != nil {
		failText(w, 502, "invalid auth response")
		return 0, false
	}
	v := b["data"]
	if v == nil {
		v = b
	}
	m, _ := v.(map[string]any)
	id := toInt64(m["id"])
	if id == 0 {
		id = toInt64(m["user_id"])
	}
	if id == 0 {
		failText(w, 502, "auth response has no user id")
		return 0, false
	}
	return id, true
}
func (a *app) chatImage(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.userID(w, r)
	if !ok {
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/chat-image")
	if p == "/prompts" && r.Method == "GET" {
		v, e := chatimage.PromptLibrary()
		respond(w, v, e)
		return
	}
	if strings.HasPrefix(p, "/gallery/images") {
		a.galleryRoute(w, r, uid, p)
		return
	}
	if p == "/conversations" {
		a.collection(w, r, uid)
		return
	}
	x := strings.Split(strings.Trim(p, "/"), "/")
	if len(x) >= 2 && x[0] == "conversations" {
		id, e := strconv.ParseInt(x[1], 10, 64)
		if e != nil {
			failText(w, 400, "invalid conversation id")
			return
		}
		a.item(w, r, uid, id, x)
		return
	}
	http.NotFound(w, r)
}
func (a *app) collection(w http.ResponseWriter, r *http.Request, uid int64) {
	switch r.Method {
	case "GET":
		p, s := qi(r, "page", 1), qi(r, "page_size", 30)
		v, n, e := a.conversations.ListConversations(r.Context(), uid, p, s, r.URL.Query().Get("search"))
		if e != nil {
			fail(w, 500, e)
			return
		}
		success(w, map[string]any{"items": v, "total": n, "page": p, "page_size": s, "pages": (n + int64(s) - 1) / int64(s)})
	case "POST":
		var x struct {
			Title string `json:"title"`
		}
		json.NewDecoder(r.Body).Decode(&x)
		if strings.TrimSpace(x.Title) == "" {
			x.Title = "新对话"
		}
		v, e := a.conversations.CreateConversation(r.Context(), uid, x.Title)
		respond(w, v, e)
	default:
		method(w)
	}
}
func (a *app) item(w http.ResponseWriter, r *http.Request, uid, id int64, x []string) {
	if len(x) == 3 && x[2] == "messages" {
		if r.Method == "DELETE" {
			v, e := a.conversations.ClearConversation(r.Context(), uid, id)
			respond(w, v, e)
			return
		}
		if r.Method == "POST" {
			var in chatimage.MessageInput
			if json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(&in) != nil {
				failText(w, 400, "invalid body")
				return
			}
			v, e := a.conversations.AppendMessage(r.Context(), uid, id, in)
			respond(w, v, e)
			return
		}
	}
	switch r.Method {
	case "GET":
		v, e := a.conversations.GetConversation(r.Context(), uid, id)
		respond(w, v, e)
	case "PATCH":
		var in struct {
			Title string `json:"title"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		v, e := a.conversations.RenameConversation(r.Context(), uid, id, in.Title)
		respond(w, v, e)
	case "DELETE":
		e := a.conversations.DeleteConversation(r.Context(), uid, id)
		respond(w, map[string]bool{"deleted": e == nil}, e)
	default:
		method(w)
	}
}
func (a *app) galleryRoute(w http.ResponseWriter, r *http.Request, uid int64, p string) {
	if p == "/gallery/images" && r.Method == "GET" {
		pg, sz := qi(r, "page", 1), qi(r, "page_size", 24)
		v, n, e := a.gallery.ListImages(r.Context(), uid, pg, sz)
		if e != nil {
			fail(w, 500, e)
			return
		}
		success(w, map[string]any{"items": v, "total": n, "page": pg, "page_size": sz, "pages": (n + int64(sz) - 1) / int64(sz)})
		return
	}
	var in gallery.SelectedImagesRequest
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.IDs) == 0 {
		failText(w, 400, "ids required")
		return
	}
	if p == "/gallery/images" && r.Method == "DELETE" {
		v, e := a.gallery.DeleteImages(r.Context(), uid, in.IDs)
		respond(w, v, e)
		return
	}
	if p == "/gallery/images/download" && r.Method == "POST" {
		a.download(w, r.Context(), uid, in.IDs)
		return
	}
	method(w)
}
func (a *app) download(w http.ResponseWriter, c context.Context, uid int64, ids []int64) {
	v, e := a.gallery.GetImages(c, uid, ids)
	if e != nil || len(v) == 0 {
		if e == nil {
			e = fmt.Errorf("images not found")
		}
		fail(w, 404, e)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="images.zip"`)
	z := zip.NewWriter(w)
	defer z.Close()
	for i, x := range v {
		u, e := url.Parse(x.URL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		q, _ := http.NewRequestWithContext(c, "GET", u.String(), nil)
		p, e := a.client.Do(q)
		if e != nil {
			continue
		}
		n := x.DisplayName
		if n == "" {
			n = path.Base(u.Path)
		}
		if n == "" || n == "/" {
			n = fmt.Sprintf("image-%d", i+1)
		}
		f, _ := z.Create(fmt.Sprintf("%03d-%s", i+1, n))
		io.CopyN(f, p.Body, 64<<20)
		p.Body.Close()
	}
}
func success(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "success", "data": v})
}
func respond(w http.ResponseWriter, v any, e error) {
	if e != nil {
		fail(w, 400, e)
		return
	}
	success(w, v)
}
func fail(w http.ResponseWriter, s int, e error) { failText(w, s, e.Error()) }
func failText(w http.ResponseWriter, s int, t string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s)
	json.NewEncoder(w).Encode(map[string]any{"code": s, "message": t})
}
func method(w http.ResponseWriter) { failText(w, 405, "method not allowed") }
func qi(r *http.Request, k string, d int) int {
	v, e := strconv.Atoi(r.URL.Query().Get(k))
	if e != nil || v < 1 {
		return d
	}
	if v > 100 {
		return 100
	}
	return v
}
func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	case json.Number:
		n, _ := x.Int64()
		return n
	}
	return 0
}
