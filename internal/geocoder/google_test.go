package geocoder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoogleGeocode_EmptyAPIKey(t *testing.T) {
	g := NewGoogleGeocoder("")
	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	assert.NoError(t, err)
	assert.Nil(t, info)
}

func TestGoogleGeocode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "test-key", r.URL.Query().Get("key"))
		assert.Equal(t, "/39.904200,116.407400", r.URL.Path)
		assert.Equal(t, "zh-CN", r.URL.Query().Get("languageCode"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"results": [{
				"formattedAddress": "\u4e2d\u56fd\u5e7f\u4e1c\u7701\u6df1\u5733\u5e02\u5357\u5c71\u533a\u7ca4\u6d77\u8857\u9053\u79d1\u6280\u56ed\u8def1\u53f7",
				"addressComponents": [
					{"longText": "1\u53f7", "shortText": "1\u53f7", "types": ["street_number"]},
					{"longText": "\u79d1\u6280\u56ed\u8def", "shortText": "\u79d1\u6280\u56ed\u8def", "types": ["route"]},
					{"longText": "\u7ca4\u6d77\u8857\u9053", "shortText": "\u7ca4\u6d77\u8857\u9053", "types": ["sublocality_level_2", "political"]},
					{"longText": "\u5357\u5c71\u533a", "shortText": "\u5357\u5c71\u533a", "types": ["sublocality", "sublocality_level_1"]},
					{"longText": "\u6df1\u5733\u5e02", "shortText": "\u6df1\u5733\u5e02", "types": ["locality", "political"]},
					{"longText": "\u5e7f\u4e1c\u7701", "shortText": "\u5e7f\u4e1c\u7701", "types": ["administrative_area_level_1", "political"]},
					{"longText": "\u4e2d\u56fd", "shortText": "CN", "types": ["country", "political"]}
				]
			}]
		}`))
	}))
	defer server.Close()

	g := &GoogleGeocoder{
		apiKey:     "test-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "中国", info.Country)
	assert.Equal(t, "广东省", info.Province)
	assert.Equal(t, "深圳市", info.City)
	assert.Equal(t, "南山区", info.District)
	assert.Equal(t, "粤海街道", info.Township)
	assert.Equal(t, "科技园路", info.Street)
	assert.Equal(t, "1号", info.StreetNumber)
	assert.Equal(t, "", info.BusinessArea)
	assert.Equal(t, "中国广东省深圳市南山区粤海街道科技园路1号", info.FormattedAddress)
	assert.Equal(t, "中国广东省深圳市南山区粤海街道科技园路1号", info.Address)
}

func TestGoogleGeocode_ZeroResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"results": []}`))
	}))
	defer server.Close()

	g := &GoogleGeocoder{
		apiKey:     "test-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 0, 0)
	assert.NoError(t, err)
	assert.Nil(t, info)
}

func TestGoogleGeocode_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{
			"error": {
				"code": 403,
				"message": "API key not valid. Please pass a valid API key.",
				"status": "PERMISSION_DENIED"
			}
		}`))
	}))
	defer server.Close()

	g := &GoogleGeocoder{
		apiKey:     "bad-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "google api error")
	assert.Contains(t, err.Error(), "API key not valid")
	assert.Nil(t, info)
}

func TestGoogleGeocode_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	g := &GoogleGeocoder{
		apiKey:     "test-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "google api error")
	assert.Contains(t, err.Error(), "HTTP 500")
	assert.Nil(t, info)
}

func TestGoogleGeocode_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	g := &GoogleGeocoder{
		apiKey:     "test-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
	assert.Nil(t, info)
}

func TestGoogleGeocode_OverQueryLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{
			"error": {
				"code": 429,
				"message": "You have exceeded your daily request quota.",
				"status": "RESOURCE_EXHAUSTED"
			}
		}`))
	}))
	defer server.Close()

	g := &GoogleGeocoder{
		apiKey:     "test-key",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	info, err := g.ReverseGeocode(context.Background(), 39.9042, 116.4074)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "google api error")
	assert.Contains(t, err.Error(), "exceeded your daily")
	assert.Nil(t, info)
}
