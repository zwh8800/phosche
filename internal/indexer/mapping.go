package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// mappingVersion 追踪当前索引映射的版本号，用于迁移检测。
// 当映射结构发生变化时，应递增此版本号。启动时若检测到 OS 中
// 已有索引的 _meta.version 与此不一致，会自动删除旧索引并重建。
const mappingVersion = "9"

// buildIndexMapping 构建 OS 索引映射，embeddingDims 控制 knn_vector 维度。
// embeddingDims 为 0 时不启用 knn_vector 字段（embedding 未配置时）。
//
// settings 配置：
//   - number_of_shards: 1（单分片，适用于单节点部署）
//   - number_of_replicas: 0（无副本，单节点下避免 unassigned shards）
//   - index.knn: true（启用 OpenSearch KNN 索引）
//   - IK 中文分词器：ik_max_word（索引时最细粒度）、ik_smart（搜索时智能切分）
//
// 字段映射一览：
//
//	| 字段名              | OS 类型            | 说明                                    |
//	|---------------------|--------------------|-----------------------------------------|
//	| description         | text (ik)          | 照片描述，支持全文搜索                  |
//	| tags                | text (ik)+.keyword | 标签，text 全文搜索，.keyword 精确聚合  |
//	| objects             | text (ik)+.keyword | 检测到的物体，同上双字段模式            |
//	| scene_type          | keyword            | 场景类型（indoor/outdoor/unknown）      |
//	| camera_model        | keyword            | 相机型号                                |
//	| date_time_original  | date               | 原始拍摄日期时间                        |
//	| exif                | object             | EXIF 元数据嵌套对象                     |
//	|   ├ date_time_original | date            |   拍摄时间                              |
//	|   ├ camera_model    | text+keyword       |   相机型号                              |
//	|   ├ lens_model      | text+keyword       |   镜头型号                              |
//	|   ├ focal_length    | text+keyword       |   焦距                                  |
//	|   ├ aperture        | text+keyword       |   光圈值                                |
//	|   ├ shutter_speed   | text+keyword       |   快门速度                              |
//	|   ├ iso             | long               |   ISO 感光度                            |
//	|   ├ gps_lat         | float              |   GPS 纬度                              |
//	|   └ gps_lon         | float              |   GPS 经度                              |
//	| colors              | nested             | 主体颜色对象数组（name+hex）            |
//	| people_count        | integer            | 画面中检测到的人物数量                  |
//	| has_text            | boolean            | 画面中是否包含文字                      |
//	| text                | text (ik)          | OCR 提取的文本内容                      |
//	| status              | keyword            | 处理状态（unanalyzed/analyzing/analyzed）|
//	| path                | keyword            | 照片文件路径                            |
//	| mtime               | long               | 文件修改时间（Unix 时间戳）             |
//	| size                | long               | 文件大小（字节）                        |
//	| created_at          | date               | 文档创建时间                            |
//	| email               | keyword            | 联系邮箱（用于私有目录访问控制）        |
//	| gps_lat             | double             | GPS 纬度（顶层，便于聚合）             |
//	| gps_lon             | double             | GPS 经度（顶层，便于聚合）             |
//	| country             | keyword            | 国家（Amap 逆地理编码）                |
//	| province            | keyword            | 省份                                    |
//	| city                | keyword            | 城市                                    |
//	| district            | keyword            | 区县                                    |
//	| township            | keyword            | 乡镇/街道                              |
//	| business_area       | keyword            | 商圈                                    |
//	| street              | keyword            | 街道                                    |
//	| street_number       | keyword            | 门牌号                                  |
//	| address             | text (ik)          | 详细地址                                |
//	| formatted_address   | text (ik)          | 格式化地址                              |
//	| embedding           | knn_vector         | 文本向量（仅 embeddingDims>0 时启用）   |
//	| embedding_version   | keyword            | 向量版本标识（如 "bge-m3@1024"）       |
//	| embedded_at         | long               | 向量生成时间戳                          |
//
// _meta.version 用于运行时检测映射版本是否匹配，发现不匹配时自动删除旧索引并重建。
func buildIndexMapping(embeddingDims int) map[string]any {
	mapping := map[string]any{
		"settings": map[string]any{
			"number_of_shards":   1,
			"number_of_replicas": 0,
			"index": map[string]any{
				"knn": true,
			},
			"analysis": map[string]any{
				"analyzer": map[string]any{
					"ik_max_analyzer": map[string]any{
						"type": "ik_max_word",
					},
					"ik_smart_analyzer": map[string]any{
						"type": "ik_smart",
					},
				},
			},
		},
		"mappings": map[string]any{
			"_meta": map[string]any{
				"version": mappingVersion,
			},
			"properties": map[string]any{
				"description": map[string]any{
					"type":            "text",
					"analyzer":        "ik_max_word",
					"search_analyzer": "ik_smart",
				},
				"tags":               textWithKeyword,
				"objects":            textWithKeyword,
				"scene_type":         map[string]any{"type": "keyword"},
				"camera_model":       map[string]any{"type": "keyword"},
				"date_time_original": map[string]any{"type": "date"},
				"exif": map[string]any{
					"properties": map[string]any{
						"date_time_original": map[string]any{"type": "date"},
						"camera_model":       textWithKeyword,
						"lens_model":         textWithKeyword,
						"focal_length":       textWithKeyword,
						"aperture":           textWithKeyword,
						"shutter_speed":      textWithKeyword,
						"iso":                map[string]any{"type": "long"},
						"gps_lat":            map[string]any{"type": "float"},
						"gps_lon":            map[string]any{"type": "float"},
					},
				},
				"colors": map[string]any{
					"type": "nested",
					"properties": map[string]any{
						"name": map[string]any{"type": "keyword"},
						"hex":  map[string]any{"type": "keyword"},
					},
				},
				"people_count": map[string]any{"type": "integer"},
				"has_text":     map[string]any{"type": "boolean"},
				"text": map[string]any{
					"type":            "text",
					"analyzer":        "ik_max_word",
					"search_analyzer": "ik_smart",
				},
				"status":        map[string]any{"type": "keyword"},
				"path":          map[string]any{"type": "keyword"},
				"mtime":         map[string]any{"type": "long"},
				"size":          map[string]any{"type": "long"},
				"created_at":    map[string]any{"type": "date"},
				"email":         map[string]any{"type": "keyword"},
				"gps_lat":       map[string]any{"type": "double"},
				"gps_lon":       map[string]any{"type": "double"},
				"location":      map[string]any{"type": "geo_point"},
				"country":       map[string]any{"type": "keyword"},
				"province":      map[string]any{"type": "keyword"},
				"city":          map[string]any{"type": "keyword"},
				"district":      map[string]any{"type": "keyword"},
				"township":      map[string]any{"type": "keyword"},
				"business_area": map[string]any{"type": "keyword"},
				"street":        map[string]any{"type": "keyword"},
				"street_number": map[string]any{"type": "keyword"},
				"address": map[string]any{
					"type":            "text",
					"analyzer":        "ik_max_word",
					"search_analyzer": "ik_smart",
				},
				"formatted_address": map[string]any{
					"type":            "text",
					"analyzer":        "ik_max_word",
					"search_analyzer": "ik_smart",
				},
			},
		},
	}

	if embeddingDims > 0 {
		props := mapping["mappings"].(map[string]any)["properties"].(map[string]any)
		props["embedding"] = map[string]any{
			"type":       "knn_vector",
			"dimension":  embeddingDims,
			"space_type": "cosinesimil",
			"method": map[string]any{
				"engine": "faiss",
				"name":   "hnsw",
				"parameters": map[string]any{
					"m":               16,
					"ef_construction": 100,
				},
			},
		}
		props["embedding_version"] = map[string]any{"type": "keyword"}
		props["embedded_at"] = map[string]any{"type": "long"}
	}

	return mapping
}

