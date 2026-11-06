package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateOriginBoundary(t *testing.T) {
	for _, tc := range []struct {
		path, method, origin string
		status               int
		allowed              string
	}{
		{"/api/client-updates/v1/check", "GET", "app://studio", 200, "*"},
		{"/downloads/manju/file.zip", "HEAD", "https://example.invalid", 200, "*"},
		{"/api/client-updates/admin/releases", "POST", "https://evil.invalid", 403, ""},
		{"/api/client-updates/admin/releases", "OPTIONS", "https://fastaiapi.cloud", 204, "https://fastaiapi.cloud"},
		{"/api/client-updates/admin/releases", "POST", "https://fastaiapi.cloud", 200, "https://fastaiapi.cloud"},
	} {
		t.Run(tc.method+tc.path+tc.origin, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			updateCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, r)
			if w.Code != tc.status || w.Header().Get("Access-Control-Allow-Origin") != tc.allowed {
				t.Fatalf("got %d origin %q", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}
