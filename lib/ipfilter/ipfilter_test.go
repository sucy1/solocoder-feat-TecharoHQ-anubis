package ipfilter

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigValid(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid entries pass",
			config: Config{
				Entries: []Entry{
					{CIDR: "192.168.1.0/24", ListType: ListTypeWhitelist},
					{CIDR: "10.0.0.0/8", ListType: ListTypeBlacklist},
				},
			},
			wantErr: false,
		},
		{
			name: "empty CIDR fails",
			config: Config{
				Entries: []Entry{
					{CIDR: "", ListType: ListTypeWhitelist},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid list type fails",
			config: Config{
				Entries: []Entry{
					{CIDR: "192.168.1.0/24", ListType: "unknown"},
				},
			},
			wantErr: true,
		},
		{
			name: "valid CIDR notation passes",
			config: Config{
				Entries: []Entry{
					{CIDR: "10.0.0.0/8", ListType: ListTypeBlacklist},
				},
			},
			wantErr: false,
		},
		{
			name: "valid single IP passes",
			config: Config{
				Entries: []Entry{
					{CIDR: "192.168.1.1", ListType: ListTypeWhitelist},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Valid()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Valid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNew(t *testing.T) {
	cfg := Config{
		Entries: []Entry{
			{CIDR: "192.168.1.0/24", ListType: ListTypeWhitelist},
		},
	}
	f, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if f == nil {
		t.Fatal("New() returned nil filter")
	}
}

func TestNew_InvalidCIDR(t *testing.T) {
	cfg := Config{
		Entries: []Entry{
			{CIDR: "not-a-cidr", ListType: ListTypeWhitelist},
		},
	}
	_, err := New(cfg, slog.Default())
	if err == nil {
		t.Fatal("New() expected error for invalid CIDR, got nil")
	}
	if !errors.Is(err, ErrInvalidCIDR) {
		t.Errorf("New() error = %v, want ErrInvalidCIDR", err)
	}
}

func TestCheck_Whitelist(t *testing.T) {
	cfg := Config{
		Entries: []Entry{
			{CIDR: "192.168.1.0/24", ListType: ListTypeWhitelist},
		},
	}
	f, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	allow, reason := f.Check(net.ParseIP("192.168.1.100"))
	if !allow || reason != "whitelist" {
		t.Errorf("Check(192.168.1.100) = (%v, %q), want (true, \"whitelist\")", allow, reason)
	}

	allow, reason = f.Check(net.ParseIP("10.0.0.1"))
	if !allow || reason != "default" {
		t.Errorf("Check(10.0.0.1) = (%v, %q), want (true, \"default\")", allow, reason)
	}
}

func TestCheck_Blacklist(t *testing.T) {
	cfg := Config{
		Entries: []Entry{
			{CIDR: "10.0.0.0/8", ListType: ListTypeBlacklist},
		},
	}
	f, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	allow, reason := f.Check(net.ParseIP("10.0.0.1"))
	if allow || reason != "blacklist" {
		t.Errorf("Check(10.0.0.1) = (%v, %q), want (false, \"blacklist\")", allow, reason)
	}

	allow, reason = f.Check(net.ParseIP("192.168.1.1"))
	if !allow || reason != "default" {
		t.Errorf("Check(192.168.1.1) = (%v, %q), want (true, \"default\")", allow, reason)
	}
}

func TestCheck_WhitelistPriority(t *testing.T) {
	cfg := Config{
		Entries: []Entry{
			{CIDR: "192.168.1.1", ListType: ListTypeWhitelist},
			{CIDR: "192.168.1.0/24", ListType: ListTypeBlacklist},
		},
	}
	f, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	allow, reason := f.Check(net.ParseIP("192.168.1.1"))
	if !allow || reason != "whitelist" {
		t.Errorf("Check(192.168.1.1) = (%v, %q), want (true, \"whitelist\")", allow, reason)
	}
}

func TestReload(t *testing.T) {
	cfg := Config{
		Entries: []Entry{
			{CIDR: "10.0.0.0/8", ListType: ListTypeBlacklist},
		},
	}
	f, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	newCfg := Config{
		Entries: []Entry{
			{CIDR: "192.168.1.0/24", ListType: ListTypeWhitelist},
		},
	}
	if err := f.Reload(newCfg); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	allow, reason := f.Check(net.ParseIP("192.168.1.50"))
	if !allow || reason != "whitelist" {
		t.Errorf("after reload Check(192.168.1.50) = (%v, %q), want (true, \"whitelist\")", allow, reason)
	}

	allow, reason = f.Check(net.ParseIP("10.0.0.1"))
	if !allow || reason != "default" {
		t.Errorf("after reload Check(10.0.0.1) = (%v, %q), want (true, \"default\")", allow, reason)
	}
}

func TestReload_InvalidConfig(t *testing.T) {
	cfg := Config{
		Entries: []Entry{
			{CIDR: "192.168.1.0/24", ListType: ListTypeWhitelist},
		},
	}
	f, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	invalidCfg := Config{
		Entries: []Entry{
			{CIDR: "not-valid", ListType: ListTypeWhitelist},
		},
	}
	if err := f.Reload(invalidCfg); err == nil {
		t.Fatal("Reload() expected error for invalid config, got nil")
	}

	allow, reason := f.Check(net.ParseIP("192.168.1.1"))
	if !allow || reason != "whitelist" {
		t.Errorf("after failed reload Check(192.168.1.1) = (%v, %q), want (true, \"whitelist\")", allow, reason)
	}
}

func TestHTTPHandler(t *testing.T) {
	cfg := Config{
		Entries: []Entry{
			{CIDR: "192.168.1.0/24", ListType: ListTypeWhitelist},
			{CIDR: "10.0.0.0/8", ListType: ListTypeBlacklist},
		},
	}
	f, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	handler := f.HTTPHandler()

	t.Run("GET /entries returns 200 with JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/entries", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /entries status = %d, want %d", rec.Code, http.StatusOK)
		}

		var result Config
		if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
			t.Fatalf("decode response body: %v", err)
		}
		if len(result.Entries) != 2 {
			t.Errorf("entries count = %d, want 2", len(result.Entries))
		}
	})

	t.Run("POST /reload with valid JSON returns 200", func(t *testing.T) {
		newCfg := Config{
			Entries: []Entry{
				{CIDR: "172.16.0.0/12", ListType: ListTypeBlacklist},
			},
		}
		body, _ := json.Marshal(newCfg)
		req := httptest.NewRequest(http.MethodPost, "/reload", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("POST /reload status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("POST /reload with invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/reload", strings.NewReader("not-json"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST /reload invalid JSON status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

func TestSingleIP(t *testing.T) {
	tests := []struct {
		name       string
		entry      Entry
		testIP     string
		wantAllow  bool
		wantReason string
	}{
		{
			name:       "single IPv4 converted to /32",
			entry:      Entry{CIDR: "192.168.1.1", ListType: ListTypeWhitelist},
			testIP:     "192.168.1.1",
			wantAllow:  true,
			wantReason: "whitelist",
		},
		{
			name:       "single IPv4 /32 does not match different IP",
			entry:      Entry{CIDR: "192.168.1.1", ListType: ListTypeWhitelist},
			testIP:     "192.168.1.2",
			wantAllow:  true,
			wantReason: "default",
		},
		{
			name:       "single IPv6 converted to /128",
			entry:      Entry{CIDR: "::1", ListType: ListTypeBlacklist},
			testIP:     "::1",
			wantAllow:  false,
			wantReason: "blacklist",
		},
		{
			name:       "single IPv6 /128 does not match different IP",
			entry:      Entry{CIDR: "::1", ListType: ListTypeBlacklist},
			testIP:     "::2",
			wantAllow:  true,
			wantReason: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Entries: []Entry{tt.entry}}
			f, err := New(cfg, slog.Default())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			allow, reason := f.Check(net.ParseIP(tt.testIP))
			if allow != tt.wantAllow || reason != tt.wantReason {
				t.Errorf("Check(%s) = (%v, %q), want (%v, %q)", tt.testIP, allow, reason, tt.wantAllow, tt.wantReason)
			}
		})
	}
}
