// Package indexer 提供 Elasticsearch 索引服务，包括连接管理、索引映射、文档 CRUD 操作和断路器保护。
package indexer

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/zwh8800/phosche/internal/config"
)

// ESClient 封装 elasticsearch.Client 并附带日志记录能力。
// 提供连接创建、TLS 跳过验证、连通性健康检查等功能。
type ESClient struct {
	client *elasticsearch.Client // ES 原生客户端，提供底层 API 调用
	logger *slog.Logger          // 结构化日志记录器
}

// NewESClient 根据配置创建 ESClient，使用默认日志记录器。
// 内部调用 NewESClientWithLogger，会 ping ES 以验证连通性，失败时返回错误。
func NewESClient(cfg config.ESConfig) (*ESClient, error) {
	return NewESClientWithLogger(cfg, slog.Default())
}

// NewESClientWithLogger 使用自定义日志记录器创建 ESClient。
// 流程：
//  1. 根据配置构建 elasticsearch.Config（地址、用户名、密码）
//  2. 如 InsecureSkipVerify 为 true，设置自定义 HTTP Transport 跳过 TLS 证书验证
//  3. 创建 ES 客户端实例
//  4. 调用 Info API 进行连通性检查（ping）
//  5. 全部通过后返回封装好的 ESClient
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

// Client 返回底层的 elasticsearch.Client 实例，供需要直接调用 ES API 的场景使用。
func (c *ESClient) Client() *elasticsearch.Client {
	return c.client
}
