package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	yaml := `
default_mode: fastest
providers:
  openai:
    enabled: true
    api_key: "sk-test"
    timeout_seconds: 30
    models:
      - gpt-4o
`
	path := writeTempConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DefaultMode != "fastest" {
		t.Errorf("DefaultMode = %q, want %q", cfg.DefaultMode, "fastest")
	}
	if cfg.Providers.OpenAI == nil {
		t.Fatal("OpenAI provider is nil")
	}
	if cfg.Providers.OpenAI.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", cfg.Providers.OpenAI.APIKey, "sk-test")
	}
	if len(cfg.Providers.OpenAI.Models) != 1 || cfg.Providers.OpenAI.Models[0] != "gpt-4o" {
		t.Errorf("Models = %v, want [gpt-4o]", cfg.Providers.OpenAI.Models)
	}
}

func TestLoad_DefaultMode(t *testing.T) {
	yaml := `
providers:
  openai:
    enabled: true
    api_key: "sk-test"
    models:
      - gpt-4o
`
	path := writeTempConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DefaultMode != "all" {
		t.Errorf("DefaultMode = %q, want default %q", cfg.DefaultMode, "all")
	}
}

func TestLoad_EnvVarResolution(t *testing.T) {
	t.Setenv("RACELLM_TEST_KEY", "sk-from-env")
	yaml := `
providers:
  openai:
    enabled: true
    api_key: "$RACELLM_TEST_KEY"
    models:
      - gpt-4o
`
	path := writeTempConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Providers.OpenAI.APIKey != "sk-from-env" {
		t.Errorf("APIKey = %q, want %q", cfg.Providers.OpenAI.APIKey, "sk-from-env")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/racellm.yaml")
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	// Tabs for indentation are illegal in YAML.
	path := writeTempConfig(t, "key:\n\t- bad_indent")
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid YAML, got nil")
	}
}

func TestLoad_ValidationEnabledMissingAPIKey(t *testing.T) {
	yaml := `
providers:
  anthropic:
    enabled: true
    api_key: ""
    models:
      - claude-sonnet-4-20250514
`
	path := writeTempConfig(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected validation error for missing API key, got nil")
	}
}

func TestLoad_ValidationEnabledNoModels(t *testing.T) {
	yaml := `
providers:
  openai:
    enabled: true
    api_key: "sk-test"
    models: []
`
	path := writeTempConfig(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected validation error for no models, got nil")
	}
}

func TestLoad_OllamaNoAPIKeyAllowed(t *testing.T) {
	yaml := `
providers:
  ollama:
    enabled: true
    models:
      - llama3
`
	path := writeTempConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error for Ollama without API key: %v", err)
	}
	if cfg.Providers.Ollama == nil || !cfg.Providers.Ollama.Enabled {
		t.Error("Ollama provider should be enabled")
	}
}

func TestFindConfigFile_NoFile(t *testing.T) {
	// Run from a temp dir that has no config file.
	tmp := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	if got := findConfigFile(); got != "" {
		t.Errorf("findConfigFile() = %q, want empty string", got)
	}
}

// writeTempConfig writes yaml content to a temporary file and returns its path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "racellm-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(f.Name())
}
