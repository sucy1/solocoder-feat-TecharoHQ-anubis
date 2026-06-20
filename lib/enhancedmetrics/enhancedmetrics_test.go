package enhancedmetrics

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewCollector_NoConfig(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	if c == nil {
		t.Fatal("expected non-nil collector")
	}
}

func TestNewCollector_WithToken(t *testing.T) {
	c := NewCollector(&MetricsConfig{
		MetricsToken: &MetricsTokenConfig{Token: "test-token"},
	}, slog.Default())
	if c == nil {
		t.Fatal("expected non-nil collector")
	}
}

func TestIncDecQueue(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	c.IncQueue()
	c.IncQueue()
	c.IncQueue()
	c.DecQueue()
	c.DecQueue()
	c.DecQueue()
	c.DecQueue()
}

func TestMiddleware_NoToken(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	c.Middleware(handler).ServeHTTP(rec, req)
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestMiddleware_ValidToken(t *testing.T) {
	c := NewCollector(&MetricsConfig{
		MetricsToken: &MetricsTokenConfig{Token: "secret"},
	}, slog.Default())
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("X-Anubis-Metrics-Token", "secret")
	rec := httptest.NewRecorder()
	c.Middleware(handler).ServeHTTP(rec, req)
	if !called {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddleware_InvalidToken(t *testing.T) {
	c := NewCollector(&MetricsConfig{
		MetricsToken: &MetricsTokenConfig{Token: "secret"},
	}, slog.Default())
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("X-Anubis-Metrics-Token", "wrong")
	rec := httptest.NewRecorder()
	c.Middleware(handler).ServeHTTP(rec, req)
	if called {
		t.Error("expected handler not to be called")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestMiddleware_QueryParamToken(t *testing.T) {
	c := NewCollector(&MetricsConfig{
		MetricsToken: &MetricsTokenConfig{Token: "secret"},
	}, slog.Default())
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	req := httptest.NewRequest(http.MethodGet, "/metrics?metrics_token=secret", nil)
	rec := httptest.NewRecorder()
	c.Middleware(handler).ServeHTTP(rec, req)
	if !called {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestSetLoad(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	c.SetLoad(1.5)
}

func TestSetSessionCacheSize(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	c.SetSessionCacheSize(42)
}

func TestSetAdaptiveDifficulty(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	c.SetAdaptiveDifficulty(10)
}

func TestRecordPoWPass(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	c.RecordPoWPass("fast")
}

func TestRecordPoWReject(t *testing.T) {
	c := NewCollector(nil, slog.Default())
	c.RecordPoWReject("fast", "failed")
}
