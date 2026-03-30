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
	DefaultMode string          `yaml:"default_mode"` // "fastest" or "all"
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

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &Config{DefaultMode: "all"}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Resolve API keys from env vars if prefixed with "$".
	resolveEnvKeys(cfg)

	return cfg, nil
}

func findConfigFile() string {
	candidates := []string{
		"racellm.yaml",
		".racellm.yaml",
	}

	// Check current directory first.
	for _, name := range candidates {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}

	// Check home directory.
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
