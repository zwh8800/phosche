package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig_Valid(t *testing.T) {
	yaml := `
watch:
  directories:
    - /path/to/photos
  recursive: true
  debounce_ms: 500
  min_dir_depth: 1
llm:
  provider: ollama
  ollama:
    base_url: http://localhost:11434
    model: llama3.2-vision
  openai:
    api_key: ""
    base_url: https://api.openai.com/v1
    model: gpt-4o
  max_retries: 3
  concurrency: 2
  timeout_seconds: 60
  output_language: zh
elasticsearch:
  addresses:
    - http://localhost:9200
  username: ""
  password: ""
  insecure_skip_verify: false
  index_name: phosche
server:
  host: "0.0.0.0"
  port: 8080
  dev_mode: true
`
	path := writeTempYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if len(cfg.Watch.Directories) != 1 || cfg.Watch.Directories[0] != "/path/to/photos" {
		t.Errorf("unexpected Watch.Directories: %v", cfg.Watch.Directories)
	}
	if cfg.Watch.Recursive != true {
		t.Errorf("Watch.Recursive = %v, want true", cfg.Watch.Recursive)
	}
	if cfg.Watch.DebounceMs != 500 {
		t.Errorf("Watch.DebounceMs = %d, want 500", cfg.Watch.DebounceMs)
	}
	if cfg.Watch.MinDirDepth != 1 {
		t.Errorf("Watch.MinDirDepth = %d, want 1", cfg.Watch.MinDirDepth)
	}
	if cfg.LLM.Provider != "ollama" {
		t.Errorf("LLM.Provider = %q, want ollama", cfg.LLM.Provider)
	}
	if cfg.LLM.Ollama.BaseURL != "http://localhost:11434" {
		t.Errorf("LLM.Ollama.BaseURL = %q", cfg.LLM.Ollama.BaseURL)
	}
	if len(cfg.Elasticsearch.Addresses) != 1 || cfg.Elasticsearch.Addresses[0] != "http://localhost:9200" {
		t.Errorf("unexpected ES.Addresses: %v", cfg.Elasticsearch.Addresses)
	}
	if cfg.Elasticsearch.IndexName != "phosche" {
		t.Errorf("ES.IndexName = %q, want phosche", cfg.Elasticsearch.IndexName)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d", cfg.Server.Port)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := writeTempYAML(t, `invalid: yaml: broken: [`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	t.Run("missing watch.directories", func(t *testing.T) {
		yaml := `
watch:
  directories: []
llm:
  provider: ollama
elasticsearch:
  addresses:
    - http://localhost:9200
  index_name: phosche
`
		path := writeTempYAML(t, yaml)
		_, err := LoadConfig(path)
		if err == nil {
			t.Fatal("expected error for empty directories, got nil")
		}
	})

	t.Run("missing index_name", func(t *testing.T) {
		yaml := `
watch:
  directories:
    - /photos
llm:
  provider: ollama
elasticsearch:
  addresses:
    - http://localhost:9200
  index_name: ""
`
		path := writeTempYAML(t, yaml)
		_, err := LoadConfig(path)
		if err == nil {
			t.Fatal("expected error for empty index_name, got nil")
		}
	})

	t.Run("invalid llm provider", func(t *testing.T) {
		yaml := `
watch:
  directories:
    - /photos
llm:
  provider: invalid-provider
elasticsearch:
  addresses:
    - http://localhost:9200
  index_name: phosche
`
		path := writeTempYAML(t, yaml)
		_, err := LoadConfig(path)
		if err == nil {
			t.Fatal("expected error for invalid provider, got nil")
		}
	})

	t.Run("missing elasticsearch addresses", func(t *testing.T) {
		yaml := `
watch:
  directories:
    - /photos
llm:
  provider: openai
elasticsearch:
  addresses: []
  index_name: phosche
`
		path := writeTempYAML(t, yaml)
		_, err := LoadConfig(path)
		if err == nil {
			t.Fatal("expected error for empty addresses, got nil")
		}
	})
}

func TestLoadConfig_Defaults(t *testing.T) {
	yaml := `
watch:
  directories:
    - /photos
llm:
  provider: openai
elasticsearch:
  addresses:
    - http://localhost:9200
  index_name: phosche
`
	path := writeTempYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Watch.DebounceMs != 500 {
		t.Errorf("Watch.DebounceMs = %d, want 500 (default)", cfg.Watch.DebounceMs)
	}
	if cfg.Watch.Recursive != true {
		t.Errorf("Watch.Recursive = %v, want true (default)", cfg.Watch.Recursive)
	}
	if cfg.Watch.MinDirDepth != 1 {
		t.Errorf("Watch.MinDirDepth = %d, want 1 (default)", cfg.Watch.MinDirDepth)
	}
	if cfg.LLM.MaxRetries != 3 {
		t.Errorf("LLM.MaxRetries = %d, want 3 (default)", cfg.LLM.MaxRetries)
	}
	if cfg.LLM.Concurrency != 2 {
		t.Errorf("LLM.Concurrency = %d, want 2 (default)", cfg.LLM.Concurrency)
	}
	if cfg.LLM.TimeoutSeconds != 60 {
		t.Errorf("LLM.TimeoutSeconds = %d, want 60 (default)", cfg.LLM.TimeoutSeconds)
	}
	if cfg.LLM.OutputLanguage != "zh" {
		t.Errorf("LLM.OutputLanguage = %q, want zh (default)", cfg.LLM.OutputLanguage)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want 0.0.0.0 (default)", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080 (default)", cfg.Server.Port)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestLoadConfig_OpenAIProvider(t *testing.T) {
	yaml := `
watch:
  directories:
    - /photos
llm:
  provider: openai
  openai:
    api_key: sk-test123
    base_url: https://api.openai.com/v1
    model: gpt-4o
elasticsearch:
  addresses:
    - http://localhost:9200
  index_name: phosche
`
	path := writeTempYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.LLM.Provider != "openai" {
		t.Errorf("LLM.Provider = %q, want openai", cfg.LLM.Provider)
	}
	if cfg.LLM.OpenAI.APIKey != "sk-test123" {
		t.Errorf("LLM.OpenAI.APIKey = %q", cfg.LLM.OpenAI.APIKey)
	}
	if cfg.LLM.OpenAI.Model != "gpt-4o" {
		t.Errorf("LLM.OpenAI.Model = %q", cfg.LLM.OpenAI.Model)
	}
}
