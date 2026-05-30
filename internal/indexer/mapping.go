package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// mappingVersion 追踪当前索引映射的版本号，用于迁移检测。
// 当映射结构发生变化时，应递增此版本号。启动时若检测到 ES 中
// 已有索引的 _meta.version 与此不一致，会发出告警但不会自动迁移。
const mappingVersion = "3"

// indexMapping 定义 ES 索引的 settings 和 mappings。
//   - number_of_shards: 1（单分片，适用于单节点部署）
//   - number_of_replicas: 0（无副本，单节点下避免 unassigned shards）
//
// 字段映射一览：
//   | 字段名              | ES 类型  | 说明                                    |
//   |---------------------|----------|----------------------------------------|
//   | description         | text     | 照片描述，支持全文搜索                     |
//   | tags                | text + .keyword | 标签，text 用于全文搜索，.keyword 用于精确聚合 |
//   | objects             | text + .keyword | 检测到的物体，同上双字段模式           |
//   | scene_type          | keyword  | 场景类型（indoor/outdoor/unknown），精确匹配  |
//   | camera_model        | keyword  | 相机型号，精确匹配                         |
//   | date_time_original  | date     | 原始拍摄日期时间                           |
//   | colors              | nested  | 主体颜色对象数组（name+hex）              |
//   | people_count        | integer  | 画面中检测到的人物数量                     |
//   | has_text            | boolean  | 画面中是否包含文字                         |
//   | text                | text     | OCR 提取的文本内容，支持全文搜索            |
//   | status              | keyword  | 处理状态（unanalyzed/analyzing/analyzed 等）|
//   | path                | keyword  | 照片文件路径                               |
//   | mtime               | long     | 文件修改时间（Unix 时间戳）                 |
//   | size                | long     | 文件大小（字节）                           |
//   | created_at          | date     | 文档创建时间                               |
//   | gps_lat             | double   | GPS 纬度                                  |
//   | gps_lon             | double   | GPS 经度                                  |
//   | country             | keyword  | 国家（Amap 逆地理编码）                    |
//   | province            | keyword  | 省份（Amap 逆地理编码）                    |
//   | city                | keyword  | 城市（Amap 逆地理编码）                    |
//   | district            | keyword  | 区县（Amap 逆地理编码）                    |
//   | address             | text     | 详细地址（Amap 逆地理编码）                |
//   | formatted_address   | text     | 格式化地址（Amap 逆地理编码）              |
//
// _meta.version 用于运行时检测映射版本是否匹配，发现不匹配时仅日志告警，不自动重建。
var indexMapping = map[string]any{
	"settings": map[string]any{
		"number_of_shards":   1,
		"number_of_replicas": 0,
	},
	"mappings": map[string]any{
		"_meta": map[string]any{
			"version": mappingVersion,
		},
		"properties": map[string]any{
			"description":        map[string]any{"type": "text"},
			"tags":               textWithKeyword,
			"objects":            textWithKeyword,
			"scene_type":         map[string]any{"type": "keyword"},
			"camera_model":       map[string]any{"type": "keyword"},
			"date_time_original": map[string]any{"type": "date"},
			"colors": map[string]any{
				"type": "nested",
				"properties": map[string]any{
					"name": map[string]any{"type": "keyword"},
					"hex":  map[string]any{"type": "keyword"},
				},
			},
			"people_count":       map[string]any{"type": "integer"},
			"has_text":           map[string]any{"type": "boolean"},
			"text":               map[string]any{"type": "text"},
			"status":             map[string]any{"type": "keyword"},
			"path":               map[string]any{"type": "keyword"},
			"mtime":              map[string]any{"type": "long"},
			"size":               map[string]any{"type": "long"},
			"created_at":         map[string]any{"type": "date"},
			"gps_lat":            map[string]any{"type": "double"},
			"gps_lon":            map[string]any{"type": "double"},
			"country":            map[string]any{"type": "keyword"},
			"province":           map[string]any{"type": "keyword"},
			"city":               map[string]any{"type": "keyword"},
			"district":           map[string]any{"type": "keyword"},
			"address":            map[string]any{"type": "text"},
			"formatted_address":  map[string]any{"type": "text"},
		},
	},
}

