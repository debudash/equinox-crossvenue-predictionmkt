package card

import (
	"fmt"
	"os"
	"slices"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is config/equinox.yaml.
type Config struct {
	Venues struct {
		Allowed []string `yaml:"allowed"`
	} `yaml:"venues"`
	// Fetchers is keyed by venue name; only fetcher code reads its own entry.
	Fetchers map[string]FetcherConfig `yaml:"fetchers"`
	Router   struct {
		ReferenceVenue string        `yaml:"reference_venue"`
		CounterVenue   string        `yaml:"counter_venue"`
		PriceMaxAge    time.Duration `yaml:"price_max_age"`
	} `yaml:"router"`
	Storage struct {
		SQLitePath string `yaml:"sqlite_path"`
	} `yaml:"storage"`
	Watch struct {
		DiscoveryEvery time.Duration `yaml:"discovery_every"`
		PricesEvery    time.Duration `yaml:"prices_every"`
		Retention      time.Duration `yaml:"retention"`
	} `yaml:"watch"`
	Feed struct {
		CacheCheckEvery time.Duration `yaml:"cache_check_every"`
		PageSize        int           `yaml:"page_size"`
		KeepSnapshots   int           `yaml:"keep_snapshots"`
	} `yaml:"feed"`
}

// FetcherConfig is one venue's fetcher settings.
type FetcherConfig struct {
	Source  string `yaml:"source"`
	BaseURL string `yaml:"base_url"`
}

// LoadConfig reads and validates a config file.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseConfig(b)
}

// ParseConfig parses and validates config YAML. It checks only what every
// command relies on; watch checks its own interval rule (W11) at startup.
func ParseConfig(b []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	r := c.Router
	switch {
	case len(c.Venues.Allowed) == 0:
		return nil, fmt.Errorf("config: venues.allowed is empty")
	case !slices.Contains(c.Venues.Allowed, r.ReferenceVenue):
		return nil, fmt.Errorf("config: router.reference_venue %q is not in venues.allowed", r.ReferenceVenue)
	case !slices.Contains(c.Venues.Allowed, r.CounterVenue):
		return nil, fmt.Errorf("config: router.counter_venue %q is not in venues.allowed", r.CounterVenue)
	case r.ReferenceVenue == r.CounterVenue:
		return nil, fmt.Errorf("config: reference and counter venue are both %q", r.ReferenceVenue)
	case r.PriceMaxAge <= 0:
		return nil, fmt.Errorf("config: router.price_max_age must be positive")
	}
	return &c, nil
}

// Roles returns the configured venue roles.
func (c *Config) Roles() Roles {
	return Roles{Reference: c.Router.ReferenceVenue, Counter: c.Router.CounterVenue}
}
