package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// configStore is the single source of truth for Config. Every read and write
// goes through it under an RWMutex, which removes the data races between the
// monitor loop, the discovery loop and the HTTP handlers that previously all
// held the same *Config.
type configStore struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func newConfigStore(path string) (*configStore, error) {
	cfg, err := loadConfig(path)
	if err != nil {
		return nil, err
	}
	return &configStore{path: path, cfg: cfg}, nil
}

// Snapshot returns a deep copy safe to read without holding the lock. Product
// slice and keyword slice are copied so callers can't mutate store state.
func (s *configStore) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.clone()
}

func (c Config) clone() Config {
	out := c
	out.Products = append([]Product(nil), c.Products...)
	out.Discover.Keywords = append([]string(nil), c.Discover.Keywords...)
	return out
}

func (s *configStore) Products() []Product {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Product(nil), s.cfg.Products...)
}

func (s *configStore) Settings() settingsDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return settingsFrom(&s.cfg)
}

func (s *configStore) Discover() DiscoverConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d := s.cfg.Discover
	d.Keywords = append([]string(nil), s.cfg.Discover.Keywords...)
	return d
}

func (s *configStore) Webhook() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.DiscordWebhook
}

// WatchParams returns what a monitor worker needs per cycle. Read fresh each
// cycle so interval/jitter edits apply without a restart.
func (s *configStore) WatchParams() (intervalSec, jitterMs, timeoutSec int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.CheckIntervalSec, s.cfg.JitterMs, s.cfg.TimeoutSec
}

// UpdateSettings replaces the monitor settings (not products/discover).
func (s *configStore) UpdateSettings(in settingsDTO) (settingsDTO, error) {
	if err := validateSettings(in); err != nil {
		return settingsDTO{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.CheckIntervalSec = in.CheckIntervalSec
	s.cfg.JitterMs = in.JitterMs
	s.cfg.TimeoutSec = in.TimeoutSec
	s.cfg.DiscordWebhook = in.DiscordWebhook
	s.cfg.AutoOpenBrowser = in.AutoOpenBrowser
	s.cfg.BeepOnHit = in.BeepOnHit
	s.cfg.Discover = in.Discover
	normalizeDiscover(&s.cfg.Discover)
	if err := saveConfig(s.path, &s.cfg); err != nil {
		return settingsDTO{}, err
	}
	return settingsFrom(&s.cfg), nil
}

func (s *configStore) UpdateDiscover(in DiscoverConfig) (DiscoverConfig, error) {
	normalizeDiscover(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Discover = in
	if err := saveConfig(s.path, &s.cfg); err != nil {
		return DiscoverConfig{}, err
	}
	return s.cfg.Discover, nil
}

// AddProduct appends a product and persists. Reports whether it was added.
func (s *configStore) AddProduct(item Product) error {
	if err := validateProductURL(item.URL); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Products = append(s.cfg.Products, item)
	return saveConfig(s.path, &s.cfg)
}

// AddDiscovered appends only if the URL isn't already watched. Returns
// whether it was actually added (used by the discovery loop).
func (s *configStore) AddDiscovered(name, url string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.cfg.Products {
		if p.URL == url {
			return false, nil
		}
	}
	s.cfg.Products = append(s.cfg.Products, Product{Name: name, URL: url})
	if err := saveConfig(s.path, &s.cfg); err != nil {
		return false, err
	}
	return true, nil
}

// AddDiscoveredBatch adds several discovered URLs at once under a single lock,
// skipping any already watched. Returns the URLs actually added.
func (s *configStore) AddDiscoveredBatch(urls []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := make(map[string]bool, len(s.cfg.Products))
	for _, p := range s.cfg.Products {
		existing[p.URL] = true
	}
	var added []string
	for _, u := range urls {
		if existing[u] {
			continue
		}
		existing[u] = true
		s.cfg.Products = append(s.cfg.Products, Product{Name: slugToName(u) + " (discovered)", URL: u})
		added = append(added, u)
	}
	if len(added) == 0 {
		return nil, nil
	}
	if err := saveConfig(s.path, &s.cfg); err != nil {
		return added, err
	}
	return added, nil
}

func (s *configStore) RemoveProduct(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.cfg.Products) {
		return fmt.Errorf("product index not found")
	}
	s.cfg.Products = append(s.cfg.Products[:index], s.cfg.Products[index+1:]...)
	return saveConfig(s.path, &s.cfg)
}

func (s *configStore) ProductCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cfg.Products)
}

// updateStatus refreshes the dashboard status snapshot from current state.
func (s *configStore) updateStatus(note string) {
	s.mu.RLock()
	n := len(s.cfg.Products)
	interval := s.cfg.CheckIntervalSec
	s.mu.RUnlock()
	setUIStatus(n, interval, note)
}

// validateSettings centralises the settings rules in one place so every
// entry point (settings PUT, config import) enforces them identically.
func validateSettings(in settingsDTO) error {
	if in.CheckIntervalSec < 10 {
		return fmt.Errorf("check_interval_sec must be >= 10 (rate-limit safety)")
	}
	if in.TimeoutSec < 5 {
		return fmt.Errorf("timeout_sec must be >= 5")
	}
	return nil
}

// validateProductURL is the single URL policy used by products API, drops and
// discovery (previously copy-pasted in three places).
func validateProductURL(url string) error {
	u := strings.TrimSpace(url)
	if u == "" {
		return fmt.Errorf("url required")
	}
	switch {
	case strings.HasPrefix(u, "https://"):
		return nil
	case strings.HasPrefix(u, "http://localhost"), strings.HasPrefix(u, "http://127.0.0.1"):
		return nil // tests / local fakes
	default:
		return fmt.Errorf("url must start with https://")
	}
}

func loadConfig(path string) (Config, error) {
	c := Config{
		CheckIntervalSec: 20,
		TimeoutSec:       15,
		Discover:         defaultDiscover(),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// First run: no config file yet, use defaults.
			return c, nil
		}
		return c, err
	}
	// Decode over defaults so absent keys keep sane values; then normalise
	// nested structs encoding/json leaves half-zeroed.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if _, ok := raw["discover"]; !ok {
		c.Discover = defaultDiscover()
	} else {
		normalizeDiscover(&c.Discover)
	}
	if c.CheckIntervalSec <= 0 {
		c.CheckIntervalSec = 20
	}
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = 15
	}
	if c.JitterMs < 0 {
		c.JitterMs = 0
	}
	return c, nil
}

func saveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
