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
	"github.com/zwh8800/phosche/internal/cache"
	"github.com/zwh8800/phosche/internal/config"
	"github.com/zwh8800/phosche/internal/embedder"
	"github.com/zwh8800/phosche/internal/geocoder"
	"github.com/zwh8800/phosche/internal/indexer"
	"github.com/zwh8800/phosche/internal/pipeline"
	"github.com/zwh8800/phosche/internal/search"
	"github.com/zwh8800/phosche/internal/static"
	"github.com/zwh8800/phosche/internal/watcher"
)

// Run 启动 phosche 服务。依次执行：加载配置 → 配置日志 → 初始化 OpenSearch 客户端并创建索引 → 创建索引服务 → 创建 LLM 客户端 + 图片分析器 → 创建文件监控器 + 目录扫描器 → 组装处理流水线 → 启动 HTTP 服务器 → 等待信号优雅关闭。
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

	// 初始化 OpenSearch 客户端并创建索引
	osClient, err := indexer.NewOSClient(cfg.OpenSearch)
	if err != nil {
		slog.Error("failed to create OpenSearch client", "error", err)
		os.Exit(1)
	}

	// 提前计算 embedding 维度，用于创建 OpenSearch 索引映射
	embeddingDims := 0
	if cfg.Embedding.Enabled {
		switch cfg.Embedding.Provider {
		case "ollama":
			embeddingDims = cfg.Embedding.Ollama.Dimensions
		case "openai":
			embeddingDims = cfg.Embedding.OpenAI.Dimensions
		}
	}

	ctx, osCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer osCancel()
	if err := osClient.EnsureIndex(ctx, cfg.OpenSearch.IndexName, embeddingDims); err != nil {
		slog.Error("failed to ensure OpenSearch index", "error", err)
		os.Exit(1)
	}

	// 创建索引服务（带断路器功能，队列容量 100）
	indexerSvc := indexer.NewIndexerService(osClient, 100)

	llmClient := analyzer.NewLLMClient(analyzer.LLMClientConfig{
		BaseURL: cfg.LLM.OpenAI.BaseURL,
		Model:   cfg.LLM.OpenAI.Model,
		APIKey:  cfg.LLM.OpenAI.APIKey,
	})

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

	// 创建 embedding 客户端（可选，用于混合检索）
	var embService *embedder.EmbeddingService
	var embCache *embedder.EmbeddingCache
	var embeddingVersion string

	if cfg.Embedding.Enabled {
		embClient, err := embedder.NewEmbeddingClient(embedder.EmbeddingClientConfig{
			Provider: cfg.Embedding.Provider,
			Ollama: embedder.OllamaEmbeddingConfig{
				BaseURL: cfg.Embedding.Ollama.BaseURL,
				Model:   cfg.Embedding.Ollama.Model,
			},
			OpenAI: embedder.OpenAIEmbeddingConfig{
				APIKey:  cfg.Embedding.OpenAI.APIKey,
				BaseURL: cfg.Embedding.OpenAI.BaseURL,
				Model:   cfg.Embedding.OpenAI.Model,
			},
			Dimensions: embeddingDims,
		})
		if err != nil {
			slog.Error("failed to create embedding client", "error", err)
			os.Exit(1)
		}

		embService = embedder.NewEmbeddingService(
			embClient,
			nil,
			cfg.Embedding.MaxRetries,
			time.Duration(cfg.Embedding.TimeoutSeconds)*time.Second,
		)

		if cfg.Embedding.QueryCache.Size > 0 {
			embCache = embedder.NewEmbeddingCache(
				cfg.Embedding.QueryCache.Size,
				time.Duration(cfg.Embedding.QueryCache.TTLMinutes)*time.Minute,
			)
		}

		var modelName string
		switch cfg.Embedding.Provider {
		case "ollama":
			modelName = cfg.Embedding.Ollama.Model
		case "openai":
			modelName = cfg.Embedding.OpenAI.Model
		}
		embeddingVersion = fmt.Sprintf("%s@%d", modelName, embeddingDims)

		slog.Info("embedding enabled",
			"provider", cfg.Embedding.Provider,
			"model", modelName,
			"dimensions", embeddingDims,
		)
	}

	cacheGen := cache.NewGenerator(cfg.Server.CacheDir)

	// 创建文件监控器（基于 fsnotify，带去抖功能）和目录扫描器
	fsWatcher := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{
		DebounceMs: cfg.Watch.DebounceMs,
	})
	dirScanner := &watcher.DirectoryScanner{}

	// 组装处理流水线（文件监控 → 解码 → AI 分析 → OpenSearch 索引）
	pipeCfg := pipeline.PipelineConfig{
		Watcher:           fsWatcher,
		Scanner:           dirScanner,
		Analyzer:          imgAnalyzer,
		Geocoder:          geoCoder,
		Indexer:           indexerSvc,
		Cache:             cacheGen,
		IndexName:         cfg.OpenSearch.IndexName,
		Dirs:              cfg.Watch.Directories,
		Recursive:         cfg.Watch.Recursive,
		ExcludeDirs:       cfg.Watch.ExcludeDirs,
		PrivateDirs:       cfg.Watch.PrivateDirs,
		Concurrency:       cfg.LLM.Concurrency,
		SkipInitialScan:   cfg.Watch.SkipInitialScan,
		EmbeddingVersion:  embeddingVersion,
		EmbedSourceTemplate: cfg.Embedding.SourceTemplate,
	}
	if embService != nil {
		pipeCfg.Embedder = embService
	}
	pl := pipeline.NewPipeline(pipeCfg)

	pipelineCtx, pipelineCancel := context.WithCancel(context.Background())
	defer pipelineCancel()

	go func() {
		if err := pl.Run(pipelineCtx); err != nil && err != context.Canceled {
			slog.Error("pipeline exited with error", "error", err)
		}
	}()

	// 创建搜索服务（构建 OpenSearch 查询，支持原生 RRF）
	searchOpts := []search.SearchOption{}
	if embService != nil {
		searchOpts = append(searchOpts, search.WithEmbedder(embService, embCache, search.HybridConfig{
			RRFRankConstant: cfg.Embedding.Hybrid.RRFRankConstant,
		}))
	}
	searchSvc := search.NewSearchService(osClient, searchOpts...)

	apiSrv := api.NewServer(searchSvc, indexerSvc, cfg.OpenSearch.IndexName)
	router := api.NewRouter(apiSrv)

	photoHandler := static.PhotoHandler(cfg.Watch.Directories, cacheGen)
	photoHandler = wrapPrivateAccess(photoHandler, cfg.Watch)

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
		"opensearch_addresses", cfg.OpenSearch.Addresses,
		"llm_provider", cfg.LLM.Provider,
		"llm_model", cfg.LLM.OpenAI.Model,
		"watch_dirs", cfg.Watch.Directories,
		"index", cfg.OpenSearch.IndexName,
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

