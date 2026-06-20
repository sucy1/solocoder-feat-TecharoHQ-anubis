package adaptivedifficulty

import (
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestConfigValid(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				MinDifficulty:     1,
				MaxDifficulty:     64,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
				RecalcInterval:    60 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "MinDifficulty less than 1",
			config: Config{
				MinDifficulty:     0,
				MaxDifficulty:     64,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
				RecalcInterval:    60 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "MaxDifficulty greater than 100",
			config: Config{
				MinDifficulty:     1,
				MaxDifficulty:     101,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
				RecalcInterval:    60 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "MinDifficulty greater than MaxDifficulty",
			config: Config{
				MinDifficulty:     50,
				MaxDifficulty:     10,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
				RecalcInterval:    60 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "TargetCPULoad less than or equal to 0",
			config: Config{
				MinDifficulty:     1,
				MaxDifficulty:     64,
				TargetCPULoad:     0,
				TargetConnections: 1000,
				RecalcInterval:    60 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "TargetConnections less than or equal to 0",
			config: Config{
				MinDifficulty:     1,
				MaxDifficulty:     64,
				TargetCPULoad:     0.7,
				TargetConnections: 0,
				RecalcInterval:    60 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "RecalcInterval less than or equal to 0",
			config: Config{
				MinDifficulty:     1,
				MaxDifficulty:     64,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
				RecalcInterval:    0,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Valid()
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if !errors.Is(err, ErrInvalidConfig) {
					t.Errorf("expected error to wrap ErrInvalidConfig, got: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestNew(t *testing.T) {
	cfg := Config{}
	ad := New(cfg, slog.Default())

	if ad.config.MinDifficulty != 1 {
		t.Errorf("expected MinDifficulty default 1, got %d", ad.config.MinDifficulty)
	}
	if ad.config.MaxDifficulty != 64 {
		t.Errorf("expected MaxDifficulty default 64, got %d", ad.config.MaxDifficulty)
	}
	if ad.config.TargetCPULoad != 0.7 {
		t.Errorf("expected TargetCPULoad default 0.7, got %f", ad.config.TargetCPULoad)
	}
	if ad.config.TargetConnections != 1000 {
		t.Errorf("expected TargetConnections default 1000, got %d", ad.config.TargetConnections)
	}
	if ad.config.RecalcInterval != 60*time.Second {
		t.Errorf("expected RecalcInterval default 60s, got %v", ad.config.RecalcInterval)
	}
	if ad.CurrentDifficulty() != ad.config.MinDifficulty {
		t.Errorf("expected initial difficulty %d, got %d", ad.config.MinDifficulty, ad.CurrentDifficulty())
	}
}

func TestCurrentDifficulty(t *testing.T) {
	cfg := Config{
		MinDifficulty:     5,
		MaxDifficulty:     64,
		TargetCPULoad:     0.7,
		TargetConnections: 1000,
		RecalcInterval:    60 * time.Second,
	}
	ad := New(cfg, slog.Default())

	if ad.CurrentDifficulty() != 5 {
		t.Errorf("expected current difficulty 5, got %d", ad.CurrentDifficulty())
	}
}

func TestIncDecConnections(t *testing.T) {
	cfg := Config{
		MinDifficulty:     1,
		MaxDifficulty:     64,
		TargetCPULoad:     0.7,
		TargetConnections: 1000,
		RecalcInterval:    60 * time.Second,
	}
	ad := New(cfg, slog.Default())

	ad.IncConnections()
	ad.IncConnections()
	ad.IncConnections()

	ad.DecConnections()
}

func TestStop(t *testing.T) {
	cfg := Config{
		MinDifficulty:     1,
		MaxDifficulty:     64,
		TargetCPULoad:     0.7,
		TargetConnections: 1000,
		RecalcInterval:    60 * time.Second,
	}
	ad := New(cfg, slog.Default())

	done := make(chan struct{})
	go func() {
		ad.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop blocked for longer than expected")
	}
}

func TestNew_Defaults_MaxStep_Smoothing(t *testing.T) {
	cfg := Config{}
	ad := New(cfg, slog.Default())

	if ad.config.MaxStep <= 0 {
		t.Errorf("expected MaxStep default > 0, got %d", ad.config.MaxStep)
	}
	if ad.config.MaxStep > 10 {
		t.Errorf("expected MaxStep default <= 10, got %d", ad.config.MaxStep)
	}
	if ad.config.SmoothingFactor <= 0 || ad.config.SmoothingFactor > 1 {
		t.Errorf("expected SmoothingFactor default in (0,1], got %f", ad.config.SmoothingFactor)
	}
}

func TestConfigValid_MaxStep_Smoothing(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid with max_step and smoothing",
			config: Config{
				MinDifficulty:     1,
				MaxDifficulty:     64,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
				RecalcInterval:    60 * time.Second,
				MaxStep:           5,
				SmoothingFactor:   0.5,
			},
			wantErr: false,
		},
		{
			name: "negative MaxStep invalid",
			config: Config{
				MinDifficulty:     1,
				MaxDifficulty:     64,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
				RecalcInterval:    60 * time.Second,
				MaxStep:           -1,
			},
			wantErr: true,
		},
		{
			name: "SmoothingFactor negative invalid",
			config: Config{
				MinDifficulty:     1,
				MaxDifficulty:     64,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
				RecalcInterval:    60 * time.Second,
				SmoothingFactor:   -0.1,
			},
			wantErr: true,
		},
		{
			name: "SmoothingFactor > 1 invalid",
			config: Config{
				MinDifficulty:     1,
				MaxDifficulty:     64,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
				RecalcInterval:    60 * time.Second,
				SmoothingFactor:   1.5,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Valid()
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestRecalculate_StepLimit(t *testing.T) {
	cfg := Config{
		MinDifficulty:     1,
		MaxDifficulty:     64,
		TargetCPULoad:     0.7,
		TargetConnections: 1000,
		RecalcInterval:    60 * time.Second,
		MaxStep:           3,
		SmoothingFactor:   1.0,
	}
	ad := New(cfg, slog.Default())

	ad.mu.Lock()
	ad.currentDifficulty = 10
	ad.mu.Unlock()

	ad.RecalculateWith(100.0, 1000000)

	after := ad.CurrentDifficulty()
	delta := after - 10
	if delta <= 0 {
		t.Fatalf("difficulty should have risen, old=%d new=%d", 10, after)
	}
	if delta > cfg.MaxStep {
		t.Fatalf("difficulty jumped by %d, exceeding MaxStep=%d (old=10 new=%d). rawTarget would be way above so step-limit must cap.", delta, cfg.MaxStep, after)
	}
	t.Logf("one-shot step: %d -> %d (delta=%d, MaxStep=%d)", 10, after, delta, cfg.MaxStep)
}

func TestRecalculate_GradualConvergence(t *testing.T) {
	cfg := Config{
		MinDifficulty:     1,
		MaxDifficulty:     64,
		TargetCPULoad:     0.7,
		TargetConnections: 1000,
		RecalcInterval:    60 * time.Second,
		MaxStep:           3,
		SmoothingFactor:   1.0,
	}
	ad := New(cfg, slog.Default())
	ad.mu.Lock()
	ad.currentDifficulty = 5
	ad.mu.Unlock()

	oldDiff := 5
	for i := 0; i < 10; i++ {
		ad.RecalculateWith(100.0, 1000000)
		newDiff := ad.CurrentDifficulty()
		delta := newDiff - oldDiff
		if delta < 0 {
			t.Fatalf("iteration %d: difficulty went backwards (%d -> %d) under high load", i, oldDiff, newDiff)
		}
		if delta > cfg.MaxStep {
			t.Fatalf("iteration %d: step %d > MaxStep %d (old=%d new=%d)", i, delta, cfg.MaxStep, oldDiff, newDiff)
		}
		oldDiff = newDiff
	}
	if oldDiff <= 5 {
		t.Fatalf("difficulty should have climbed significantly after 10 high-load recalculations, got %d", oldDiff)
	}
	t.Logf("convergence: started at 5, after 10 steps = %d", oldDiff)
}

func TestRecalculate_SmoothingApplied(t *testing.T) {
	cfg := Config{
		MinDifficulty:     1,
		MaxDifficulty:     100,
		TargetCPULoad:     1.0,
		TargetConnections: 1,
		RecalcInterval:    60 * time.Second,
		MaxStep:           99,
		SmoothingFactor:   0.5,
	}
	ad := New(cfg, slog.Default())
	ad.mu.Lock()
	ad.currentDifficulty = 10
	ad.mu.Unlock()

	ad.RecalculateWith(50.0, 1)

	after := ad.CurrentDifficulty()
	baseDiff := 1
	rawTarget := baseDiff + int(float64(baseDiff)*(50.0/1.0)) + int(float64(baseDiff)*(1.0/1.0))
	if rawTarget > 100 {
		rawTarget = 100
	}
	expectedSmoothed := int(float64(10)*(1-cfg.SmoothingFactor)+float64(rawTarget)*cfg.SmoothingFactor + 0.5)
	if after != expectedSmoothed {
		t.Errorf("smoothing mismatch: got %d, expected smoothed %d (rawTarget=%d, old=10, alpha=0.5)", after, expectedSmoothed, rawTarget)
	}
	t.Logf("smoothing: raw=%d smoothed=%d (expected=%d)", rawTarget, after, expectedSmoothed)
}

func TestRecalculate_DownwardStepLimit(t *testing.T) {
	cfg := Config{
		MinDifficulty:     1,
		MaxDifficulty:     100,
		TargetCPULoad:     1.0,
		TargetConnections: 1000,
		RecalcInterval:    60 * time.Second,
		MaxStep:           4,
		SmoothingFactor:   1.0,
	}
	ad := New(cfg, slog.Default())
	ad.mu.Lock()
	ad.currentDifficulty = 90
	ad.mu.Unlock()

	ad.RecalculateWith(0.0001, 0)

	after := ad.CurrentDifficulty()
	drop := 90 - after
	if drop > cfg.MaxStep {
		t.Fatalf("downward jump %d exceeds MaxStep=%d (old=90 new=%d)", drop, cfg.MaxStep, after)
	}
	if drop <= 0 {
		t.Fatalf("difficulty should drop under zero load, old=90 new=%d", after)
	}
	t.Logf("downward step: 90 -> %d (drop=%d, MaxStep=%d)", after, drop, cfg.MaxStep)
}
