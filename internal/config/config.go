// Package config 提供基于 YAML 的配置管理，包括配置文件加载、默认值填充和配置校验。
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 是应用的根配置结构体，对应 config.yaml 的顶层字段。
type Config struct {
	Watch         WatchConfig  `yaml:"watch"`
	LLM           LLMConfig    `yaml:"llm"`
	OpenSearch    OSConfig      `yaml:"opensearch"`
	Server        ServerConfig `yaml:"server"`
	Env           EnvConfig       `yaml:"env"`
	Embedding     EmbeddingConfig `yaml:"embedding"`
}

// WatchConfig 是文件监控配置，控制照片目录的监控行为。
type WatchConfig struct {
	Directories []string `yaml:"directories"`    // 监控目录列表
	Recursive   bool     `yaml:"recursive"`      // 递归监控
	DebounceMs  int      `yaml:"debounce_ms"`    // 去抖间隔毫秒
	MinDirDepth int      `yaml:"min_dir_depth"`  // 最小深度
	ExcludeDirs    []string          `yaml:"exclude_dirs"`        // 排除的目录名列表
	SkipInitialScan bool            `yaml:"skip_initial_scan"`  // 跳过启动时扫描，默认 false（即扫描）
	PrivateDirs    map[string][]string `yaml:"private_dirs"`    // 私有目录及其授权用户邮箱列表
}

func (w WatchConfig) OwnerEmail(path string) string {
	for dir, emails := range w.PrivateDirs {
		if strings.HasPrefix(path, dir) {
			if len(emails) > 0 {
				return emails[0]
			}
		}
	}
	return ""
}

func (w WatchConfig) IsAuthorized(path string, userEmail string) bool {
	for dir, emails := range w.PrivateDirs {
		if strings.HasPrefix(path, dir) {
			for _, email := range emails {
				if email == userEmail {
					return true
				}
			}
			return false
		}
	}
	return true
}

// LLMConfig 是 AI 分析配置，控制 LLM 后端的选择和分析参数。
type LLMConfig struct {
	Provider       string       `yaml:"provider"`        // LLM 提供商，当前仅支持 "openai"（含 Ollama 等兼容协议）
	OpenAI         OpenAIConfig `yaml:"openai"`          // OpenAI 兼容协议配置
	MaxRetries     int          `yaml:"max_retries"`     // 分析失败时的最大重试次数
	Concurrency    int          `yaml:"concurrency"`     // 同时进行的 AI 分析任务数
	TimeoutSeconds int          `yaml:"timeout_seconds"` // 单次 AI 分析请求的超时时间（秒）
	OutputLanguage string       `yaml:"output_language"` // 输出语言，影响分析结果描述的语言
}

// OpenAIConfig 是 OpenAI 兼容协议配置（也适用于 Ollama 等本地 OpenAI 兼容服务）。
type OpenAIConfig struct {
	APIKey         string `yaml:"api_key"`          // API 密钥（可选，Ollama 等本地服务留空）
	BaseURL        string `yaml:"base_url"`         // API 地址，如 https://api.openai.com/v1 或 http://localhost:11434/v1
	Model          string `yaml:"model"`            // 模型名称，如 gpt-4o 或 llama3.2-vision
	ResponseFormat string `yaml:"response_format"`  // 响应格式：json_object（默认）、json_schema、text
}

// OSConfig 是 OpenSearch 连接配置。
type OSConfig struct {
	Addresses          []string `yaml:"addresses"`             // OpenSearch 节点地址列表
	Username           string   `yaml:"username"`              // OpenSearch 认证用户名
	Password           string   `yaml:"password"`              // OpenSearch 认证密码
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify"`  // 跳过 TLS 验证
	IndexName          string   `yaml:"index_name"`            // OpenSearch 索引名称
}

// ServerConfig 是 HTTP 服务器配置。
type ServerConfig struct {
	Host     string `yaml:"host"`      // 监听地址
	Port     int    `yaml:"port"`      // 监听端口
	DevMode  bool   `yaml:"dev_mode"`  // 开发模式开关
	LogLevel string `yaml:"log_level"` // 日志级别（debug/info/warn/error）
	CacheDir string `yaml:"cache_dir"` // 照片缓存目录，空表示不缓存（实时生成）
}

// EnvConfig 是环境变量配置，包含外部服务的 API Key 等。
type EnvConfig struct {
	AMAPKey string `yaml:"amap_key"`
}

