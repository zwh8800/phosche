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
	"github.com/zwh8800/phosche/internal/indexer"
	"github.com/zwh8800/phosche/internal/pipeline"
	"github.com/zwh8800/phosche/internal/search"
	"github.com/zwh8800/phosche/internal/static"
	"github.com/zwh8800/phosche/internal/watcher"
)

func Run(distFS fs.FS, configPath string) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Server.LogLevel),
	})))

	logStartupInfo(cfg)

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

	indexerSvc := indexer.NewIndexerService(esClient, 100)

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

	imgAnalyzer := analyzer.NewImageAnalyzer(
		llmClient,
		cfg.LLM.Prompt,
		cfg.LLM.MaxRetries,
		time.Duration(cfg.LLM.TimeoutSeconds)*time.Second,
	)

	fsWatcher := watcher.NewFSNotifyWatcher(watcher.WatcherConfig{
		DebounceMs: cfg.Watch.DebounceMs,
	})
	dirScanner := &watcher.DirectoryScanner{}

	pl := pipeline.NewPipeline(pipeline.PipelineConfig{
		Watcher:           fsWatcher,
		Scanner:           dirScanner,
		Analyzer:          imgAnalyzer,
		Indexer:           indexerSvc,
		IndexName:         cfg.Elasticsearch.IndexName,
		Dirs:              cfg.Watch.Directories,
		Recursive:         cfg.Watch.Recursive,
		Concurrency:       cfg.LLM.Concurrency,
	})

	pipelineCtx, pipelineCancel := context.WithCancel(context.Background())
	defer pipelineCancel()

	go func() {
		if err := pl.Run(pipelineCtx); err != nil && err != context.Canceled {
			slog.Error("pipeline exited with error", "error", err)
		}
	}()

	searchSvc := search.NewSearchService(esClient)

	apiSrv := api.NewServer(searchSvc, indexerSvc, cfg.Elasticsearch.IndexName)
	router := api.NewRouter(apiSrv)

	photoHandler := static.PhotoHandler(cfg.Server.PhotoBasePath)

	httpHandler := newMux(router, photoHandler, distFS, cfg.Server.DevMode)

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

	pipelineCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("phosche stopped")
}

func logStartupInfo(cfg *config.Config) {
	slog.Info("phosche starting",
		"port", cfg.Server.Port,
		"es_addresses", cfg.Elasticsearch.Addresses,
		"llm_provider", cfg.LLM.Provider,
		"watch_dirs", cfg.Watch.Directories,
		"index", cfg.Elasticsearch.IndexName,
	)
}

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
