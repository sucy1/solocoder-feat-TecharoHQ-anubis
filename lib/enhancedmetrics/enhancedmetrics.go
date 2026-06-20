package enhancedmetrics

import (
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	PoWComputeTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "anubis_pow_compute_time_seconds",
		Help:    "Time taken for PoW challenge computation in seconds",
		Buckets: prometheus.DefBuckets,
	})

	PassCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "anubis_validation_pass_total",
		Help: "Total number of validation passes",
	}, []string{"method"})

	RejectCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "anubis_validation_reject_total",
		Help: "Total number of validation rejections",
	}, []string{"method", "reason"})

	CurrentLoad = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "anubis_current_load",
		Help: "Current system load average (1 minute)",
	})

	QueueLength = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "anubis_queue_length",
		Help: "Current number of connections waiting in queue",
	})

	SessionCacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "anubis_session_cache_size",
		Help: "Current number of entries in session cache",
	})

	AdaptiveDifficulty = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "anubis_adaptive_difficulty",
		Help: "Current adaptive PoW difficulty level",
	})
)

type MetricsTokenConfig struct {
	Token string `json:"token" yaml:"token"`
}

type MetricsConfig struct {
	MetricsToken *MetricsTokenConfig `json:"metricsToken,omitempty" yaml:"metricsToken,omitempty"`
}

type Collector struct {
	queueLength   atomic.Int64
	metricsConfig *MetricsTokenConfig
	lg            *slog.Logger
}

func NewCollector(cfg *MetricsConfig, lg *slog.Logger) *Collector {
	c := &Collector{
		lg: lg,
	}
	if cfg != nil {
		c.metricsConfig = cfg.MetricsToken
	}
	return c
}

func (c *Collector) IncQueue() {
	c.queueLength.Add(1)
	QueueLength.Set(float64(c.queueLength.Load()))
}

func (c *Collector) DecQueue() {
	c.queueLength.Add(-1)
	QueueLength.Set(float64(c.queueLength.Load()))
}

func (c *Collector) SetLoad(load float64) {
	CurrentLoad.Set(load)
}

func (c *Collector) SetSessionCacheSize(size int) {
	SessionCacheSize.Set(float64(size))
}

func (c *Collector) SetAdaptiveDifficulty(difficulty int) {
	AdaptiveDifficulty.Set(float64(difficulty))
}

func (c *Collector) RecordPoWPass(method string) {
	PassCount.WithLabelValues(method).Inc()
}

func (c *Collector) RecordPoWReject(method, reason string) {
	RejectCount.WithLabelValues(method, reason).Inc()
}

func (c *Collector) Middleware(next http.Handler) http.Handler {
	if c.metricsConfig == nil || c.metricsConfig.Token == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Anubis-Metrics-Token")
		if token == "" {
			token = r.URL.Query().Get("metrics_token")
		}

		if token != c.metricsConfig.Token {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
