package sessioncache

import (
	"container/list"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

var (
	ErrInvalidConfig = errors.New("sessioncache: invalid configuration")
	ErrMissingHMACKey = errors.New("sessioncache: HMAC key is required")
)

type Config struct {
	MaxEntries       int           `json:"maxEntries" yaml:"maxEntries"`
	DefaultTTL       time.Duration `json:"defaultTTL" yaml:"defaultTTL"`
	HMACKey          string        `json:"hmacKey" yaml:"hmacKey"`
	RotationInterval time.Duration `json:"rotation_interval" yaml:"rotation_interval"`
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
	if c.RotationInterval < 0 {
		return fmt.Errorf("%w: rotation_interval must be >= 0", ErrInvalidConfig)
	}
	return nil
}

type signedToken struct {
	KeyID     int       `json:"kid"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
	Nonce     string    `json:"nonce"`
	Sig       string    `json:"sig"`
}

type Session struct {
	Token     string    `json:"token"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type keyEntry struct {
	id     int
	key    []byte
	active time.Time
}

type Cache struct {
	config     Config
	mu         sync.Mutex
	entries    map[string]*list.Element
	lru        *list.List
	hmacKeys   []keyEntry
	nextKeyID  int
	done       chan struct{}
	lg         *slog.Logger
}

func deriveKey(seed string, kid int) []byte {
	h := sha256.New()
	h.Write([]byte(seed))
	h.Write([]byte(fmt.Sprintf("|v%d", kid)))
	sum := h.Sum(nil)
	return sum
}

func New(cfg Config, lg *slog.Logger) (*Cache, error) {
	if err := cfg.Valid(); err != nil {
		return nil, err
	}
	if cfg.RotationInterval <= 0 {
		cfg.RotationInterval = 24 * time.Hour
	}

	initialKey := deriveKey(cfg.HMACKey, 0)

	cache := &Cache{
		config:  cfg,
		entries: make(map[string]*list.Element),
		lru:     list.New(),
		hmacKeys: []keyEntry{
			{id: 0, key: initialKey, active: time.Now()},
		},
		nextKeyID: 1,
		done:      make(chan struct{}),
		lg:        lg,
	}
	return cache, nil
}

func (c *Cache) Start(ctx context.Context) {
	if c.config.RotationInterval <= 0 {
		return
	}
	ticker := time.NewTicker(c.config.RotationInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.done:
				return
			case <-ticker.C:
				c.Rotate()
			}
		}
	}()
}

func (c *Cache) Stop() {
	close(c.done)
}

func (c *Cache) Rotate() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldestMaxAge := 3 * c.config.RotationInterval
	now := time.Now()
	filtered := c.hmacKeys[:0]
	for _, k := range c.hmacKeys {
		if now.Sub(k.active) <= oldestMaxAge {
			filtered = append(filtered, k)
		}
	}
	c.hmacKeys = filtered

	newKey := deriveKey(c.config.HMACKey, c.nextKeyID)
	newEntry := keyEntry{
		id:     c.nextKeyID,
		key:    newKey,
		active: now,
	}
	c.hmacKeys = append(c.hmacKeys, newEntry)
	c.lg.Info("session cache: HMAC key rotated", "new_kid", c.nextKeyID, "total_keys", len(c.hmacKeys))
	c.nextKeyID++
	return newEntry.id
}

func (c *Cache) activeKey() (int, []byte) {
	if len(c.hmacKeys) == 0 {
		return -1, nil
	}
	best := c.hmacKeys[0]
	for _, k := range c.hmacKeys[1:] {
		if k.active.After(best.active) {
			best = k
		} else if k.active.Equal(best.active) && k.id > best.id {
			best = k
		}
	}
	return best.id, best.key
}

func (c *Cache) findKey(kid int) []byte {
	for _, k := range c.hmacKeys {
		if k.id == kid {
			return k.key
		}
	}
	return nil
}

func signToken(st *signedToken, key []byte) string {
	stCopy := *st
	stCopy.Sig = ""
	payload, _ := json.Marshal(stCopy)
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *Cache) Create(ip string) Session {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	kid, key := c.activeKey()
	if key == nil {
		return Session{}
	}

	nonceBytes := make([]byte, 16)
	rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)

	expires := now.Add(c.config.DefaultTTL)

	st := signedToken{
		KeyID:     kid,
		IP:        ip,
		CreatedAt: now,
		ExpiresAt: expires,
		Nonce:     nonce,
	}
	st.Sig = signToken(&st, key)

	payloadBytes, _ := json.Marshal(st)
	token := hex.EncodeToString(payloadBytes)

	sess := Session{
		Token:     token,
		IP:        ip,
		CreatedAt: now,
		ExpiresAt: expires,
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
	var st signedToken
	tokenBytes, err := hex.DecodeString(token)
	if err != nil {
		return nil, false
	}
	if err := json.Unmarshal(tokenBytes, &st); err != nil {
		return nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.findKey(st.KeyID)
	if key == nil {
		return nil, false
	}

	expectedSig := signToken(&st, key)
	if !hmac.Equal([]byte(expectedSig), []byte(st.Sig)) {
		return nil, false
	}

	now := time.Now()
	if now.After(st.ExpiresAt) {
		elem, ok := c.entries[token]
		if ok {
			c.lru.Remove(elem)
			delete(c.entries, token)
		}
		return nil, false
	}

	elem, ok := c.entries[token]
	if !ok {
		return nil, false
	}

	sess := elem.Value.(Session)
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
