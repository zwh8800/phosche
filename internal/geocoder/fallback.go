package geocoder

import (
	"context"
	"log/slog"

	"github.com/zwh8800/phosche/internal/types"
)

// FallbackGeocoder 实现 Geocoder 接口，使用主编码器和回退编码器提供
// 高可用的逆地理编码。当主编码器返回空结果或失败时，自动回退到备用编码器。
type FallbackGeocoder struct {
	primary  Geocoder
	fallback Geocoder
}

// NewFallbackGeocoder 创建一个新的 FallbackGeocoder 实例。
// primary 和 fallback 可以为 nil，nil 接口会被安全地跳过。
func NewFallbackGeocoder(primary, fallback Geocoder) Geocoder {
	return &FallbackGeocoder{
		primary:  primary,
		fallback: fallback,
	}
}

// ReverseGeocode 将经纬度坐标转换为结构化地址信息。
// 优先使用主编码器，失败或返回空结果时回退到备用编码器。
func (f *FallbackGeocoder) ReverseGeocode(ctx context.Context, lat, lon float64) (*types.GeoInfo, error) {
	// 如果没有主编码器，尝试回退编码器
	if f.primary == nil {
		if f.fallback != nil {
			return f.fallback.ReverseGeocode(ctx, lat, lon)
		}
		return nil, nil
	}

	geoInfo, err := f.primary.ReverseGeocode(ctx, lat, lon)
	if err != nil {
		slog.Warn("fallback: primary geocoder failed, trying fallback", "error", err)
		if f.fallback != nil {
			return f.fallback.ReverseGeocode(ctx, lat, lon)
		}
		return nil, err
	}

	// 检查主编码器结果是否为空
	if shouldFallback(geoInfo) {
		slog.Debug("fallback: primary returned empty result, trying fallback")
		if f.fallback != nil {
			fbInfo, fbErr := f.fallback.ReverseGeocode(ctx, lat, lon)
			if fbErr != nil {
				slog.Warn("fallback: fallback geocoder also failed", "error", fbErr)
				return geoInfo, nil // 返回主编码器结果，即使为空
			}
			if fbInfo != nil && fbInfo.FormattedAddress != "" {
				return fbInfo, nil
			}
			slog.Debug("fallback: fallback also returned empty")
		}
		return geoInfo, nil
	}

	return geoInfo, nil
}

// shouldFallback 判断逆地理编码结果是否需要回退。
// 当结果为空指针或 FormattedAddress 为空时，认为需要回退。
func shouldFallback(geo *types.GeoInfo) bool {
	return geo == nil || geo.FormattedAddress == ""
}
