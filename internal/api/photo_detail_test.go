package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apperrors "github.com/zwh8800/phosche/internal/errors"
	"github.com/zwh8800/phosche/internal/types"
)

type mockIndexer struct {
	getPhotoFunc     func(ctx context.Context, path string, indexName string) (*types.PhotoDocument, error)
	getPhotoByIDFunc func(ctx context.Context, id string, indexName string) (*types.PhotoDocument, error)
}

func (m *mockIndexer) GetPhoto(ctx context.Context, path string, indexName string) (*types.PhotoDocument, error) {
	return m.getPhotoFunc(ctx, path, indexName)
}

func (m *mockIndexer) GetPhotoByID(ctx context.Context, id string, indexName string) (*types.PhotoDocument, error) {
	if m.getPhotoByIDFunc != nil {
		return m.getPhotoByIDFunc(ctx, id, indexName)
	}
	return nil, nil
}

func (m *mockIndexer) DeletePhoto(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockIndexer) UpdateEXIF(_ context.Context, _ string, _ *types.EXIFInfo, _ string) error {
	return nil
}

func (m *mockIndexer) UpdateGeo(_ context.Context, _ string, _ *types.GeoInfo, _ string) error {
	return nil
}

func (m *mockIndexer) ScrollAll(_ context.Context, _ string, _ func(*types.PhotoDocument) error) error {
	return nil
}

func TestGetPhotoDetail_Success(t *testing.T) {
	expectedDoc := &types.PhotoDocument{
		Photo: types.Photo{
			ID:     "abc123",
			Path:   "2024/01/IMG_001.jpg",
			MTime:  1700000000,
			Size:   204800,
			Status: types.StatusAnalyzed,
			EXIF: &types.EXIFInfo{
				CameraModel:      "Canon EOS R5",
				DateTimeOriginal: "2024:01:15 10:30:00",
				ISO:              400,
				GPSLat:           35.6762,
				GPSLon:           139.6503,
			},
			CreatedAt: 1700000000,
		},
		AnalysisResult: types.AnalysisResult{
			Description: "A beautiful sunset over Tokyo",
			Tags:        []string{"sunset", "tokyo", "skyline"},
			Objects:     []string{"sun", "building", "cloud"},
			SceneType:   "cityscape",
			Colors:      []types.ColorInfo{{Name: "橙色", Hex: "#F97316"}, {Name: "蓝色", Hex: "#3B82F6"}, {Name: "紫色", Hex: "#A855F7"}},
			PeopleCount: 0,
			HasText:     false,
			Confidence:  0.95,
		},
	}

	mock := &mockIndexer{
		getPhotoByIDFunc: func(_ context.Context, id string, indexName string) (*types.PhotoDocument, error) {
			assert.Equal(t, "abc123def456", id)
			assert.Equal(t, "photos-index", indexName)
			return expectedDoc, nil
		},
	}

	router := NewRouter(&Server{
		Indexer:   mock,
		IndexName: "photos-index",
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos/abc123def456")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "abc123", body["id"])
	assert.Equal(t, "2024/01/IMG_001.jpg", body["path"])
	assert.Equal(t, "/photos/2024/01/IMG_001.jpg", body["photo_url"])
	assert.Equal(t, "analyzed", body["status"])

	exif, ok := body["exif"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Canon EOS R5", exif["camera_model"])
	assert.Equal(t, float64(400), exif["iso"])
	assert.Equal(t, float64(400), exif["iso"])

	assert.Equal(t, "A beautiful sunset over Tokyo", body["description"])
	assert.Equal(t, "cityscape", body["scene_type"])
	assert.Equal(t, float64(0.95), body["confidence"])

	tags, ok := body["tags"].([]any)
	require.True(t, ok)
	assert.Equal(t, "sunset", tags[0])
}

func TestGetPhotoDetail_NotFound(t *testing.T) {
	mock := &mockIndexer{
		getPhotoByIDFunc: func(_ context.Context, id string, indexName string) (*types.PhotoDocument, error) {
			return nil, apperrors.NewNotFoundError("photo not found: " + id)
		},
	}

	router := NewRouter(&Server{
		Indexer: mock,
		IndexName:    "photos-index",
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "NOT_FOUND", errObj["code"])
	assert.Equal(t, "Photo not found", errObj["message"])
}

func TestGetPhotoDetail_InternalError(t *testing.T) {
	mock := &mockIndexer{
		getPhotoByIDFunc: func(_ context.Context, id string, indexName string) (*types.PhotoDocument, error) {
			return nil, apperrors.NewInternalError(assert.AnError)
		},
	}

	router := NewRouter(&Server{
		Indexer: mock,
		IndexName:    "photos-index",
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/photos/error-photo")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
	assert.Equal(t, "Failed to get photo", errObj["message"])
}
