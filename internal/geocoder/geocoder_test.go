package geocoder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReverseGeocode_EmptyAPIKey(t *testing.T) {
	g := NewAmapGeocoder("")
	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	assert.NoError(t, err)
	assert.Nil(t, info)
}

func TestReverseGeocode_Success_ProvinceLevelCity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "test-key", r.URL.Query().Get("key"))
		assert.Equal(t, "116.407400,39.904200", r.URL.Query().Get("location"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "1",
			"info": "OK",
			"regeocode": {
				"formatted_address": "\u5317\u4eac\u5e02\u671d\u9633\u533a\u961c\u901a\u4e1c\u5927\u88576\u53f7",
				"addressComponent": {
					"country": "\u4e2d\u56fd",
					"province": "\u5317\u4eac\u5e02",
					"city": [],
					"district": "\u671d\u9633\u533a"
				}
			}
		}`))
	}))
	defer server.Close()

	g := &AmapGeocoder{
		apiKey:     "test-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "中国", info.Country)
	assert.Equal(t, "北京市", info.Province)
	assert.Equal(t, "", info.City)
	assert.Equal(t, "朝阳区", info.District)
	assert.Equal(t, "北京市朝阳区阜通东大街6号", info.FormattedAddress)
}

func TestReverseGeocode_Success_NormalCity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "1",
			"info": "OK",
			"regeocode": {
				"formatted_address": "\u5e7f\u4e1c\u7701\u6df1\u5733\u5e02\u5357\u5c71\u533a\u79d1\u6280\u56ed\u8def1\u53f7",
				"addressComponent": {
					"country": "\u4e2d\u56fd",
					"province": "\u5e7f\u4e1c\u7701",
					"city": "\u6df1\u5733\u5e02",
					"district": "\u5357\u5c71\u533a"
				}
			}
		}`))
	}))
	defer server.Close()

	g := &AmapGeocoder{
		apiKey:     "test-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 22.5431, 114.0579)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "中国", info.Country)
	assert.Equal(t, "广东省", info.Province)
	assert.Equal(t, "深圳市", info.City)
	assert.Equal(t, "南山区", info.District)
	assert.Equal(t, "广东省深圳市南山区科技园路1号", info.FormattedAddress)
}

func TestReverseGeocode_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "0",
			"info": "INVALID_USER_KEY"
		}`))
	}))
	defer server.Close()

	g := &AmapGeocoder{
		apiKey:     "bad-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "amap api error")
	assert.Contains(t, err.Error(), "INVALID_USER_KEY")
	assert.Nil(t, info)
}

func TestReverseGeocode_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	g := &AmapGeocoder{
		apiKey:     "test-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status: 500")
	assert.Nil(t, info)
}

func TestReverseGeocode_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	g := &AmapGeocoder{
		apiKey:     "test-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
	assert.Nil(t, info)
}
