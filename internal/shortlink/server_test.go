package shortlink

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterHealthCheck(t *testing.T) {
	mux := http.NewServeMux()
	registerHealthCheck(mux, "/ping")

	t.Run("GET", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body != "pong" {
			t.Fatalf("body = %q, want %q", body, "pong")
		}
	})

	t.Run("HEAD", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodHead, "/ping", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("body len = %d, want 0", rec.Body.Len())
		}
	})

	t.Run("POST", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/ping", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})
}
