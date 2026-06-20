package validationchain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

type StepType string

const (
	StepTypePoW          StepType = "pow"
	StepTypeHCaptcha     StepType = "hcaptcha"
	StepTypeIPReputation StepType = "ip_reputation"
)

var (
	ErrUnknownStepType = errors.New("validationchain: unknown step type")
	ErrStepFailed      = errors.New("validationchain: step failed")
	ErrMissingConfig   = errors.New("validationchain: missing required config")
)

type Step struct {
	Type    StepType       `json:"type" yaml:"type"`
	Config  map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
	Enabled bool           `json:"enabled" yaml:"enabled"`
}

func (s Step) Valid() error {
	switch s.Type {
	case StepTypePoW, StepTypeHCaptcha, StepTypeIPReputation:
	default:
		return fmt.Errorf("%w: %s", ErrUnknownStepType, s.Type)
	}

	switch s.Type {
	case StepTypeHCaptcha:
		if _, ok := s.Config["secret"]; !ok {
			return fmt.Errorf("%w: hcaptcha requires secret", ErrMissingConfig)
		}
	case StepTypeIPReputation:
		if _, ok := s.Config["deny_list"]; !ok {
			return fmt.Errorf("%w: ip_reputation requires deny_list", ErrMissingConfig)
		}
	}

	return nil
}

type Chain struct {
	Steps []Step `json:"steps" yaml:"steps"`
}

func (c Chain) Valid() error {
	var errs []error

	for i, s := range c.Steps {
		if err := s.Valid(); err != nil {
			errs = append(errs, fmt.Errorf("step %d: %w", i, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

type ChainResult struct {
	Passed     bool   `json:"passed" yaml:"passed"`
	FailedStep string `json:"failed_step,omitempty" yaml:"failed_step,omitempty"`
	Error      error  `json:"error,omitempty" yaml:"error,omitempty"`
}

func NewChain(steps []Step) *Chain {
	return &Chain{Steps: steps}
}

func (c *Chain) Validate(ctx context.Context, r *http.Request, lg *slog.Logger) ChainResult {
	for _, step := range c.Steps {
		if !step.Enabled {
			lg.Info("skipping disabled step", "type", string(step.Type))
			continue
		}

		lg.Info("validating step", "type", string(step.Type))

		var err error

		switch step.Type {
		case StepTypePoW:
			err = validatePoW(ctx, r, step.Config, lg)
		case StepTypeHCaptcha:
			err = validateHCaptcha(ctx, r, step.Config, lg)
		case StepTypeIPReputation:
			err = validateIPReputation(ctx, r, step.Config, lg)
		default:
			err = fmt.Errorf("%w: %s", ErrUnknownStepType, step.Type)
		}

		if err != nil {
			lg.Error("step failed", "type", string(step.Type), "error", err)
			return ChainResult{
				Passed:     false,
				FailedStep: string(step.Type),
				Error:      fmt.Errorf("%w: %s: %v", ErrStepFailed, step.Type, err),
			}
		}

		lg.Info("step passed", "type", string(step.Type))
	}

	return ChainResult{Passed: true}
}

func validatePoW(_ context.Context, _ *http.Request, _ map[string]any, _ *slog.Logger) error {
	return nil
}

func validateHCaptcha(_ context.Context, r *http.Request, config map[string]any, lg *slog.Logger) error {
	secret, ok := config["secret"]
	if !ok {
		lg.Error("hcaptcha secret missing from config")
		return fmt.Errorf("%w: hcaptcha secret", ErrMissingConfig)
	}

	if _, ok := secret.(string); !ok {
		lg.Error("hcaptcha secret is not a string")
		return fmt.Errorf("%w: hcaptcha secret must be a string", ErrMissingConfig)
	}

	response := r.FormValue("h-captcha-response")
	if response == "" {
		return fmt.Errorf("%w: h-captcha-response is empty", ErrStepFailed)
	}

	return nil
}

func validateIPReputation(_ context.Context, r *http.Request, config map[string]any, lg *slog.Logger) error {
	ip := r.Header.Get("X-Real-Ip")
	if ip == "" {
		lg.Warn("X-Real-Ip header missing")
		return nil
	}

	denyListVal, ok := config["deny_list"]
	if !ok {
		lg.Error("ip_reputation deny_list missing from config")
		return fmt.Errorf("%w: ip_reputation deny_list", ErrMissingConfig)
	}

	denyList, ok := denyListVal.([]string)
	if !ok {
		if ifaceList, ok := denyListVal.([]any); ok {
			denyList = make([]string, 0, len(ifaceList))
			for _, v := range ifaceList {
				s, ok := v.(string)
				if !ok {
					lg.Error("ip_reputation deny_list contains non-string entry")
					return fmt.Errorf("%w: ip_reputation deny_list must contain strings", ErrMissingConfig)
				}
				denyList = append(denyList, s)
			}
		} else {
			lg.Error("ip_reputation deny_list is not a string list")
			return fmt.Errorf("%w: ip_reputation deny_list must be a list of strings", ErrMissingConfig)
		}
	}

	for _, denied := range denyList {
		if ip == denied {
			lg.Info("ip found in deny list", "ip", ip)
			return fmt.Errorf("%w: ip %s is denied", ErrStepFailed, ip)
		}
	}

	return nil
}
