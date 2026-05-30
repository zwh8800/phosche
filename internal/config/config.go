// Package config 提供基于 YAML 的配置管理，包括配置文件加载、默认值填充和配置校验。
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 是应用的根配置结构体，对应 config.yaml 的顶层字段。
type Config struct {
	Watch         WatchConfig  `yaml:"watch"`
	LLM           LLMConfig    `yaml:"llm"`
	Elasticsearch ESConfig     `yaml:"elasticsearch"`
	Server        ServerConfig `yaml:"server"`
	Env           EnvConfig    `yaml:"env"`
}

// WatchConfig 是文件监控配置，控制照片目录的监控行为。
type WatchConfig struct {
	Directories []string `yaml:"directories"` // 监控目录列表
	Recursive   bool     `yaml:"recursive"`   // 递归监控
	DebounceMs  int      `yaml:"debounce_ms"` // 去抖间隔毫秒
	MinDirDepth int      `yaml:"min_dir_depth"` // 最小深度
}

// LLMConfig 是 AI 分析配置，控制 LLM 后端的选择和分析参数。
type LLMConfig struct {
	Provider       string       `yaml:"provider"`        // LLM 提供商，可选 "ollama" 或 "openai"
	Ollama         OllamaConfig `yaml:"ollama"`          // Ollama 本地模型配置
	OpenAI         OpenAIConfig `yaml:"openai"`          // OpenAI（或兼容接口）云端 API 配置
	MaxRetries     int          `yaml:"max_retries"`     // 分析失败时的最大重试次数
	Concurrency    int          `yaml:"concurrency"`     // 同时进行的 AI 分析任务数
	TimeoutSeconds int          `yaml:"timeout_seconds"` // 单次 AI 分析请求的超时时间（秒）
	OutputLanguage string       `yaml:"output_language"` // 输出语言，影响分析结果描述的语言
}

// OllamaConfig 是 Ollama 本地模型配置。
type OllamaConfig struct {
	BaseURL string `yaml:"base_url"` // Ollama 服务地址，如 http://localhost:11434
	Model   string `yaml:"model"`    // 视觉模型名称，如 llama3.2-vision
}

// OpenAIConfig 是 OpenAI（或兼容接口）云端 API 配置。
type OpenAIConfig struct {
	APIKey  string `yaml:"api_key"`  // OpenAI API 密钥
	BaseURL string `yaml:"base_url"` // API 地址，可替换为兼容的第三方接口
	Model   string `yaml:"model"`    // 模型名称，如 gpt-4o
}

// ESConfig 是 Elasticsearch 连接配置。
type ESConfig struct {
	Addresses          []string `yaml:"addresses"`             // ES 节点地址列表
	Username           string   `yaml:"username"`              // ES 认证用户名
	Password           string   `yaml:"password"`              // ES 认证密码
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify"`  // 跳过 TLS 验证
	IndexName          string   `yaml:"index_name"`            // ES 索引名称
}

// ServerConfig 是 HTTP 服务器配置。
type ServerConfig struct {
	Host     string `yaml:"host"`      // 监听地址
	Port     int    `yaml:"port"`      // 监听端口
	DevMode  bool   `yaml:"dev_mode"`  // 开发模式开关
	LogLevel string `yaml:"log_level"` // 日志级别（debug/info/warn/error）
}

// EnvConfig 是环境变量配置，包含外部服务的 API Key 等。
type EnvConfig struct {
	AMAPKey string `yaml:"amap_key"`
}

// LoadConfig 加载并解析 YAML 配置文件，应用默认值，校验必填项。
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

// applyDefaults 为未设置的配置项填充默认值。
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

// validate 校验配置的必填项和枚举值合法性。
func validate(cfg *Config) error {
	// watch.directories 必须配置至少一个监控目录
	if len(cfg.Watch.Directories) == 0 {
		return fmt.Errorf("watch.directories: must not be empty")
	}

	// llm.provider 必须是 "ollama" 或 "openai"
	switch cfg.LLM.Provider {
	case "ollama", "openai":
	default:
		return fmt.Errorf("llm.provider: must be one of [ollama, openai], got %q", cfg.LLM.Provider)
	}

	// elasticsearch.addresses 和 index_name 为必填项
	if len(cfg.Elasticsearch.Addresses) == 0 {
		return fmt.Errorf("elasticsearch.addresses: must not be empty")
	}
	if cfg.Elasticsearch.IndexName == "" {
		return fmt.Errorf("elasticsearch.index_name: must not be empty")
	}

	return nil
}
