package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthEndpoint(t *testing.T) {
	router := NewRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "0.1.0", body["version"])
}

func TestHealthEndpoint_ContentType(t *testing.T) {
	router := NewRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	assert.Contains(t, strings.ToLower(ct), "application/json")
}

func TestCORSHeaders(t *testing.T) {
	router := NewRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/health", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://example.com")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORSPreflight(t *testing.T) {
	router := NewRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/api/something", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestTimeout(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Timeout(1 * time.Second))
	r.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(31 * time.Second)
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/slow")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
}

func TestNotFound(t *testing.T) {
	router := NewRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestMethodNotAllowed(t *testing.T) {
	router := NewRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/health", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}
