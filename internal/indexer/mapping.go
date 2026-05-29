package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

const mappingVersion = "2"

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
			"colors":             map[string]any{"type": "keyword"},
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
		},
	},
}

var textWithKeyword = map[string]any{
	"type": "text",
	"fields": map[string]any{
		"keyword": map[string]any{"type": "keyword"},
	},
}

// EnsureIndex checks if the index exists. If not, it creates it with the
// current mapping. If it exists, it checks the _meta.version field and
// warns on mismatch (it does NOT auto-migrate).
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
