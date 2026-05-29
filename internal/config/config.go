package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Watch         WatchConfig  `yaml:"watch"`
	LLM           LLMConfig    `yaml:"llm"`
	Elasticsearch ESConfig     `yaml:"elasticsearch"`
	Server        ServerConfig `yaml:"server"`
}

type WatchConfig struct {
	Directories []string `yaml:"directories"`
	Recursive   bool     `yaml:"recursive"`
	DebounceMs  int      `yaml:"debounce_ms"`
	MinDirDepth int      `yaml:"min_dir_depth"`
}

type LLMConfig struct {
	Provider       string       `yaml:"provider"`
	Ollama         OllamaConfig `yaml:"ollama"`
	OpenAI         OpenAIConfig `yaml:"openai"`
	Prompt         string       `yaml:"prompt"`
	MaxRetries     int          `yaml:"max_retries"`
	Concurrency    int          `yaml:"concurrency"`
	TimeoutSeconds int          `yaml:"timeout_seconds"`
	OutputLanguage string       `yaml:"output_language"`
}

type OllamaConfig struct {
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

type OpenAIConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

type ESConfig struct {
	Addresses          []string `yaml:"addresses"`
	Username           string   `yaml:"username"`
	Password           string   `yaml:"password"`
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify"`
	IndexName          string   `yaml:"index_name"`
}

type ServerConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	DevMode  bool   `yaml:"dev_mode"`
	LogLevel string `yaml:"log_level"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Watch.DebounceMs == 0 {
		cfg.Watch.DebounceMs = 500
	}
	if cfg.Watch.MinDirDepth == 0 {
		cfg.Watch.MinDirDepth = 1
	}
	if !cfg.Watch.Recursive {
		cfg.Watch.Recursive = true
	}

	if cfg.LLM.MaxRetries == 0 {
		cfg.LLM.MaxRetries = 3
	}
	if cfg.LLM.Concurrency == 0 {
		cfg.LLM.Concurrency = 2
	}
	if cfg.LLM.TimeoutSeconds == 0 {
		cfg.LLM.TimeoutSeconds = 60
	}
	if cfg.LLM.OutputLanguage == "" {
		cfg.LLM.OutputLanguage = "zh"
	}

	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.LogLevel == "" {
		cfg.Server.LogLevel = "info"
	}
}

func validate(cfg *Config) error {
	if len(cfg.Watch.Directories) == 0 {
		return fmt.Errorf("watch.directories: must not be empty")
	}

	switch cfg.LLM.Provider {
	case "ollama", "openai":
	default:
		return fmt.Errorf("llm.provider: must be one of [ollama, openai], got %q", cfg.LLM.Provider)
	}

	if len(cfg.Elasticsearch.Addresses) == 0 {
		return fmt.Errorf("elasticsearch.addresses: must not be empty")
	}
	if cfg.Elasticsearch.IndexName == "" {
		return fmt.Errorf("elasticsearch.index_name: must not be empty")
	}

	return nil
}
