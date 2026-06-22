// Package indexer 提供 OpenSearch 索引迁移框架，支持逐版本增量迁移而非删除重建。
package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"github.com/zwh8800/phosche/internal/config"
)

// MigrationFunc 是版本迁移函数的类型定义。
// 迁移脚本自行根据 cfg 创建所需的外部依赖（如 embedding 客户端、geocoder 等）。
// 返回 nil 表示迁移成功，返回 error 表示迁移失败（服务将降级启动）。
type MigrationFunc func(ctx context.Context, client *OSClient, indexName string, cfg *config.Config) error

// Migration 描述一个从 version-1 到 version 的增量迁移。
type Migration struct {
	// Version 是迁移的目标版本号（从 version-1 迁移到 version）。
	Version int

	// Mapping 是增量 mapping 定义，将通过 PUT mapping API 应用到已有索引。
	// 格式为 OpenSearch PUT mapping 请求体的 "properties" 部分，例如：
	//   map[string]any{
	//       "new_field": map[string]any{"type": "keyword"},
	//   }
	// 为 nil 表示此版本不需要新增 mapping 字段（仅数据回填或 _meta 更新）。
	Mapping map[string]any

	// Migrate 是数据回填函数，在 mapping 更新后执行。
	// 典型用途：遍历已有文档，调用外部服务生成新字段的值并更新到 OpenSearch。
	// 为 nil 表示此版本不需要数据回填（仅 mapping 变更）。
	Migrate MigrationFunc
}

// migrations 是全局迁移注册表，key 为目标版本号。
// 新增版本迁移时，在此注册表中添加条目即可。
// 例如：当 MappingVersion 从 9 升级到 10 时，添加 migrations[10]。
var migrations = map[int]Migration{}

// GetMigrations 返回当前注册的所有迁移（用于测试和调试）。
func GetMigrations() map[int]Migration {
	return migrations
}

// RegisterMigration 注册一个版本迁移到全局注册表。
// 通常在包初始化时调用（init 函数或变量声明），也可在测试中使用。
func RegisterMigration(m Migration) {
	migrations[m.Version] = m
}

// runMigrations 从 fromVersion 逐版本执行迁移到 MappingVersion。
// 迁移按版本号升序依次执行：fromVersion → fromVersion+1 → ... → MappingVersion。
//
// 每个迁移步骤：
//  1. 如果注册表中有该版本的迁移 → 执行 mapping 更新 + 数据回填
//  2. 如果注册表中没有 → 仅更新 _meta.version（纯版本号变更，无实际迁移逻辑）
//  3. 更新 OpenSearch 索引的 _meta.version 为当前迁移的目标版本
//
// 迁移失败时记录错误日志但不中断后续迁移和服务启动（降级模式）。
// 已完成的迁移步骤（_meta.version 已更新）不会被重复执行。
func (c *OSClient) runMigrations(ctx context.Context, indexName string, fromVersion int, cfg *config.Config) error {
	if fromVersion >= MappingVersion() {
		return nil
	}

	c.logger.Info("starting incremental mapping migration",
		"from_version", fromVersion,
		"to_version", MappingVersion(),
		"index", indexName,
	)

	for v := fromVersion + 1; v <= MappingVersion(); v++ {
		migration, exists := migrations[v]

		if exists {
			c.logger.Info("executing migration",
				"target_version", v,
				"has_mapping", migration.Mapping != nil,
				"has_migrate_func", migration.Migrate != nil,
			)

			// Step 1: Apply incremental mapping (PUT mapping API)
			if migration.Mapping != nil {
				if err := c.applyMapping(ctx, indexName, migration.Mapping); err != nil {
					c.logger.Error("migration mapping update failed, skipping to next version",
						"target_version", v,
						"error", err,
					)
					// Mapping 更新失败时跳过数据回填，不更新 _meta.version
					// 下次启动时会重新尝试
					continue
				}
			}

			// Step 2: Execute data backfill
			if migration.Migrate != nil {
				if err := migration.Migrate(ctx, c, indexName, cfg); err != nil {
					c.logger.Error("migration data backfill failed",
						"target_version", v,
						"error", err,
					)
					// 数据回填失败，不更新 _meta.version，下次启动会重试
					continue
				}
			}
		} else {
			c.logger.Info("no migration registered for version, updating _meta only",
				"target_version", v,
			)
		}

		// Step 3: Update _meta.version to mark this migration step as complete
		if err := c.updateMetaVersion(ctx, indexName, v); err != nil {
			c.logger.Error("failed to update _meta.version after migration",
				"target_version", v,
				"error", err,
			)
			return fmt.Errorf("update _meta.version to %d: %w", v, err)
		}

		c.logger.Info("migration step completed",
			"target_version", v,
		)
	}

	c.logger.Info("all migrations completed",
		"from_version", fromVersion,
		"to_version", MappingVersion(),
	)
	return nil
}

// applyMapping 使用 PUT mapping API 将增量 mapping 应用到已有索引。
// properties 是 OpenSearch PUT mapping 请求体中的 "properties" 部分。
func (c *OSClient) applyMapping(ctx context.Context, indexName string, properties map[string]any) error {
	mappingBody := map[string]any{
		"properties": properties,
	}
	bodyBytes, err := json.Marshal(mappingBody)
	if err != nil {
		return fmt.Errorf("marshal mapping body: %w", err)
	}

	_, err = c.client.Indices.Mapping.Put(ctx, opensearchapi.MappingPutReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
	})
	if err != nil {
		return fmt.Errorf("put mapping: %w", err)
	}

	c.logger.Info("incremental mapping applied", "index", indexName)
	return nil
}

// updateMetaVersion 更新索引的 _meta.version 字段，标记迁移步骤完成。
// 使用 PUT mapping API 更新 _meta，不会影响已有的字段映射。
func (c *OSClient) updateMetaVersion(ctx context.Context, indexName string, version int) error {
	metaBody := map[string]any{
		"_meta": map[string]any{
			"version": version,
		},
	}
	bodyBytes, err := json.Marshal(metaBody)
	if err != nil {
		return fmt.Errorf("marshal _meta body: %w", err)
	}

	_, err = c.client.Indices.Mapping.Put(ctx, opensearchapi.MappingPutReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
	})
	if err != nil {
		return fmt.Errorf("put _meta mapping: %w", err)
	}

	c.logger.Debug("_meta.version updated", "version", version, "index", indexName)
	return nil
}
