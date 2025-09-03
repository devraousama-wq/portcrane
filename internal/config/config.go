package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listeners []Listener `yaml:"listeners"`
	Routes    []Route    `yaml:"routes"`
	Pools     Pools      `yaml:"pools"`
	Logging   Logging    `yaml:"logging"`
	Metrics   Metrics    `yaml:"metrics"`
	Admin     Admin      `yaml:"admin"`
	TLS       TLS        `yaml:"tls"`
	Discovery Discovery  `yaml:"discovery"`
}

type Listener struct {
	Name     string `yaml:"name"`
	Protocol string `yaml:"protocol"`
	Bind     string `yaml:"bind"`
	TLS      string `yaml:"tls"`
}

type Route struct {
	Name       string      `yaml:"name"`
	Match      RouteMatch  `yaml:"match"`
	Pool       string      `yaml:"pool"`
	Middleware []string    `yaml:"middleware"`
}

type RouteMatch struct {
	Host       string            `yaml:"host"`
	PathPrefix string            `yaml:"path_prefix"`
	Method     string            `yaml:"method"`
	Headers    map[string]string `yaml:"headers"`
}

type Pools map[string]Pool

type Pool struct {
	Policy    string     `yaml:"policy"`
	Upstreams []Upstream `yaml:"upstreams"`
	Health    Health     `yaml:"health"`
	Breaker   Breaker    `yaml:"breaker"`
}

type Upstream struct {
	ID       string `yaml:"id"`
	Address  string `yaml:"address"`
	Weight   int    `yaml:"weight"`
	MaxConns int    `yaml:"max_conns"`
}

type Health struct {
	Active  ActiveHealth  `yaml:"active"`
	Passive PassiveHealth `yaml:"passive"`
}

type ActiveHealth struct {
	Enabled   bool   `yaml:"enabled"`
	Protocol  string `yaml:"protocol"`
	Path      string `yaml:"path"`
	Interval  string `yaml:"interval"`
	Timeout   string `yaml:"timeout"`
	Healthy   int    `yaml:"healthy_threshold"`
	Unhealthy int    `yaml:"unhealthy_threshold"`
}

type PassiveHealth struct {
	Enabled           bool `yaml:"enabled"`
	MaxFailures       int  `yaml:"max_failures"`
	ConsecutiveErrors int  `yaml:"consecutive_errors"`
}

type Breaker struct {
	Enabled        bool    `yaml:"enabled"`
	FailureRatio   float64 `yaml:"failure_ratio"`
	Consecutive5xx int     `yaml:"consecutive_5xx"`
	Cooldown       string  `yaml:"cooldown"`
}

type Logging struct {
	Level  string `yaml:"level"`
	Access string `yaml:"access"`
}

type Metrics struct {
	Bind string `yaml:"bind"`
}

type Admin struct {
	Bind  string `yaml:"bind"`
	Token string `yaml:"token"`
}

type TLS struct {
	Certs   []TLSCert `yaml:"certs"`
	ACME    ACME      `yaml:"acme"`
	HSTS    bool      `yaml:"hsts"`
	MinVersion string `yaml:"min_version"`
}

type TLSCert struct {
	Name     string `yaml:"name"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	Hosts    []string `yaml:"hosts"`
}

type ACME struct {
	Enabled  bool     `yaml:"enabled"`
	Email    string   `yaml:"email"`
	CacheDir string   `yaml:"cache_dir"`
	Hosts    []string `yaml:"hosts"`
}

type Discovery struct {
	File FileDiscovery `yaml:"file"`
	DNS  DNSDiscovery  `yaml:"dns"`
}

type FileDiscovery struct {
	Enabled  bool   `yaml:"enabled"`
	Path     string `yaml:"path"`
	Interval string `yaml:"interval"`
}

type DNSDiscovery struct {
	Enabled  bool   `yaml:"enabled"`
	Name     string `yaml:"name"`
	Interval string `yaml:"interval"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Listeners) == 0 {
		return fmt.Errorf("at least one listener required")
	}
	for i, l := range c.Listeners {
		if l.Bind == "" {
			return fmt.Errorf("listener %d missing bind", i)
		}
		if l.Protocol == "" {
			c.Listeners[i].Protocol = "http"
		}
	}
	if len(c.Pools) == 0 {
		return fmt.Errorf("at least one pool required")
	}
	for name, pool := range c.Pools {
		if len(pool.Upstreams) == 0 {
			return fmt.Errorf("pool %s has no upstreams", name)
		}
		if pool.Policy == "" {
			pool.Policy = "round_robin"
			c.Pools[name] = pool
		}
	}
	return nil
}
