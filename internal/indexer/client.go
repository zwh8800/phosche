package indexer

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/zwh8800/phosche/internal/config"
)

// ESClient wraps the Elasticsearch client with logging support.
type ESClient struct {
	client *elasticsearch.Client
	logger *slog.Logger
}

// NewESClient creates a new ESClient from the given config.
// It pings Elasticsearch to verify connectivity before returning.
func NewESClient(cfg config.ESConfig) (*ESClient, error) {
	return NewESClientWithLogger(cfg, slog.Default())
}

// NewESClientWithLogger creates a new ESClient with a custom logger.
func NewESClientWithLogger(cfg config.ESConfig, logger *slog.Logger) (*ESClient, error) {
	esCfg := elasticsearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
	}

	if cfg.InsecureSkipVerify {
		esCfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	client, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return nil, fmt.Errorf("create es client: %w", err)
	}

	resp, err := client.Info()
	if err != nil {
		return nil, fmt.Errorf("ping es: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return nil, fmt.Errorf("ping es returned: %s", resp.Status())
	}

	return &ESClient{client: client, logger: logger}, nil
}

// Client returns the underlying elasticsearch client.
func (c *ESClient) Client() *elasticsearch.Client {
	return c.client
}
