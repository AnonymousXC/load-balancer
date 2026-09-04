package config

import (
	"fmt"
	"os"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Backends  []BackendConfig `yaml:"backends"`
	Health    HealthConfig    `yaml:"health"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Strategy  string          `yaml:"strategy"`
	Fasthttp  bool            `yaml:"fasthttp"`
	TLS       TLSConfig       `yaml:"tls"`
	Security  SecurityConfig  `yaml:"security"`
}

type ServerConfig struct {
	Listen       string `yaml:"listen"`
	ReadTimeout  int    `yaml:"read_timeout"`
	WriteTimeout int    `yaml:"write_timeout"`
	IdleTimeout  int    `yaml:"idle_timeout"`
	AdminListen  string `yaml:"admin_listen"`
}

type TLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	CAFile     string `yaml:"ca_file"`
	MinVersion string `yaml:"min_version"`
	MaxVersion string `yaml:"max_version"`
}

type SecurityConfig struct {
	EnableCORS      bool     `yaml:"enable_cors"`
	AllowedOrigins  []string `yaml:"allowed_origins"`
	AllowedMethods  []string `yaml:"allowed_methods"`
	AllowedHeaders  []string `yaml:"allowed_headers"`
	IPWhitelist     []string `yaml:"ip_whitelist"`
	IPBlacklist     []string `yaml:"ip_blacklist"`
	MaxRequestSize  int64    `yaml:"max_request_size"`
	MaxResponseSize int64    `yaml:"max_response_size"`
}

type BackendConfig struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type HealthConfig struct {
	Interval int    `yaml:"interval_seconds"`
	Timeout  int    `yaml:"timeout_seconds"`
	Path     string `yaml:"path"`
}

type RateLimitConfig struct {
	RPS   int `yaml:"rps"`
	Burst int `yaml:"burst"`
}

type Manager struct {
	path   string
	config atomic.Value
}

func NewManager(path string) (*Manager, error) {
	m := &Manager{path: path}
	cfg, err := m.load()
	if err != nil {
		return nil, err
	}
	m.config.Store(cfg)
	return m, nil
}

func (m *Manager) load() (*Config, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := ValidateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func (m *Manager) Reload() error {
	cfg, err := m.load()
	if err != nil {
		return err
	}
	m.config.Store(cfg)
	return nil
}

func (m *Manager) Get() *Config {
	return m.config.Load().(*Config)
}