// textWithKeyword 是可复用的字段映射模板，用于需要同时支持两种查询模式的字段：
//   - text 类型：支持全文搜索（分词、相关性评分）
//   - .keyword 子字段：支持精确匹配和聚合（terms aggregation）
//
// 典型用法：tags、objects 字段。搜索时使用 tags 做全文匹配，筛选时使用 tags.keyword 做精确过滤。
var textWithKeyword = map[string]any{
	"type": "text",
	"fields": map[string]any{
		"keyword": map[string]any{"type": "keyword"},
	},
}

// EnsureIndex 确保 ES 索引存在且映射版本匹配。
//
// 流程：
//  1. 调用 indexExists 检查索引是否存在
//  2. 若存在 → 调用 checkMappingVersion 校验 _meta.version 是否匹配
//  3. 若不存在 → 调用 createIndex 使用当前 indexMapping 创建索引
//
// 注意：当映射版本不匹配时仅发出日志告警，不会自动执行迁移或重建。
func (c *ESClient) EnsureIndex(ctx context.Context, indexName string) error {
	exists, err := c.indexExists(ctx, indexName)
	if err != nil {
		return err
	}
	if exists {
		return c.checkMappingVersion(ctx, indexName)
	}
	return c.createIndex(ctx, indexName)
}

// indexExists 通过 ES IndicesExists API 检查指定索引是否存在。
// 返回 (true, nil) 表示存在，(false, nil) 表示不存在（404），其他状态码视为错误。
func (c *ESClient) indexExists(ctx context.Context, indexName string) (bool, error) {
	req := esapi.IndicesExistsRequest{
		Index: []string{indexName},
	}
	resp, err := req.Do(ctx, c.client)
	if err != nil {
		return false, fmt.Errorf("check index exists: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		return true, nil
	case 404:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status checking index %q: %s", indexName, resp.Status())
	}
}

// checkMappingVersion 获取 ES 中已有索引的映射，提取 _meta.version 并与当前 mappingVersion 比较。
// 如果版本缺失或不一致，会通过日志发出告警（Warn 级别），但不会阻止服务启动。
func (c *ESClient) checkMappingVersion(ctx context.Context, indexName string) error {
	req := esapi.IndicesGetMappingRequest{
		Index: []string{indexName},
	}
	resp, err := req.Do(ctx, c.client)
	if err != nil {
		return fmt.Errorf("get mapping: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("get mapping returned %s: %s", resp.Status(), string(b))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode mapping response: %w", err)
	}

	version := extractMetaVersion(result, indexName)
	if version == "" {
		c.logger.Warn("index mapping has no _meta.version, consider recreating index",
			"expected_version", mappingVersion,
			"index", indexName,
		)
	} else if version != mappingVersion {
		c.logger.Warn("index mapping version mismatch, consider recreating index",
			"expected_version", mappingVersion,
			"actual_version", version,
			"index", indexName,
		)
	}
	return nil
}

func extractMetaVersion(result map[string]any, indexName string) string {
	idx, ok := result[indexName].(map[string]any)
	if !ok {
		return ""
	}
	mappings, ok := idx["mappings"].(map[string]any)
	if !ok {
		return ""
	}
	meta, ok := mappings["_meta"].(map[string]any)
	if !ok {
		return ""
	}
	v, ok := meta["version"].(string)
	if !ok {
		return ""
	}
	return v
}

func (c *ESClient) createIndex(ctx context.Context, indexName string) error {
	body, err := json.Marshal(indexMapping)
	if err != nil {
		return fmt.Errorf("marshal mapping: %w", err)
	}

	req := esapi.IndicesCreateRequest{
		Index: indexName,
		Body:  bytes.NewReader(body),
	}
	resp, err := req.Do(ctx, c.client)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create index returned %s: %s", resp.Status(), string(b))
	}
	return nil
}