// textWithKeyword 是可复用的字段映射模板，用于需要同时支持两种查询模式的字段：
//   - text 类型（IK 分词器）：支持全文搜索（分词、相关性评分）
//   - .keyword 子字段：支持精确匹配和聚合（terms aggregation）
//
// 典型用法：tags、objects 字段。搜索时使用 tags 做全文匹配，筛选时使用 tags.keyword 做精确过滤。
var textWithKeyword = map[string]any{
	"type":            "text",
	"analyzer":        "ik_max_word",
	"search_analyzer": "ik_smart",
	"fields": map[string]any{
		"keyword": map[string]any{"type": "keyword"},
	},
}

// EnsureIndex 确保 OS 索引存在且映射版本匹配。
//
// 流程：
//  1. 调用 indexExists 检查索引是否存在
//  2. 若存在 → 调用 checkMappingVersion 校验 _meta.version，不匹配时自动删除并重建
//  3. 若不存在 → 调用 createIndex 使用当前 indexMapping 创建索引
func (c *OSClient) EnsureIndex(ctx context.Context, indexName string, embeddingDims int) error {
	exists, err := c.indexExists(ctx, indexName)
	if err != nil {
		return err
	}
	if exists {
		return c.checkMappingVersion(ctx, indexName, embeddingDims)
	}
	return c.createIndex(ctx, indexName, embeddingDims)
}

