// Package config handles AgentLens configuration loading.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for AgentLens.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Storage   StorageConfig   `yaml:"storage"`
	Telemetry TelemetryConfig `yaml:"telemetry"`
	Proxy     ProxyConfig     `yaml:"proxy"`
	Evaluator EvaluatorConfig `yaml:"evaluator"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	GatewayPort   int `yaml:"gateway_port"`   // Port for the proxy gateway (default: 8080)
	DashboardPort int `yaml:"dashboard_port"` // Port for the dashboard UI (default: 8090)
}

// ProxyConfig holds upstream LLM provider URLs.
type ProxyConfig struct {
	AnthropicUpstream string `yaml:"anthropic_upstream"`
	OpenAIUpstream    string `yaml:"openai_upstream"`
	GeminiUpstream    string `yaml:"gemini_upstream"`
}

// DefaultProxyConfig returns the default upstream URLs.
func DefaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		AnthropicUpstream: "https://api.anthropic.com",
		OpenAIUpstream:    "https://api.openai.com",
		GeminiUpstream:    "https://generativelanguage.googleapis.com",
	}
}

// StorageConfig holds database settings.
type StorageConfig struct {
	PostgresDSN string `yaml:"postgres_dsn"`
}

// UpstreamConfig holds the upstream LLM provider endpoints.
type UpstreamConfig struct {
	Anthropic string `yaml:"anthropic"`
	OpenAI    string `yaml:"openai"`
	Gemini    string `yaml:"gemini"`
}

// TelemetryConfig holds observability settings.
type TelemetryConfig struct {
	OTLPEndpoint   string `yaml:"otlp_endpoint"`
	PrometheusPort int    `yaml:"prometheus_port"`
}

// EvaluatorConfig holds settings for the LLM judge.
type EvaluatorConfig struct {
	Enabled         bool   `yaml:"enabled"`
	APIBase         string `yaml:"api_base"`
	APIKey          string `yaml:"api_key"`
	Model           string `yaml:"model"`
	IntervalSeconds int    `yaml:"interval_seconds"`
}

// Default returns a configuration with sensible defaults.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			GatewayPort:   8080,
			DashboardPort: 8090,
		},
		Storage: StorageConfig{
			PostgresDSN: "postgres://agentlens:agentlens@localhost:5432/agentlens?sslmode=disable",
		},
		Telemetry: TelemetryConfig{
			OTLPEndpoint:   "http://localhost:4318",
			PrometheusPort: 9091,
		},
		Proxy: DefaultProxyConfig(),
		Evaluator: EvaluatorConfig{
			Enabled:         true,
			APIBase:         "https://generativelanguage.googleapis.com/v1beta/openai",
			APIKey:          "",
			Model:           "gemini-1.5-flash",
			IntervalSeconds: 60,
		},
	}
}

// Load reads configuration from a YAML file, falling back to defaults.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path == "" {
		path = "config.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // use defaults
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Environment variable overrides.
	if dsn := os.Getenv("AGENTLENS_POSTGRES_DSN"); dsn != "" {
		cfg.Storage.PostgresDSN = dsn
	}
	if otlp := os.Getenv("AGENTLENS_OTLP_ENDPOINT"); otlp != "" {
		cfg.Telemetry.OTLPEndpoint = otlp
	}
	if evalKey := os.Getenv("AGENTLENS_EVALUATOR_API_KEY"); evalKey != "" {
		cfg.Evaluator.APIKey = evalKey
	}

	return cfg, nil
}
