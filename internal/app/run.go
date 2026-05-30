// Package app 负责 phosche 应用的装配与生命周期管理，包括依赖注入、组件启动和优雅关闭。
package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zwh8800/phosche/internal/analyzer"
	"github.com/zwh8800/phosche/internal/api"
	"github.com/zwh8800/phosche/internal/config"
	"github.com/zwh8800/phosche/internal/geocoder"
	"github.com/zwh8800/phosche/internal/indexer"
	"github.com/zwh8800/phosche/internal/pipeline"
	"github.com/zwh8800/phosche/internal/search"
	"github.com/zwh8800/phosche/internal/static"
	"github.com/zwh8800/phosche/internal/watcher"
)

// Run 启动 phosche 服务。依次执行：加载配置 → 配置日志 → 初始化 ES 客户端并创建索引 → 创建索引服务 → 创建 LLM 客户端 + 图片分析器 → 创建文件监控器 + 目录扫描器 → 组装处理流水线 → 启动 HTTP 服务器 → 等待信号优雅关闭。
func Run(distFS fs.FS, configPath string) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// 配置结构化日志（JSON 格式，可配置级别）
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Server.LogLevel),
	})))

	logStartupInfo(cfg)

	// 初始化 ES 客户端并创建索引
	esClient, err := indexer.NewESClient(cfg.Elasticsearch)
	if err != nil {
		slog.Error("failed to create ES client", "error", err)
		os.Exit(1)
	}

	ctx, esCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer esCancel()
	if err := esClient.EnsureIndex(ctx, cfg.Elasticsearch.IndexName); err != nil {
		slog.Error("failed to ensure ES index", "error", err)
		os.Exit(1)
	}

	// 创建索引服务（带断路器功能，队列容量 100）
	indexerSvc := indexer.NewIndexerService(esClient, 100)

	// 创建 LLM 客户端（通过工厂方法，支持 Ollama 和 OpenAI 两种后端）
	llmClient, err := analyzer.NewLLMClient(analyzer.LLMClientConfig{
		Provider: cfg.LLM.Provider,
		Ollama: analyzer.OllamaClientConfig{
			BaseURL: cfg.LLM.Ollama.BaseURL,
			Model:   cfg.LLM.Ollama.Model,
		},
		OpenAI: analyzer.OpenAIClientConfig{
			APIKey:  cfg.LLM.OpenAI.APIKey,
			BaseURL: cfg.LLM.OpenAI.BaseURL,
			Model:   cfg.LLM.OpenAI.Model,
		},
	})
	if err != nil {
		slog.Error("failed to create LLM client", "error", err)
		os.Exit(1)
	}

	// 创建图片分析器（配置重试次数和超时时间）
	imgAnalyzer := analyzer.NewImageAnalyzer(
		llmClient,
		"",
		cfg.LLM.MaxRetries,
		time.Duration(cfg.LLM.TimeoutSeconds)*time.Second,
	)

	geoCoder := geocoder.NewGeocoder(cfg.Env.AMAPKey)
	if cfg.Env.AMAPKey == "" {
		slog.Info("geocoding disabled: no amap_key configured")
	}

	// 创建文件监控器（基于 fsnotify，带去抖功能）和目录扫描器
	fsWatcher := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{
		DebounceMs: cfg.Watch.DebounceMs,
	})
	dirScanner := &watcher.DirectoryScanner{}

	// 组装处理流水线（文件监控 → 解码 → AI 分析 → ES 索引）
	pl := pipeline.NewPipeline(pipeline.PipelineConfig{
		Watcher:           fsWatcher,
		Scanner:           dirScanner,
		Analyzer:          imgAnalyzer,
		Geocoder:          geoCoder,
		Indexer:           indexerSvc,
		IndexName:         cfg.Elasticsearch.IndexName,
		Dirs:              cfg.Watch.Directories,
		Recursive:         cfg.Watch.Recursive,
		ExcludeDirs:       cfg.Watch.ExcludeDirs,
		Concurrency:       cfg.LLM.Concurrency,
	})

	pipelineCtx, pipelineCancel := context.WithCancel(context.Background())
	defer pipelineCancel()

	go func() {
		if err := pl.Run(pipelineCtx); err != nil && err != context.Canceled {
			slog.Error("pipeline exited with error", "error", err)
		}
	}()

	// 创建搜索服务（构建 ES 查询）
	searchSvc := search.NewSearchService(esClient)

	apiSrv := api.NewServer(searchSvc, indexerSvc, cfg.Elasticsearch.IndexName)
	router := api.NewRouter(apiSrv)

	photoHandler := static.PhotoHandler(cfg.Watch.Directories)

	httpHandler := newMux(router, photoHandler, distFS, cfg.Server.DevMode)

	// 启动 HTTP 服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: httpHandler,
	}

	go func() {
		slog.Info("HTTP server listening", "addr", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-sigCtx.Done()
		slog.Info("shutting down...")

	// 取消 pipeline 上下文，触发流水线优雅关闭
	pipelineCancel()

	// 使用 10 秒超时关闭 HTTP 服务器
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("phosche stopped")
}

// logStartupInfo 记录服务启动时的关键配置信息。
func logStartupInfo(cfg *config.Config) {
	slog.Info("phosche starting",
		"port", cfg.Server.Port,
		"es_addresses", cfg.Elasticsearch.Addresses,
		"llm_provider", cfg.LLM.Provider,
		"watch_dirs", cfg.Watch.Directories,
		"index", cfg.Elasticsearch.IndexName,
	)
}

// newMux 创建 HTTP 请求分发器，将请求路由到不同的处理器：/health 和 /api/* → chi 路由器；/photos/* → 静态照片服务；其他 → SPA 静态文件（生产模式）或 404（开发模式）。
func newMux(router http.Handler, photoHandler http.Handler, distFS fs.FS, devMode bool) http.Handler {
	var spa http.Handler
	if distFS != nil && !devMode {
		spa = spaHandler(distFS)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/api/"):
			router.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/photos/"):
			photoHandler.ServeHTTP(w, r)
		case !devMode && spa != nil:
			spa.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// spaHandler 提供 SPA 单页应用静态文件服务。对于不存在的文件路径，回退到 index.html 以支持客户端路由。
func spaHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			r.URL.Path = "/index.html"
			fileServer.ServeHTTP(w, r)
			return
		}
		cleanPath := strings.TrimPrefix(path, "/")
		if _, err := fs.Stat(distFS, cleanPath); err != nil {
			r.URL.Path = "/index.html"
		}
		fileServer.ServeHTTP(w, r)
	})
}

// parseLogLevel 将日志级别字符串转换为 slog.Level。
func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
