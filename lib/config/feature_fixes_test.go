package config_test

import (
	"testing"

	"github.com/TecharoHQ/anubis/lib/config"
)

func TestIPFilterConfigValid_CIDR(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.IPFilterConfig
		wantErr bool
	}{
		{
			name: "valid IPv4 CIDR",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "192.168.1.0/24", ListType: "whitelist"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid IPv4 single IP",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "8.8.8.8", ListType: "blacklist"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid IPv6 CIDR",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "2001:db8::/32", ListType: "whitelist"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid IPv6 single address",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "::1", ListType: "whitelist"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid CIDR mask bits too high (IPv4 /33)",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "10.0.0.0/33", ListType: "blacklist"},
				},
			},
			wantErr: true,
		},
		{
			name: "malformed CIDR - not a number after slash",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "10.0.0.0/xx", ListType: "blacklist"},
				},
			},
			wantErr: true,
		},
		{
			name: "garbage string - not an IP",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "definitely-not-an-ip", ListType: "blacklist"},
				},
			},
			wantErr: true,
		},
		{
			name: "IPv4 octet out of range (300)",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "300.1.2.3", ListType: "whitelist"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty CIDR rejected",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "", ListType: "whitelist"},
				},
			},
			wantErr: true,
		},
		{
			name: "bad list_type rejected",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "10.0.0.1", ListType: "greylist"},
				},
			},
			wantErr: true,
		},
		{
			name: "multi entry valid passes",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "10.0.0.1", ListType: "whitelist"},
					{CIDR: "172.16.0.0/12", ListType: "blacklist"},
					{CIDR: "::1", ListType: "whitelist"},
				},
			},
			wantErr: false,
		},
		{
			name: "multi entry second invalid - still rejected",
			cfg: config.IPFilterConfig{
				Entries: []config.IPFilterEntry{
					{CIDR: "10.0.0.1", ListType: "whitelist"},
					{CIDR: "256.256.256.256", ListType: "blacklist"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Valid()
			if tc.wantErr && err == nil {
				t.Errorf("Expected error for config %+v, got nil", tc.cfg)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for config %+v, got: %v", tc.cfg, err)
			}
			if err != nil {
				t.Logf("  got error: %v", err)
			}
		})
	}
}

func TestAdaptiveDifficultyConfigValid_MaxStep_Smoothing(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.AdaptiveDifficultyConfig
		wantErr bool
	}{
		{
			name: "disabled skips check",
			cfg: config.AdaptiveDifficultyConfig{
				Enabled:         false,
				MaxStep:         -1,
				SmoothingFactor: -5.0,
			},
			wantErr: false,
		},
		{
			name: "enabled with valid values",
			cfg: config.AdaptiveDifficultyConfig{
				Enabled:           true,
				MinDifficulty:     1,
				MaxDifficulty:     64,
				MaxStep:           5,
				SmoothingFactor:   0.3,
				TargetCPULoad:     0.7,
				TargetConnections: 1000,
			},
			wantErr: false,
		},
		{
			name: "enabled negative MaxStep rejected",
			cfg: config.AdaptiveDifficultyConfig{
				Enabled:       true,
				MinDifficulty: 1,
				MaxDifficulty: 64,
				MaxStep:       -3,
			},
			wantErr: true,
		},
		{
			name: "enabled smoothing out of range (>1) rejected",
			cfg: config.AdaptiveDifficultyConfig{
				Enabled:         true,
				MinDifficulty:   1,
				MaxDifficulty:   64,
				SmoothingFactor: 2.0,
			},
			wantErr: true,
		},
		{
			name: "enabled smoothing out of range (<0) rejected",
			cfg: config.AdaptiveDifficultyConfig{
				Enabled:         true,
				MinDifficulty:   1,
				MaxDifficulty:   64,
				SmoothingFactor: -0.1,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Valid()
			if tc.wantErr && err == nil {
				t.Errorf("Expected error for config %+v, got nil", tc.cfg)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for config %+v, got: %v", tc.cfg, err)
			}
			if err != nil {
				t.Logf("  got error: %v", err)
			}
		})
	}
}

func TestSessionCacheConfigValid_RotationInterval(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.SessionCacheConfig
		wantErr bool
	}{
		{
			name:    "disabled skips",
			cfg:     config.SessionCacheConfig{Enabled: false, RotationInterval: -100},
			wantErr: false,
		},
		{
			name: "enabled valid with rotation",
			cfg: config.SessionCacheConfig{
				Enabled:          true,
				MaxEntries:       100,
				HMACKey:          "secret",
				RotationInterval: 24 * 3600000000000,
			},
			wantErr: false,
		},
		{
			name: "enabled negative rotation rejected",
			cfg: config.SessionCacheConfig{
				Enabled:          true,
				MaxEntries:       100,
				HMACKey:          "secret",
				RotationInterval: -1,
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Valid()
			if tc.wantErr && err == nil {
				t.Errorf("Expected error for %+v, got nil", tc.cfg)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Expected no error for %+v, got %v", tc.cfg, err)
			}
		})
	}
}
