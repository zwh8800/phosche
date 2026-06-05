package geocoder

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zwh8800/phosche/internal/types"
)

// mockGeocoder 用于测试的模拟逆地理编码器。
type mockGeocoder struct {
	geo   *types.GeoInfo
	err   error
	calls int // 记录调用次数
}

func (m *mockGeocoder) ReverseGeocode(ctx context.Context, lat, lon float64) (*types.GeoInfo, error) {
	m.calls++
	return m.geo, m.err
}

func makeValidGeo() *types.GeoInfo {
	return &types.GeoInfo{
		Country:          "中国",
		Province:         "北京市",
		City:             "北京市",
		District:         "朝阳区",
		FormattedAddress: "中国北京市朝阳区",
	}
}

func makeEmptyGeo() *types.GeoInfo {
	return &types.GeoInfo{
		FormattedAddress: "",
	}
}

// Test 1: 主编码器返回有效数据时，直接使用主编码器结果，不触发回退。
func TestFallback_PrimaryReturnsValidData(t *testing.T) {
	primary := &mockGeocoder{geo: makeValidGeo()}
	fallback := &mockGeocoder{geo: makeValidGeo()}

	fb := NewFallbackGeocoder(primary, fallback)
	info, err := fb.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "中国北京市朝阳区", info.FormattedAddress)
	assert.Equal(t, 1, primary.calls, "primary should be called exactly once")
	assert.Equal(t, 0, fallback.calls, "fallback should never be called")
}

// Test 2: 主编码器返回 (nil, nil) 时，调用回退编码器。
func TestFallback_PrimaryReturnsNil(t *testing.T) {
	primary := &mockGeocoder{geo: nil, err: nil}
	fallback := &mockGeocoder{geo: makeValidGeo()}

	fb := NewFallbackGeocoder(primary, fallback)
	info, err := fb.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "中国北京市朝阳区", info.FormattedAddress)
	assert.Equal(t, 1, primary.calls)
	assert.Equal(t, 1, fallback.calls, "fallback should be called once")
}

// Test 3: 主编码器返回空 FormattedAddress 时，调用回退编码器。
func TestFallback_PrimaryReturnsEmptyFormattedAddress(t *testing.T) {
	primary := &mockGeocoder{geo: makeEmptyGeo()}
	fallback := &mockGeocoder{geo: makeValidGeo()}

	fb := NewFallbackGeocoder(primary, fallback)
	info, err := fb.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "中国北京市朝阳区", info.FormattedAddress)
	assert.Equal(t, 1, primary.calls)
	assert.Equal(t, 1, fallback.calls, "fallback should be called when primary result is empty")
}

// Test 4: 主编码器返回 error 时，调用回退编码器。
func TestFallback_PrimaryError(t *testing.T) {
	primary := &mockGeocoder{err: errors.New("primary down")}
	fallback := &mockGeocoder{geo: makeValidGeo()}

	fb := NewFallbackGeocoder(primary, fallback)
	info, err := fb.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "中国北京市朝阳区", info.FormattedAddress)
	assert.Equal(t, 1, primary.calls)
	assert.Equal(t, 1, fallback.calls)
}

// Test 5: 主编码器和回退编码器都返回 (nil, nil)，应返回 (nil, nil) 且无 error。
func TestFallback_BothReturnNil(t *testing.T) {
	primary := &mockGeocoder{geo: nil, err: nil}
	fallback := &mockGeocoder{geo: nil, err: nil}

	fb := NewFallbackGeocoder(primary, fallback)
	info, err := fb.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	assert.NoError(t, err)
	assert.Nil(t, info)
	assert.Equal(t, 1, primary.calls)
	assert.Equal(t, 1, fallback.calls)
}

// Test 6: 主编码器和回退编码器都返回 error，应返回回退的 error。
func TestFallback_BothError(t *testing.T) {
	primary := &mockGeocoder{err: errors.New("primary error")}
	fallbackErr := errors.New("fallback error")
	fallback := &mockGeocoder{err: fallbackErr}

	fb := NewFallbackGeocoder(primary, fallback)
	info, err := fb.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	assert.Error(t, err)
	assert.Equal(t, fallbackErr, err, "should return fallback error")
	assert.Nil(t, info)
	assert.Equal(t, 1, primary.calls)
	assert.Equal(t, 1, fallback.calls)
}

// Test 7: 主编码器返回空 FormattedAddress，回退编码器也返回空，应返回主编码器结果（最佳努力，无死循环）。
func TestFallback_PrimaryEmptyFallbackEmpty(t *testing.T) {
	primary := &mockGeocoder{geo: makeEmptyGeo()}
	fallback := &mockGeocoder{geo: makeEmptyGeo()}

	fb := NewFallbackGeocoder(primary, fallback)
	info, err := fb.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "", info.FormattedAddress)
	assert.Equal(t, 1, primary.calls)
	assert.Equal(t, 1, fallback.calls)
}

// Test 8: NewFallbackGeocoder(nil, nil) — nil 接口处理，应优雅返回 (nil, nil)。
func TestFallback_NilConstructor(t *testing.T) {
	fb := NewFallbackGeocoder(nil, nil)

	info, err := fb.ReverseGeocode(context.Background(), 39.9042, 116.4074)

	assert.NoError(t, err)
	assert.Nil(t, info)
}
