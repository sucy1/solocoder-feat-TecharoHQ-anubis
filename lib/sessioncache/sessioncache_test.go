package sessioncache

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestConfigValid(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name: "valid config",
			cfg: Config{
				MaxEntries: 100,
				DefaultTTL: time.Minute,
				HMACKey:    "secret",
			},
			wantErr: nil,
		},
		{
			name: "maxEntries zero",
			cfg: Config{
				MaxEntries: 0,
				DefaultTTL: time.Minute,
				HMACKey:    "secret",
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "maxEntries negative",
			cfg: Config{
				MaxEntries: -1,
				DefaultTTL: time.Minute,
				HMACKey:    "secret",
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "defaultTTL zero",
			cfg: Config{
				MaxEntries: 100,
				DefaultTTL: 0,
				HMACKey:    "secret",
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "defaultTTL negative",
			cfg: Config{
				MaxEntries: 100,
				DefaultTTL: -1 * time.Second,
				HMACKey:    "secret",
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "hmacKey empty",
			cfg: Config{
				MaxEntries: 100,
				DefaultTTL: time.Minute,
				HMACKey:    "",
			},
			wantErr: ErrMissingHMACKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Valid()
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Valid() = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Valid() = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestNew(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Minute,
		HMACKey:    "secret",
	}
	c, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if c == nil {
		t.Fatal("New() returned nil cache")
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Minute,
		HMACKey:    "",
	}
	_, err := New(cfg, slog.Default())
	if !errors.Is(err, ErrMissingHMACKey) {
		t.Errorf("New() error = %v, want ErrMissingHMACKey", err)
	}
}

func TestCreate(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Minute,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())

	sess := c.Create("192.168.1.1")

	if sess.Token == "" {
		t.Error("Create() Token is empty")
	}
	if sess.IP != "192.168.1.1" {
		t.Errorf("Create() IP = %q, want %q", sess.IP, "192.168.1.1")
	}
	if sess.CreatedAt.IsZero() {
		t.Error("Create() CreatedAt is zero")
	}
	if !sess.ExpiresAt.After(sess.CreatedAt) {
		t.Error("Create() ExpiresAt is not after CreatedAt")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Minute,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())

	sess := c.Create("10.0.0.1")
	got, ok := c.Validate(sess.Token)
	if !ok {
		t.Fatal("Validate() returned false for valid session")
	}
	if got.Token != sess.Token {
		t.Errorf("Validate() Token = %q, want %q", got.Token, sess.Token)
	}
	if got.IP != sess.IP {
		t.Errorf("Validate() IP = %q, want %q", got.IP, sess.IP)
	}
}

func TestValidate_Expired(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: 1 * time.Nanosecond,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())

	sess := c.Create("10.0.0.2")
	time.Sleep(10 * time.Millisecond)

	got, ok := c.Validate(sess.Token)
	if ok {
		t.Error("Validate() returned true for expired session")
	}
	if got != nil {
		t.Errorf("Validate() got = %v, want nil", got)
	}
}

func TestValidate_NotFound(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Minute,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())

	got, ok := c.Validate("nonexistent-token")
	if ok {
		t.Error("Validate() returned true for nonexistent token")
	}
	if got != nil {
		t.Errorf("Validate() got = %v, want nil", got)
	}
}

func TestRevoke(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Minute,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())

	sess := c.Create("10.0.0.3")
	revoked := c.Revoke(sess.Token)
	if !revoked {
		t.Error("Revoke() returned false for existing session")
	}

	_, ok := c.Validate(sess.Token)
	if ok {
		t.Error("Validate() returned true for revoked session")
	}
}

func TestRevoke_NotFound(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Minute,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())

	revoked := c.Revoke("nonexistent-token")
	if revoked {
		t.Error("Revoke() returned true for nonexistent token")
	}
}

func TestLRUEviction(t *testing.T) {
	cfg := Config{
		MaxEntries: 2,
		DefaultTTL: time.Minute,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())

	sess1 := c.Create("10.0.0.1")
	c.Create("10.0.0.2")
	c.Create("10.0.0.3")

	if c.Size() != 2 {
		t.Errorf("Size() = %d, want 2", c.Size())
	}

	_, ok := c.Validate(sess1.Token)
	if ok {
		t.Error("Validate() returned true for evicted session")
	}
}

func TestSize(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Minute,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())

	for i := 0; i < 5; i++ {
		c.Create("10.0.0.1")
	}

	if c.Size() != 5 {
		t.Errorf("Size() = %d, want 5", c.Size())
	}
}

func TestConfigValid_RotationInterval(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "rotation_interval zero defaults are OK at package-level",
			cfg: Config{
				MaxEntries: 100,
				DefaultTTL: time.Minute,
				HMACKey:    "secret",
			},
			wantErr: false,
		},
		{
			name: "rotation_interval negative invalid",
			cfg: Config{
				MaxEntries:       100,
				DefaultTTL:       time.Minute,
				HMACKey:          "secret",
				RotationInterval: -1 * time.Hour,
			},
			wantErr: true,
		},
		{
			name: "rotation_interval positive valid",
			cfg: Config{
				MaxEntries:       100,
				DefaultTTL:       time.Minute,
				HMACKey:          "secret",
				RotationInterval: 24 * time.Hour,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Valid()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestNew_DefaultRotationInterval(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Minute,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())
	if c.config.RotationInterval <= 0 {
		t.Errorf("expected RotationInterval default > 0, got %v", c.config.RotationInterval)
	}
}

func TestRotate_KeyID(t *testing.T) {
	cfg := Config{
		MaxEntries:       100,
		DefaultTTL:       time.Hour,
		HMACKey:          "secret",
		RotationInterval: 24 * time.Hour,
	}
	c, _ := New(cfg, slog.Default())

	sess := c.Create("10.0.0.1")
	if _, ok := c.Validate(sess.Token); !ok {
		t.Fatal("pre-rotation token should validate")
	}

	newKid := c.Rotate()
	if newKid < 0 {
		t.Fatalf("Rotate returned invalid key id %d", newKid)
	}

	if _, ok := c.Validate(sess.Token); !ok {
		t.Error("post-rotation old token should still validate via old key")
	}

	sess2 := c.Create("10.0.0.2")
	if sess2.Token == sess.Token {
		t.Error("new session after rotation must have different token")
	}

	var st2 signedToken
	b2, _ := hex.DecodeString(sess2.Token)
	json.Unmarshal(b2, &st2)
	if st2.KeyID != newKid {
		t.Errorf("new session signed with kid %d, want %d", st2.KeyID, newKid)
	}
}

func TestValidate_TamperedSignature(t *testing.T) {
	cfg := Config{
		MaxEntries: 100,
		DefaultTTL: time.Hour,
		HMACKey:    "secret",
	}
	c, _ := New(cfg, slog.Default())

	sess := c.Create("10.0.0.1")
	raw, _ := hex.DecodeString(sess.Token)
	var st signedToken
	json.Unmarshal(raw, &st)

	st.Sig = "0000000000000000000000000000000000000000000000000000000000000000"
	forgedBytes, _ := json.Marshal(st)
	forged := hex.EncodeToString(forgedBytes)

	if _, ok := c.Validate(forged); ok {
		t.Error("tampered signature should not validate")
	}
}