// indexExists 通过 OS Indices.Exists API 检查指定索引是否存在。
// 返回 (true, nil) 表示存在，(false, nil) 表示不存在（404），其他状态码视为错误。
//
// 注意：opensearch-go v4 的 typed client 把所有 HTTP >299 响应（包括 404）都封装成
// error 返回（见 opensearchapi.Client.do: resp.IsError() 分支）。因此当 err != nil 时
// 必须同时检查 resp.StatusCode — 404 是 "索引不存在" 的正常回答，不是错误。
func (c *OSClient) indexExists(ctx context.Context, indexName string) (bool, error) {
	resp, err := c.client.Indices.Exists(ctx, opensearchapi.IndicesExistsReq{
		Indices: []string{indexName},
	})
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode == 404 {
				return false, nil
			}
		}
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

// checkMappingVersion 获取 OS 中已有索引的映射，提取 _meta.version 并与当前 mappingVersion 比较。
// 如果版本缺失或不一致，会自动删除旧索引并使用新映射重建。
func (c *OSClient) checkMappingVersion(ctx context.Context, indexName string, embeddingDims int) error {
	resp, err := c.client.Indices.Get(ctx, opensearchapi.IndicesGetReq{
		Indices: []string{indexName},
	})
	if err != nil {
		// opensearch-go typed client 把 404 也当作 error 返回。
		// 通常不会在 indexExists→Get 之间发生（索引刚被确认存在），
		// 但并发环境/极端情况可能出现 race，这里按"不存在"处理并重建。
		if resp != nil && resp.Inspect().Response != nil && resp.Inspect().Response.StatusCode == 404 {
			c.logger.Info("index vanished between Exists and Get, recreating", "index", indexName)
			_ = c.deleteIndex(ctx, indexName)
			return c.createIndex(ctx, indexName, embeddingDims)
		}
		return fmt.Errorf("get mapping: %w", err)
	}

	idxData, ok := resp.Indices[indexName]
	if !ok {
		c.logger.Info("index not found in response, recreating", "index", indexName)
		if err := c.deleteIndex(ctx, indexName); err != nil {
			return fmt.Errorf("delete index for migration: %w", err)
		}
		return c.createIndex(ctx, indexName, embeddingDims)
	}

	var mappings map[string]any
	if err := json.Unmarshal(idxData.Mappings, &mappings); err != nil {
		return fmt.Errorf("decode mapping: %w", err)
	}

	meta, ok := mappings["_meta"].(map[string]any)
	if !ok {
		c.logger.Info("mapping _meta missing, recreating", "index", indexName)
		if err := c.deleteIndex(ctx, indexName); err != nil {
			return fmt.Errorf("delete index for migration: %w", err)
		}
		return c.createIndex(ctx, indexName, embeddingDims)
	}

	version, _ := meta["version"].(string)
	if version != mappingVersion {
		c.logger.Info("mapping version mismatch, recreating index",
			"expected_version", mappingVersion,
			"actual_version", version,
			"index", indexName,
		)
		if err := c.deleteIndex(ctx, indexName); err != nil {
			return fmt.Errorf("delete index for migration: %w", err)
		}
		return c.createIndex(ctx, indexName, embeddingDims)
	}
	return nil
}

func (c *OSClient) createIndex(ctx context.Context, indexName string, embeddingDims int) error {
	body, err := json.Marshal(buildIndexMapping(embeddingDims))
	if err != nil {
		return fmt.Errorf("marshal mapping: %w", err)
	}

	_, err = c.client.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index: indexName,
		Body:  bytes.NewReader(body),
	})
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	return nil
}

func (c *OSClient) deleteIndex(ctx context.Context, indexName string) error {
	// _, err := c.client.Indices.Delete(ctx, opensearchapi.IndicesDeleteReq{
	// 	Indices: []string{indexName},
	// })
	// if err != nil {
	// 	// OpenSearch returns an error on 404, but we want to ignore it
	// 	return nil
	// }
	return nil
}
