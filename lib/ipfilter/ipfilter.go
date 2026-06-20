package ipfilter

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
)

var (
	ErrInvalidCIDR     = errors.New("ipfilter: invalid CIDR")
	ErrInvalidListType = errors.New("ipfilter: invalid list type")
)

type ListType string

const (
	ListTypeWhitelist ListType = "whitelist"
	ListTypeBlacklist ListType = "blacklist"
)

func (lt ListType) Valid() error {
	switch lt {
	case ListTypeWhitelist, ListTypeBlacklist:
		return nil
	default:
		return ErrInvalidListType
	}
}

type Entry struct {
	CIDR     string   `json:"cidr" yaml:"cidr"`
	ListType ListType `json:"list_type" yaml:"list_type"`
}

type Config struct {
	Entries []Entry `json:"entries" yaml:"entries"`
}

func (c Config) Valid() error {
	var errs []error
	for i, e := range c.Entries {
		if err := e.ListType.Valid(); err != nil {
			errs = append(errs, fmt.Errorf("entry %d: %w: %q", i, err, e.ListType))
		}
		cidr := e.CIDR
		if cidr == "" {
			errs = append(errs, fmt.Errorf("entry %d: %w: empty CIDR", i, ErrInvalidCIDR))
			continue
		}
		if !stringsContainsSlash(cidr) {
			ip := net.ParseIP(cidr)
			if ip == nil {
				errs = append(errs, fmt.Errorf("entry %d: %w: %q", i, ErrInvalidCIDR, cidr))
				continue
			}
		} else {
			_, _, err := net.ParseCIDR(cidr)
			if err != nil {
				errs = append(errs, fmt.Errorf("entry %d: %w: %q", i, ErrInvalidCIDR, cidr))
				continue
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func stringsContainsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}

type parsedEntry struct {
	network  *net.IPNet
	listType ListType
	original string
}

type IPFilter struct {
	config   Config
	networks []parsedEntry
	mu       sync.RWMutex
	lg       *slog.Logger
}

func New(cfg Config, lg *slog.Logger) (*IPFilter, error) {
	if err := cfg.Valid(); err != nil {
		return nil, err
	}

	networks, err := parseEntries(cfg.Entries)
	if err != nil {
		return nil, err
	}

	return &IPFilter{
		config:   cfg,
		networks: networks,
		lg:       lg,
	}, nil
}

func parseEntries(entries []Entry) ([]parsedEntry, error) {
	result := make([]parsedEntry, 0, len(entries))
	for _, e := range entries {
		cidr := e.CIDR
		var ipNet *net.IPNet

		if !stringsContainsSlash(cidr) {
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, fmt.Errorf("%w: %q", ErrInvalidCIDR, cidr)
			}
			if v4 := ip.To4(); v4 != nil {
				_, network, err := net.ParseCIDR(v4.String() + "/32")
				if err != nil {
					return nil, fmt.Errorf("%w: %q", ErrInvalidCIDR, cidr)
				}
				ipNet = network
			} else {
				_, network, err := net.ParseCIDR(ip.String() + "/128")
				if err != nil {
					return nil, fmt.Errorf("%w: %q", ErrInvalidCIDR, cidr)
				}
				ipNet = network
			}
		} else {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("%w: %q", ErrInvalidCIDR, cidr)
			}
			ipNet = network
		}

		result = append(result, parsedEntry{
			network:  ipNet,
			listType: e.ListType,
			original: e.CIDR,
		})
	}
	return result, nil
}

func (f *IPFilter) Check(ip net.IP) (allow bool, reason string) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, entry := range f.networks {
		if entry.listType == ListTypeWhitelist && entry.network.Contains(ip) {
			return true, "whitelist"
		}
	}

	for _, entry := range f.networks {
		if entry.listType == ListTypeBlacklist && entry.network.Contains(ip) {
			return false, "blacklist"
		}
	}

	return true, "default"
}

func (f *IPFilter) Reload(cfg Config) error {
	if err := cfg.Valid(); err != nil {
		return err
	}

	networks, err := parseEntries(cfg.Entries)
	if err != nil {
		return err
	}

	f.mu.Lock()
	f.config = cfg
	f.networks = networks
	f.mu.Unlock()

	f.lg.Debug("ipfilter: configuration reloaded", "entry_count", len(cfg.Entries))
	return nil
}

func (f *IPFilter) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /reload", func(w http.ResponseWriter, r *http.Request) {
		var cfg Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := f.Reload(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /entries", func(w http.ResponseWriter, r *http.Request) {
		f.mu.RLock()
		entries := make([]Entry, len(f.config.Entries))
		copy(entries, f.config.Entries)
		f.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Config{Entries: entries})
	})

	return mux
}
