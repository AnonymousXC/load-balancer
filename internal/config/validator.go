package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Validator struct {
	config *Config
	errors []string
}

func NewValidator(config *Config) *Validator {
	return &Validator{
		config: config,
		errors: make([]string, 0),
	}
}

func (v *Validator) Validate() error {
	v.validateServer()
	v.validateBackends()
	v.validateHealth()
	v.validateRateLimit()
	v.validateStrategy()
	v.validateTLS()
	v.validateSecurity()

	if len(v.errors) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(v.errors, "; "))
	}
	return nil
}

func (v *Validator) validateServer() {
	if v.config.Server.Listen == "" {
		v.errors = append(v.errors, "server.listen is required")
	}

	if v.config.Server.ReadTimeout <= 0 {
		v.errors = append(v.errors, "server.read_timeout must be positive")
	}

	if v.config.Server.WriteTimeout <= 0 {
		v.errors = append(v.errors, "server.write_timeout must be positive")
	}

	if v.config.Server.IdleTimeout <= 0 {
		v.errors = append(v.errors, "server.idle_timeout must be positive")
	}
}

func (v *Validator) validateBackends() {
	if len(v.config.Backends) == 0 {
		v.errors = append(v.errors, "at least one backend is required")
		return
	}

	for i, backend := range v.config.Backends {
		if backend.URL == "" {
			v.errors = append(v.errors, fmt.Sprintf("backend[%d].url is required", i))
			continue
		}

		parsedURL, err := url.Parse(backend.URL)
		if err != nil {
			v.errors = append(v.errors, fmt.Sprintf("backend[%d].url is invalid: %v", i, err))
			continue
		}

		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			v.errors = append(v.errors, fmt.Sprintf("backend[%d].url must use http or https scheme", i))
		}

		if parsedURL.Host == "" {
			v.errors = append(v.errors, fmt.Sprintf("backend[%d].url must include host", i))
		}

		if backend.Weight < 0 {
			v.errors = append(v.errors, fmt.Sprintf("backend[%d].weight must be non-negative", i))
		}
	}
}

func (v *Validator) validateHealth() {
	if v.config.Health.Interval <= 0 {
		v.errors = append(v.errors, "health.interval_seconds must be positive")
	}

	if v.config.Health.Timeout <= 0 {
		v.errors = append(v.errors, "health.timeout_seconds must be positive")
	}

	if v.config.Health.Timeout > v.config.Health.Interval {
		v.errors = append(v.errors, "health.timeout_seconds must be less than health.interval_seconds")
	}

	if v.config.Health.Path == "" {
		v.errors = append(v.errors, "health.path is required")
	}

	if !strings.HasPrefix(v.config.Health.Path, "/") {
		v.errors = append(v.errors, "health.path must start with /")
	}
}

func (v *Validator) validateRateLimit() {
	if v.config.RateLimit.RPS < 0 {
		v.errors = append(v.errors, "rate_limit.rps must be non-negative")
	}

	if v.config.RateLimit.Burst < 0 {
		v.errors = append(v.errors, "rate_limit.burst must be non-negative")
	}

	if v.config.RateLimit.Burst > 0 && v.config.RateLimit.RPS == 0 {
		v.errors = append(v.errors, "rate_limit.rps must be positive when rate_limit.burst is set")
	}
}

func (v *Validator) validateStrategy() {
	validStrategies := map[string]bool{
		"round_robin":          true,
		"least_conn":           true,
		"consistent_hash":      true,
		"weighted_round_robin": true,
	}

	if !validStrategies[v.config.Strategy] {
		v.errors = append(v.errors, fmt.Sprintf("invalid strategy: %s (valid options: round_robin, least_conn, consistent_hash, weighted_round_robin)", v.config.Strategy))
	}
}

func (v *Validator) validateTLS() {
	if v.config.TLS.Enabled {
		if v.config.TLS.CertFile == "" {
			v.errors = append(v.errors, "tls.cert_file is required when tls.enabled is true")
		} else if _, err := os.Stat(v.config.TLS.CertFile); os.IsNotExist(err) {
			v.errors = append(v.errors, fmt.Sprintf("tls.cert_file does not exist: %s", v.config.TLS.CertFile))
		}

		if v.config.TLS.KeyFile == "" {
			v.errors = append(v.errors, "tls.key_file is required when tls.enabled is true")
		} else if _, err := os.Stat(v.config.TLS.KeyFile); os.IsNotExist(err) {
			v.errors = append(v.errors, fmt.Sprintf("tls.key_file does not exist: %s", v.config.TLS.KeyFile))
		}

		if v.config.TLS.CAFile != "" {
			if _, err := os.Stat(v.config.TLS.CAFile); os.IsNotExist(err) {
				v.errors = append(v.errors, fmt.Sprintf("tls.ca_file does not exist: %s", v.config.TLS.CAFile))
			}
		}

		validVersions := map[string]bool{
			"TLS1.0": true,
			"TLS1.1": true,
			"TLS1.2": true,
			"TLS1.3": true,
		}

		if v.config.TLS.MinVersion != "" && !validVersions[v.config.TLS.MinVersion] {
			v.errors = append(v.errors, "tls.min_version must be one of: TLS1.0, TLS1.1, TLS1.2, TLS1.3")
		}

		if v.config.TLS.MaxVersion != "" && !validVersions[v.config.TLS.MaxVersion] {
			v.errors = append(v.errors, "tls.max_version must be one of: TLS1.0, TLS1.1, TLS1.2, TLS1.3")
		}
	}
}

func (v *Validator) validateSecurity() {
	if v.config.Security.MaxRequestSize < 0 {
		v.errors = append(v.errors, "security.max_request_size must be non-negative")
	}

	if v.config.Security.MaxResponseSize < 0 {
		v.errors = append(v.errors, "security.max_response_size must be non-negative")
	}
}

func ValidateConfig(config *Config) error {
	validator := NewValidator(config)
	return validator.Validate()
}
