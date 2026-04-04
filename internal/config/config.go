// Package config handles loading and parsing the racellm configuration file.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/khang/racellm/internal/provider"
)

// Config is the top-level configuration for RaceLLM.
type Config struct {
	DefaultMode string          `yaml:"default_mode"`
	Providers   ProvidersConfig `yaml:"providers"`
}

// ProvidersConfig holds per-provider settings.
type ProvidersConfig struct {
	OpenAI    *ProviderEntry `yaml:"openai,omitempty"`
	Anthropic *ProviderEntry `yaml:"anthropic,omitempty"`
	Gemini    *ProviderEntry `yaml:"gemini,omitempty"`
	Ollama    *ProviderEntry `yaml:"ollama,omitempty"`
}

// ProviderEntry combines the provider config with a list of models to race.
type ProviderEntry struct {
	provider.ProviderConfig `yaml:",inline"`
	Models                  []string `yaml:"models"`
	Enabled                 bool     `yaml:"enabled"`
}

// Load reads the config file from the given path. If path is empty,
// it searches the default locations.
func Load(path string) (*Config, error) {
	if path == "" {
		path = findConfigFile()
	}
	if path == "" {
		return nil, fmt.Errorf("no config file found; create one at ~/.racellm.yaml or pass --config")
	}

	data, err := os.ReadFile(path) // nolint:gosec // G304: config file path is provided by the user at runtime by design
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &Config{DefaultMode: "all"}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	resolveEnvKeys(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func findConfigFile() string {
	candidates := []string{
		"racellm.yaml",
		".racellm.yaml",
	}

	for _, name := range candidates {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range candidates {
		p := filepath.Join(home, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

func resolveEnvKeys(cfg *Config) {
	resolve := func(entry *ProviderEntry) {
		if entry == nil {
			return
		}
		if entry.APIKey != "" && entry.APIKey[0] == '$' {
			entry.APIKey = os.Getenv(entry.APIKey[1:])
		}
	}
	resolve(cfg.Providers.OpenAI)
	resolve(cfg.Providers.Anthropic)
	resolve(cfg.Providers.Gemini)
	resolve(cfg.Providers.Ollama)
}

// validate checks that every enabled provider that requires an API key has one set.
func validate(cfg *Config) error {
	type check struct {
		name  string
		entry *ProviderEntry
		// needsKey is false for providers that work without an API key (e.g. Ollama)
		needsKey bool
	}
	checks := []check{
		{"openai", cfg.Providers.OpenAI, true},
		{"anthropic", cfg.Providers.Anthropic, true},
		{"gemini", cfg.Providers.Gemini, true},
		{"ollama", cfg.Providers.Ollama, false},
	}
	for _, c := range checks {
		if c.entry == nil || !c.entry.Enabled {
			continue
		}
		if c.needsKey && c.entry.APIKey == "" {
			return fmt.Errorf("%s is enabled but api_key is not set (set the env var or add it to your config)", c.name)
		}
		if len(c.entry.Models) == 0 {
			return fmt.Errorf("%s is enabled but has no models configured", c.name)
		}
	}
	return nil
}
