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

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
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

func setupTestOS(t *testing.T) (*OSClient, func()) {
	t.Helper()

	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "opensearchproject/opensearch:2.19.5",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":            "single-node",
			"DISABLE_SECURITY_PLUGIN":   "true",
			"OPENSEARCH_JAVA_OPTS":      "-Xms512m -Xmx512m",
		},
		WaitingFor: wait.ForHTTP("/").WithPort("9200/tcp").WithStartupTimeout(90 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start OpenSearch container")

	cleanup := func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer termCancel()
		if err := container.Terminate(termCtx); err != nil {
			t.Logf("failed to terminate OpenSearch container: %v", err)
		}
	}

	mappedPort, err := container.MappedPort(ctx, "9200")
	require.NoError(t, err, "failed to get mapped port")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get container host")

	address := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	cfg := config.OSConfig{
		Addresses: []string{address},
	}

	osClient, err := NewOSClient(cfg)
	require.NoError(t, err, "failed to create OpenSearch client")

	return osClient, cleanup
}

func captureLogger(buf *bytes.Buffer) *slog.Logger {
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	return slog.New(handler)
}

func TestOSClient_Connect(t *testing.T) {
	client, cleanup := setupTestOS(t)
	defer cleanup()

	resp, err := client.client.Info(context.Background(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Version.Number, "Info() should return version")
}

func TestEnsureIndex_Create(t *testing.T) {
	client, cleanup := setupTestOS(t)
	defer cleanup()

	ctx := context.Background()
	indexName := "test_photos_create"

	err := client.EnsureIndex(ctx, indexName, 0, &config.Config{})
	require.NoError(t, err, "EnsureIndex should create the index")

	exists, err := client.indexExists(ctx, indexName)
	require.NoError(t, err)
	assert.True(t, exists, "index should exist after EnsureIndex")

	getResp, err := client.client.Indices.Get(ctx, opensearchapi.IndicesGetReq{
		Indices: []string{indexName},
	})
	require.NoError(t, err)

	idxData, ok := getResp.Indices[indexName]
	require.True(t, ok, "index should be in response")

	var mappings map[string]any
	require.NoError(t, json.Unmarshal(idxData.Mappings, &mappings))

	meta, ok := mappings["_meta"].(map[string]any)
	require.True(t, ok, "_meta should exist")

	version, ok := meta["version"].(float64)
	require.True(t, ok, "version should be a number")
	assert.Equal(t, float64(mappingVersion), version, "_meta.version should match")

	_, _ = client.client.Indices.Delete(ctx, opensearchapi.IndicesDeleteReq{
		Indices: []string{indexName},
	})
}

func TestEnsureIndex_Idempotent(t *testing.T) {
	client, cleanup := setupTestOS(t)
	defer cleanup()

	ctx := context.Background()
	indexName := "test_photos_idempotent"

	err := client.EnsureIndex(ctx, indexName, 0, &config.Config{})
	require.NoError(t, err)

	err = client.EnsureIndex(ctx, indexName, 0, &config.Config{})
	require.NoError(t, err)

	_, _ = client.client.Indices.Delete(ctx, opensearchapi.IndicesDeleteReq{
		Indices: []string{indexName},
	})
}

func TestEnsureIndex_VersionMismatch(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "opensearchproject/opensearch:2.19.5",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":            "single-node",
			"DISABLE_SECURITY_PLUGIN":   "true",
			"OPENSEARCH_JAVA_OPTS":      "-Xms512m -Xmx512m",
		},
		WaitingFor: wait.ForHTTP("/").WithPort("9200/tcp").WithStartupTimeout(90 * time.Second),
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

	// Create an index with a version lower than current code's mappingVersion
	// Using the full current mapping but with _meta.version = 8 (one version behind)
	currentMapping := buildIndexMapping(0)
	mappingCopy := map[string]any{
		"mappings": map[string]any{
			"_meta": map[string]any{
				"version": 8, // One version behind current mappingVersion (9)
			},
			"properties": currentMapping["mappings"].(map[string]any)["properties"],
		},
	}
	bodyBytes, err := json.Marshal(mappingCopy)
	require.NoError(t, err)

	rawOS, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: []string{address},
		},
	})
	require.NoError(t, err)

	_, err = rawOS.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index: indexName,
		Body:  bytes.NewReader(bodyBytes),
	})
	require.NoError(t, err, "should create old version index successfully")

	var buf bytes.Buffer
	logger := captureLogger(&buf)

	osCfg := config.OSConfig{Addresses: []string{address}}
	testClient, err := NewOSClientWithLogger(osCfg, logger)
	require.NoError(t, err)

	err = testClient.EnsureIndex(ctx, indexName, 0, &config.Config{})
	require.NoError(t, err, "EnsureIndex should succeed with incremental migration")

	// Verify _meta.version was updated to current mappingVersion
	getResp, err := testClient.client.Indices.Get(ctx, opensearchapi.IndicesGetReq{
		Indices: []string{indexName},
	})
	require.NoError(t, err)

	idxData, ok := getResp.Indices[indexName]
	require.True(t, ok, "index should exist")

	var mappings map[string]any
	require.NoError(t, json.Unmarshal(idxData.Mappings, &mappings))

	meta, ok := mappings["_meta"].(map[string]any)
	require.True(t, ok, "_meta should exist")

	version, ok := meta["version"].(float64)
	require.True(t, ok, "version should be a number")
	assert.Equal(t, float64(MappingVersion()), version, "_meta.version should be updated to current version")

	logOutput := buf.String()
	t.Logf("log output: %s", logOutput)
	assert.Contains(t, logOutput, "incremental migration",
		"log should contain migration message")

	_, _ = rawOS.Indices.Delete(ctx, opensearchapi.IndicesDeleteReq{
		Indices: []string{indexName},
	})
}

func TestOSClient_InvalidAddress(t *testing.T) {
	cfg := config.OSConfig{
		Addresses: []string{"http://127.0.0.1:1"},
	}
	_, err := NewOSClient(cfg)
	assert.Error(t, err, "should return error for unreachable address")
	assert.False(t, strings.Contains(err.Error(), "panic"), "should not panic")
}

func TestOSClient_Connect_WithTLS(t *testing.T) {
	cfg := config.OSConfig{
		Addresses:          []string{"http://127.0.0.1:1"},
		InsecureSkipVerify: true,
	}
	_, err := NewOSClient(cfg)
	assert.Error(t, err, "should set TLS config without panicking")
}