// wrapPrivateAccess 为私有目录的静态文件提供访问控制。
// 若请求路径属于私有目录，但 JWT email 不匹配 → 403。
func wrapPrivateAccess(next http.Handler, wCfg config.WatchConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !wCfg.IsAuthorized(r.URL.Path, api.UserEmailFromContext(r.Context())) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// spaHandler 提供 SPA 单页应用静态文件服务。对于不存在的文件路径，回退到 index.html 以支持客户端路由。
// http.FileServer 会将 /index.html 自动重定向到 ./，导致无限循环，因此 index.html 需要单独处理。
func spaHandler(distFS fs.FS) http.Handler {
	subFS, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		slog.Error("failed to create sub filesystem for SPA", "error", err)
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(subFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(r.URL.Path, "/")

		if cleanPath == "" || cleanPath == "index.html" {
			serveIndexHTML(w, subFS)
			return
		}

		if f, err := subFS.Open(cleanPath); err == nil {
			defer f.Close()
			if stat, err := f.Stat(); err == nil && !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		serveIndexHTML(w, subFS)
	})
}

// serveIndexHTML 绕过 http.FileServer 直接提供 index.html，避免其对 /index.html → ./ 的重定向。
func serveIndexHTML(w http.ResponseWriter, fsys fs.FS) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
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
