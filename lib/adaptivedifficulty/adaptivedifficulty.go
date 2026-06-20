package adaptivedifficulty

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/load"
)

var ErrInvalidConfig = errors.New("adaptivedifficulty: invalid config")

type Config struct {
	MinDifficulty    int           `json:"min_difficulty" yaml:"min_difficulty"`
	MaxDifficulty    int           `json:"max_difficulty" yaml:"max_difficulty"`
	TargetCPULoad    float64       `json:"target_cpu_load" yaml:"target_cpu_load"`
	TargetConnections int          `json:"target_connections" yaml:"target_connections"`
	RecalcInterval   time.Duration `json:"recalc_interval" yaml:"recalc_interval"`
}

func (c Config) Valid() error {
	var errs []error
	if c.MinDifficulty < 1 {
		errs = append(errs, errors.New("MinDifficulty must be >= 1"))
	}
	if c.MaxDifficulty > 100 {
		errs = append(errs, errors.New("MaxDifficulty must be <= 100"))
	}
	if c.MinDifficulty > c.MaxDifficulty {
		errs = append(errs, errors.New("MinDifficulty must be <= MaxDifficulty"))
	}
	if c.TargetCPULoad <= 0 {
		errs = append(errs, errors.New("TargetCPULoad must be > 0"))
	}
	if c.TargetConnections <= 0 {
		errs = append(errs, errors.New("TargetConnections must be > 0"))
	}
	if c.RecalcInterval <= 0 {
		errs = append(errs, errors.New("RecalcInterval must be > 0"))
	}
	if len(errs) > 0 {
		return errors.Join(ErrInvalidConfig, errors.Join(errs...))
	}
	return nil
}

type AdaptiveDifficulty struct {
	config            Config
	currentDifficulty int
	connectionCount   int64
	mu                sync.RWMutex
	lg                *slog.Logger
	done              chan struct{}
}

func New(cfg Config, lg *slog.Logger) *AdaptiveDifficulty {
	if cfg.MinDifficulty < 1 {
		cfg.MinDifficulty = 1
	}
	if cfg.MaxDifficulty < 1 {
		cfg.MaxDifficulty = 64
	}
	if cfg.TargetCPULoad <= 0 {
		cfg.TargetCPULoad = 0.7
	}
	if cfg.TargetConnections <= 0 {
		cfg.TargetConnections = 1000
	}
	if cfg.RecalcInterval <= 0 {
		cfg.RecalcInterval = 60 * time.Second
	}
	return &AdaptiveDifficulty{
		config:            cfg,
		currentDifficulty: cfg.MinDifficulty,
		lg:                lg,
		done:              make(chan struct{}),
	}
}

func (a *AdaptiveDifficulty) Start(ctx context.Context) {
	ticker := time.NewTicker(a.config.RecalcInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-a.done:
				return
			case <-ticker.C:
				a.recalculate()
			}
		}
	}()
}

func (a *AdaptiveDifficulty) Stop() {
	close(a.done)
}

func (a *AdaptiveDifficulty) CurrentDifficulty() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentDifficulty
}

func (a *AdaptiveDifficulty) IncConnections() {
	atomic.AddInt64(&a.connectionCount, 1)
}

func (a *AdaptiveDifficulty) DecConnections() {
	atomic.AddInt64(&a.connectionCount, -1)
}

func (a *AdaptiveDifficulty) recalculate() {
	avg, err := load.Avg()
	if err != nil {
		a.lg.Error("failed to get load average", "error", err)
		return
	}

	cpuLoad := avg.Load1
	connCount := atomic.LoadInt64(&a.connectionCount)
	baseDiff := a.config.MinDifficulty

	newDiff := baseDiff +
		int(float64(baseDiff)*(cpuLoad/a.config.TargetCPULoad)) +
		int(float64(baseDiff)*(float64(connCount)/float64(a.config.TargetConnections)))

	if newDiff < a.config.MinDifficulty {
		newDiff = a.config.MinDifficulty
	}
	if newDiff > a.config.MaxDifficulty {
		newDiff = a.config.MaxDifficulty
	}

	a.mu.Lock()
	oldDiff := a.currentDifficulty
	if newDiff != oldDiff {
		a.currentDifficulty = newDiff
	}
	a.mu.Unlock()

	if newDiff != oldDiff {
		a.lg.Info("adaptive difficulty recalculated", "old", oldDiff, "new", newDiff, "cpu_load", cpuLoad, "connections", connCount)
	}
}
