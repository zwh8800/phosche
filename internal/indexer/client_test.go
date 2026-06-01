package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zwh8800/phosche/internal/config"
)

func dockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func setupTestES(t *testing.T) (*ESClient, func()) {
	t.Helper()

	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "docker.elastic.co/elasticsearch/elasticsearch:8.17.0",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":         "single-node",
			"xpack.security.enabled": "false",
			"ES_JAVA_OPTS":           "-Xms512m -Xmx512m",
		},
		WaitingFor: wait.ForHTTP("/").WithPort("9200/tcp").WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start ES container")

	cleanup := func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer termCancel()
		if err := container.Terminate(termCtx); err != nil {
			t.Logf("failed to terminate ES container: %v", err)
		}
	}

	mappedPort, err := container.MappedPort(ctx, "9200")
	require.NoError(t, err, "failed to get mapped port")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get container host")

	address := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	cfg := config.ESConfig{
		Addresses: []string{address},
	}

	esClient, err := NewESClient(cfg)
	require.NoError(t, err, "failed to create ES client")

	return esClient, cleanup
}

func captureLogger(buf *bytes.Buffer) *slog.Logger {
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	return slog.New(handler)
}

func TestESClient_Connect(t *testing.T) {
	client, cleanup := setupTestES(t)
	defer cleanup()

	resp, err := client.client.Info()
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.False(t, resp.IsError(), "Info() should return success")
}

func TestEnsureIndex_Create(t *testing.T) {
	client, cleanup := setupTestES(t)
	defer cleanup()

	ctx := context.Background()
	indexName := "test_photos_create"

	err := client.EnsureIndex(ctx, indexName, 0)
	require.NoError(t, err, "EnsureIndex should create the index")

	exists, err := client.indexExists(ctx, indexName)
	require.NoError(t, err)
	assert.True(t, exists, "index should exist after EnsureIndex")

	req := esapi.IndicesGetMappingRequest{
		Index: []string{indexName},
	}
	resp, err := req.Do(ctx, client.client)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.False(t, resp.IsError())

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	version := extractMetaVersion(result, indexName)
	assert.Equal(t, mappingVersion, version, "_meta.version should match")

	delReq := esapi.IndicesDeleteRequest{Index: []string{indexName}}
	delResp, err := delReq.Do(ctx, client.client)
	require.NoError(t, err)
	defer delResp.Body.Close()
}

func TestEnsureIndex_Idempotent(t *testing.T) {
	client, cleanup := setupTestES(t)
	defer cleanup()

	ctx := context.Background()
	indexName := "test_photos_idempotent"

	err := client.EnsureIndex(ctx, indexName, 0)
	require.NoError(t, err)

	err = client.EnsureIndex(ctx, indexName, 0)
	require.NoError(t, err)

	delReq := esapi.IndicesDeleteRequest{Index: []string{indexName}}
	delResp, _ := delReq.Do(ctx, client.client)
	if delResp != nil {
		delResp.Body.Close()
	}
}

func TestEnsureIndex_VersionMismatch(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "docker.elastic.co/elasticsearch/elasticsearch:8.17.0",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":         "single-node",
			"xpack.security.enabled": "false",
			"ES_JAVA_OPTS":           "-Xms512m -Xmx512m",
		},
		WaitingFor: wait.ForHTTP("/").WithPort("9200/tcp").WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctxTimeout, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer termCancel()
		container.Terminate(termCtx)
	})

	mappedPort, err := container.MappedPort(ctxTimeout, "9200")
	require.NoError(t, err)

	host, err := container.Host(ctxTimeout)
	require.NoError(t, err)

	address := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	ctx := context.Background()
	indexName := "test_photos_version_mismatch"

	oldMapping := map[string]any{
		"mappings": map[string]any{
			"_meta": map[string]any{
				"version": "0",
			},
			"properties": map[string]any{
				"test_field": map[string]any{"type": "keyword"},
			},
		},
	}
	bodyBytes, err := json.Marshal(oldMapping)
	require.NoError(t, err)

	rawES, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{address},
	})
	require.NoError(t, err)

	createReq := esapi.IndicesCreateRequest{
		Index: indexName,
		Body:  bytes.NewReader(bodyBytes),
	}
	createResp, err := createReq.Do(ctx, rawES)
	require.NoError(t, err)
	createResp.Body.Close()
	require.False(t, createResp.IsError(), "should create old index successfully")

	var buf bytes.Buffer
	logger := captureLogger(&buf)

	esCfg := config.ESConfig{Addresses: []string{address}}
	testClient, err := NewESClientWithLogger(esCfg, logger)
	require.NoError(t, err)

	err = testClient.EnsureIndex(ctx, indexName, 0)
	require.NoError(t, err, "EnsureIndex should succeed (warning only)")

	logOutput := buf.String()
	t.Logf("log output: %s", logOutput)
	assert.Contains(t, logOutput, "index mapping version mismatch",
		"log should contain version mismatch warning")
	assert.Contains(t, logOutput, `expected_version=1`,
		"log should contain expected version")
	assert.Contains(t, logOutput, `actual_version=0`,
		"log should contain actual version")

	delReq := esapi.IndicesDeleteRequest{Index: []string{indexName}}
	delResp, _ := delReq.Do(ctx, rawES)
	if delResp != nil {
		delResp.Body.Close()
	}
}

func TestESClient_InvalidAddress(t *testing.T) {
	cfg := config.ESConfig{
		Addresses: []string{"http://127.0.0.1:1"},
	}
	_, err := NewESClient(cfg)
	assert.Error(t, err, "should return error for unreachable address")
	assert.False(t, strings.Contains(err.Error(), "panic"), "should not panic")
}

func TestESClient_Connect_WithTLS(t *testing.T) {
	cfg := config.ESConfig{
		Addresses:          []string{"http://127.0.0.1:1"},
		InsecureSkipVerify: true,
	}
	_, err := NewESClient(cfg)
	assert.Error(t, err, "should set TLS config without panicking")
}
