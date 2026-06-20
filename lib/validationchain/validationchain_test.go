package validationchain

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStepValid(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr error
	}{
		{
			name:    "valid pow step",
			step:    Step{Type: StepTypePoW, Config: map[string]any{}, Enabled: true},
			wantErr: nil,
		},
		{
			name:    "valid hcaptcha step with secret",
			step:    Step{Type: StepTypeHCaptcha, Config: map[string]any{"secret": "s"}, Enabled: true},
			wantErr: nil,
		},
		{
			name:    "valid ip_reputation step with deny_list",
			step:    Step{Type: StepTypeIPReputation, Config: map[string]any{"deny_list": []string{"10.0.0.1"}}, Enabled: true},
			wantErr: nil,
		},
		{
			name:    "unknown step type",
			step:    Step{Type: StepType("unknown"), Config: map[string]any{}, Enabled: true},
			wantErr: ErrUnknownStepType,
		},
		{
			name:    "hcaptcha without secret",
			step:    Step{Type: StepTypeHCaptcha, Config: map[string]any{}, Enabled: true},
			wantErr: ErrMissingConfig,
		},
		{
			name:    "ip_reputation without deny_list",
			step:    Step{Type: StepTypeIPReputation, Config: map[string]any{}, Enabled: true},
			wantErr: ErrMissingConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.step.Valid()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Step.Valid() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("Step.Valid() unexpected error = %v", err)
			}
		})
	}
}

func TestChainValid(t *testing.T) {
	tests := []struct {
		name    string
		chain   Chain
		wantErr bool
	}{
		{
			name:    "empty chain valid",
			chain:   Chain{Steps: []Step{}},
			wantErr: false,
		},
		{
			name: "chain with all valid steps",
			chain: Chain{Steps: []Step{
				{Type: StepTypePoW, Config: map[string]any{}, Enabled: true},
				{Type: StepTypeHCaptcha, Config: map[string]any{"secret": "s"}, Enabled: true},
			}},
			wantErr: false,
		},
		{
			name: "chain with invalid step",
			chain: Chain{Steps: []Step{
				{Type: StepType("unknown"), Config: map[string]any{}, Enabled: true},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.chain.Valid()
			if (err != nil) != tt.wantErr {
				t.Errorf("Chain.Valid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestChainValidate_PoWPass(t *testing.T) {
	chain := NewChain([]Step{
		{Type: StepTypePoW, Config: map[string]any{}, Enabled: true},
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	result := chain.Validate(context.Background(), req, slog.Default())
	if !result.Passed {
		t.Errorf("expected Passed=true, got false")
	}
}

func TestChainValidate_HCaptchaFails(t *testing.T) {
	chain := NewChain([]Step{
		{Type: StepTypeHCaptcha, Config: map[string]any{"secret": "test"}, Enabled: true},
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	result := chain.Validate(context.Background(), req, slog.Default())
	if result.Passed {
		t.Errorf("expected Passed=false, got true")
	}
	if result.FailedStep != "hcaptcha" {
		t.Errorf("expected FailedStep=hcaptcha, got %s", result.FailedStep)
	}
}

func TestChainValidate_IPReputationFails(t *testing.T) {
	chain := NewChain([]Step{
		{Type: StepTypeIPReputation, Config: map[string]any{"deny_list": []string{"10.0.0.1"}}, Enabled: true},
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-Ip", "10.0.0.1")
	result := chain.Validate(context.Background(), req, slog.Default())
	if result.Passed {
		t.Errorf("expected Passed=false, got true")
	}
	if result.FailedStep != "ip_reputation" {
		t.Errorf("expected FailedStep=ip_reputation, got %s", result.FailedStep)
	}
}

func TestChainValidate_IPReputationPass(t *testing.T) {
	chain := NewChain([]Step{
		{Type: StepTypeIPReputation, Config: map[string]any{"deny_list": []string{"10.0.0.1"}}, Enabled: true},
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-Ip", "192.168.1.1")
	result := chain.Validate(context.Background(), req, slog.Default())
	if !result.Passed {
		t.Errorf("expected Passed=true, got false")
	}
}

func TestChainValidate_DisabledStepSkipped(t *testing.T) {
	chain := NewChain([]Step{
		{Type: StepTypeHCaptcha, Config: map[string]any{"secret": "test"}, Enabled: false},
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	result := chain.Validate(context.Background(), req, slog.Default())
	if !result.Passed {
		t.Errorf("expected Passed=true, got false")
	}
}

func TestChainValidate_EarlyTermination(t *testing.T) {
	chain := NewChain([]Step{
		{Type: StepTypeIPReputation, Config: map[string]any{"deny_list": []string{"10.0.0.1"}}, Enabled: true},
		{Type: StepTypePoW, Config: map[string]any{}, Enabled: true},
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-Ip", "10.0.0.1")
	result := chain.Validate(context.Background(), req, slog.Default())
	if result.Passed {
		t.Errorf("expected Passed=false, got true")
	}
	if result.FailedStep != "ip_reputation" {
		t.Errorf("expected FailedStep=ip_reputation, got %s", result.FailedStep)
	}
}

func TestNewChain(t *testing.T) {
	steps := []Step{
		{Type: StepTypePoW, Config: map[string]any{}, Enabled: true},
		{Type: StepTypeHCaptcha, Config: map[string]any{"secret": "s"}, Enabled: false},
	}
	chain := NewChain(steps)
	if chain == nil {
		t.Fatal("NewChain returned nil")
	}
	if len(chain.Steps) != len(steps) {
		t.Errorf("expected %d steps, got %d", len(steps), len(chain.Steps))
	}
	for i, s := range chain.Steps {
		if s.Type != steps[i].Type {
			t.Errorf("step %d: expected Type=%s, got %s", i, steps[i].Type, s.Type)
		}
		if s.Enabled != steps[i].Enabled {
			t.Errorf("step %d: expected Enabled=%v, got %v", i, steps[i].Enabled, s.Enabled)
		}
	}
}
