package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func serveUpdateServer(handler http.Handler) error {
	server := &http.Server{Addr: ":8080", Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop.Done():
		ctx, done := context.WithTimeout(context.Background(), 30*time.Minute)
		defer done()
		return server.Shutdown(ctx)
	}
}

// The independent update origin accepts bearer-authenticated administration
// from the existing official panel. Public update checks never need cookies.
func updateCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		public := (r.Method == "GET" || r.Method == "HEAD") && (strings.HasPrefix(r.URL.Path, "/downloads/manju/") || r.URL.Path == "/api/client-updates/v1/check" || r.URL.Path == "/api/client-updates/public" || r.URL.Path == "/health")
		origin := r.Header.Get("Origin")
		if public {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			if origin != "https://fastaiapi.cloud" && origin != "https://www.fastaiapi.cloud" && origin != "https://updates.fastaiapi.cloud" {
				failText(w, http.StatusForbidden, "管理页面来源不受信任")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Content-Range")
		w.Header().Set("Access-Control-Allow-Methods", "GET,HEAD,POST,OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
