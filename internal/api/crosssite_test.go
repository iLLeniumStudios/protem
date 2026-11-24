package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func crossSiteServer(t *testing.T, devAuth bool) *Server {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).Build()
	s, err := NewServer(c, ServerOptions{Addr: "127.0.0.1:0", DevAuth: devAuth})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

func TestWritesMustBeJSONSoAFormOnAnotherSiteCannotForgeThem(t *testing.T) {
	s := crossSiteServer(t, true)
	post := func(contentType string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/namespaces/team/requests/r1/approve", strings.NewReader(`{"comment":"ok"}`))
		r.Header.Set("X-Test-User", bobEmail)
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	for _, formType := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x"} {
		if code := post(formType); code != http.StatusUnsupportedMediaType {
			t.Errorf("POST with Content-Type %q = %d, want 415", formType, code)
		}
	}
	if code := post("application/json; charset=utf-8"); code != http.StatusNotFound {
		t.Errorf("JSON POST for a missing request = %d, want it to reach the handler (404)", code)
	}
}

func TestOversizedBodyIsRefused(t *testing.T) {
	s := crossSiteServer(t, true)
	body := `{"profileRef":"p","reason":"` + strings.Repeat("a", 2<<20) + `"}`
	r := httptest.NewRequest(http.MethodPost, "/api/namespaces/team/requests", strings.NewReader(body))
	r.Header.Set("X-Test-User", aliceEmail)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("2 MiB body = %d, want 400", w.Code)
	}
}

func TestCORSOnlyForLoopbackOriginsInDevAuth(t *testing.T) {
	allowed := func(s *Server, origin string) string {
		r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Header().Get("Access-Control-Allow-Origin")
	}
	dev := crossSiteServer(t, true)
	for _, origin := range []string{"http://localhost:3000", "http://127.0.0.1:5173"} {
		if got := allowed(dev, origin); got != origin {
			t.Errorf("dev-auth: origin %q allowed as %q, want it echoed", origin, got)
		}
	}
	for _, origin := range []string{"http://localhost.evil.example", "http://127.0.0.1.evil.example", "https://evil.example"} {
		if got := allowed(dev, origin); got != "" {
			t.Errorf("dev-auth: origin %q allowed as %q, want no CORS header", origin, got)
		}
	}
	if got := allowed(crossSiteServer(t, false), "http://localhost:3000"); got != "" {
		t.Errorf("without dev-auth: origin allowed as %q, want no CORS header", got)
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/api/namespaces/x/requests/y/approve", nil)
	preflight.Header.Set("Origin", "http://localhost:5173")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	w := httptest.NewRecorder()
	dev.Handler().ServeHTTP(w, preflight)
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), http.MethodPost) {
		t.Errorf("dev-auth preflight = %d with methods %q, want 200 allowing POST", w.Code, w.Header().Get("Access-Control-Allow-Methods"))
	}
}
