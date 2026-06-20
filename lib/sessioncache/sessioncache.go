package sessioncache

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"container/list"
	"sync"
	"time"
)

var (
	ErrInvalidConfig = errors.New("sessioncache: invalid configuration")
	ErrMissingHMACKey = errors.New("sessioncache: HMAC key is required")
)

type Config struct {
	MaxEntries  int           `json:"maxEntries" yaml:"maxEntries"`
	DefaultTTL  time.Duration `json:"defaultTTL" yaml:"defaultTTL"`
	HMACKey     string        `json:"hmacKey" yaml:"hmacKey"`
}

func (c Config) Valid() error {
	if c.MaxEntries <= 0 {
		return fmt.Errorf("%w: maxEntries must be greater than zero", ErrInvalidConfig)
	}
	if c.DefaultTTL <= 0 {
		return fmt.Errorf("%w: defaultTTL must be greater than zero", ErrInvalidConfig)
	}
	if c.HMACKey == "" {
		return ErrMissingHMACKey
	}
	return nil
}

type Session struct {
	Token     string    `json:"token"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Cache struct {
	config  Config
	mu      sync.Mutex
	entries map[string]*list.Element
	lru     *list.List
	hmacKey []byte
	lg      *slog.Logger
}

func New(cfg Config, lg *slog.Logger) (*Cache, error) {
	if err := cfg.Valid(); err != nil {
		return nil, err
	}

	h := sha256.Sum256([]byte(cfg.HMACKey))
	hmacKey := h[:]

	return &Cache{
		config:  cfg,
		entries: make(map[string]*list.Element),
		lru:     list.New(),
		hmacKey: hmacKey,
		lg:      lg,
	}, nil
}

func (c *Cache) Create(ip string) Session {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	randBytes := make([]byte, 16)
	rand.Read(randBytes)

	mac := hmac.New(sha256.New, c.hmacKey)
	mac.Write([]byte(ip))
	mac.Write([]byte(now.Format(time.RFC3339Nano)))
	mac.Write(randBytes)
	token := hex.EncodeToString(mac.Sum(nil))

	sess := Session{
		Token:     token,
		IP:        ip,
		CreatedAt: now,
		ExpiresAt: now.Add(c.config.DefaultTTL),
	}

	elem := c.lru.PushFront(sess)
	c.entries[token] = elem

	for len(c.entries) > c.config.MaxEntries {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		c.lru.Remove(oldest)
		delete(c.entries, oldest.Value.(Session).Token)
	}

	return sess
}

func (c *Cache) Validate(token string) (*Session, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[token]
	if !ok {
		return nil, false
	}

	sess := elem.Value.(Session)
	if time.Now().After(sess.ExpiresAt) {
		c.lru.Remove(elem)
		delete(c.entries, token)
		return nil, false
	}

	c.lru.MoveToFront(elem)
	return &sess, true
}

func (c *Cache) Revoke(token string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[token]
	if !ok {
		return false
	}

	c.lru.Remove(elem)
	delete(c.entries, token)
	return true
}

func (c *Cache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