// EmbeddingConfig 是文本向量化配置，控制 embedding 服务的选择和混合检索参数。
type EmbeddingConfig struct {
	Enabled        bool                  `yaml:"enabled"`         // 是否启用 embedding
	Provider       string                `yaml:"provider"`        // embedding 提供商，可选 "ollama" 或 "openai"
	Ollama         EmbeddingOllamaConfig `yaml:"ollama"`          // Ollama embedding 配置
	OpenAI         EmbeddingOpenAIConfig `yaml:"openai"`          // OpenAI embedding 配置
	SourceTemplate string                `yaml:"source_template"` // embedding 输入文本模板
	QueryCache     QueryCacheConfig      `yaml:"query_cache"`     // 查询 embedding 缓存配置
	Required       bool                  `yaml:"required"`        // embedding 失败是否阻塞文档落库
	MaxRetries     int                   `yaml:"max_retries"`     // 最大重试次数
	TimeoutSeconds int                   `yaml:"timeout_seconds"` // 单次请求超时时间（秒）
	Hybrid         HybridConfig          `yaml:"hybrid"`          // 混合检索参数
}

// EmbeddingOllamaConfig 是 Ollama embedding 配置。
type EmbeddingOllamaConfig struct {
	BaseURL    string `yaml:"base_url"`   // Ollama 服务地址
	Model      string `yaml:"model"`      // embedding 模型名称
	Dimensions int    `yaml:"dimensions"` // 向量维度
}

// EmbeddingOpenAIConfig 是 OpenAI embedding 配置。
type EmbeddingOpenAIConfig struct {
	APIKey     string `yaml:"api_key"`    // OpenAI API 密钥
	BaseURL    string `yaml:"base_url"`   // API 地址
	Model      string `yaml:"model"`      // 模型名称
	Dimensions int    `yaml:"dimensions"` // 向量维度（支持 Matryoshka 截断）
}

// QueryCacheConfig 是查询 embedding 缓存配置。
type QueryCacheConfig struct {
	Size       int `yaml:"size"`        // LRU 缓存大小
	TTLMinutes int `yaml:"ttl_minutes"` // 缓存过期时间（分钟）
}

// HybridConfig 是混合检索参数配置。
type HybridConfig struct {
	RRFRankConstant int `yaml:"rrf_rank_constant"` // RRF rank_constant，控制排名差异的权重衰减速度
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

	// Embedding defaults
	if cfg.Embedding.Ollama.BaseURL == "" {
		cfg.Embedding.Ollama.BaseURL = "http://localhost:11434"
	}
	if cfg.Embedding.Ollama.Model == "" {
		cfg.Embedding.Ollama.Model = "bge-m3"
	}
	if cfg.Embedding.Ollama.Dimensions == 0 {
		cfg.Embedding.Ollama.Dimensions = 1024
	}
	if cfg.Embedding.OpenAI.BaseURL == "" {
		cfg.Embedding.OpenAI.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Embedding.OpenAI.Model == "" {
		cfg.Embedding.OpenAI.Model = "text-embedding-3-small"
	}
	if cfg.Embedding.OpenAI.Dimensions == 0 {
		cfg.Embedding.OpenAI.Dimensions = 1024
	}
	if cfg.Embedding.QueryCache.Size == 0 {
		cfg.Embedding.QueryCache.Size = 512
	}
	if cfg.Embedding.QueryCache.TTLMinutes == 0 {
		cfg.Embedding.QueryCache.TTLMinutes = 60
	}
	if cfg.Embedding.MaxRetries == 0 {
		cfg.Embedding.MaxRetries = 2
	}
	if cfg.Embedding.TimeoutSeconds == 0 {
		cfg.Embedding.TimeoutSeconds = 15
	}
	if cfg.Embedding.Hybrid.RRFRankConstant == 0 {
		cfg.Embedding.Hybrid.RRFRankConstant = 60
	}
}

// validate 校验配置的必填项和枚举值合法性。
func validate(cfg *Config) error {
	// watch.directories 必须配置至少一个监控目录
	if len(cfg.Watch.Directories) == 0 {
		return fmt.Errorf("watch.directories: must not be empty")
	}

	// llm.provider 必须是 "openai"
	if cfg.LLM.Provider != "openai" {
		return fmt.Errorf("llm.provider: must be \"openai\", got %q", cfg.LLM.Provider)
	}

	// llm.openai.response_format 必须是 json_object、json_schema、text 之一（允许为空，等同 json_object）
	switch cfg.LLM.OpenAI.ResponseFormat {
	case "", "json_object", "json_schema", "text":
	default:
		return fmt.Errorf("llm.openai.response_format: must be one of [json_object, json_schema, text], got %q", cfg.LLM.OpenAI.ResponseFormat)
	}

	// opensearch.addresses 和 index_name 为必填项
	if len(cfg.OpenSearch.Addresses) == 0 {
		return fmt.Errorf("opensearch.addresses: must not be empty")
	}
	if cfg.OpenSearch.IndexName == "" {
		return fmt.Errorf("opensearch.index_name: must not be empty")
	}

	// Embedding validation (only when enabled)
	if cfg.Embedding.Enabled {
		switch cfg.Embedding.Provider {
		case "ollama", "openai":
		default:
			return fmt.Errorf("embedding.provider: must be one of [ollama, openai], got %q", cfg.Embedding.Provider)
		}

	}

	return nil
}
