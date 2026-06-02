// Package indexer 提供 OpenSearch 索引服务，包括连接管理、索引映射、文档 CRUD 操作和断路器保护。
package indexer

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"github.com/zwh8800/phosche/internal/config"
)

// OSClient 封装 opensearchapi.Client 并附带日志记录能力。
// 提供连接创建、TLS 跳过验证、连通性健康检查等功能。
type OSClient struct {
	client *opensearchapi.Client // OpenSearch API 客户端，提供底层 API 调用
	logger *slog.Logger          // 结构化日志记录器
}

// NewOSClient 根据配置创建 OSClient，使用默认日志记录器。
// 内部调用 NewOSClientWithLogger，会 ping OpenSearch 以验证连通性，失败时返回错误。
func NewOSClient(cfg config.OSConfig) (*OSClient, error) {
	return NewOSClientWithLogger(cfg, slog.Default())
}

// NewOSClientWithLogger 使用自定义日志记录器创建 OSClient。
// 流程：
//  1. 根据配置构建 opensearch.Config（地址、用户名、密码）
//  2. 如 InsecureSkipVerify 为 true，设置自定义 HTTP Transport 跳过 TLS 证书验证
//  3. 创建 OpenSearch 客户端实例
//  4. 调用 Info API 进行连通性检查（ping）
//  5. 全部通过后返回封装好的 OSClient
func NewOSClientWithLogger(cfg config.OSConfig, logger *slog.Logger) (*OSClient, error) {
	osCfg := opensearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
	}

	if cfg.InsecureSkipVerify {
		osCfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	client, err := opensearchapi.NewClient(opensearchapi.Config{Client: osCfg})
	if err != nil {
		return nil, fmt.Errorf("create opensearch client: %w", err)
	}

	// Ping: use Info API to verify connectivity and log cluster info
	resp, err := client.Info(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("ping opensearch: %w", err)
	}

	logger.Info("connected to opensearch",
		"cluster", resp.ClusterName,
		"version", resp.Version.Number,
	)

	return &OSClient{client: client, logger: logger}, nil
}

// Client 返回底层的 opensearchapi.Client 实例，供需要直接调用 OpenSearch API 的场景使用。
func (c *OSClient) Client() *opensearchapi.Client {
	return c.client
}
