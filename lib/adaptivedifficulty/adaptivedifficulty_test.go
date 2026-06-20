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
