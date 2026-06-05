// Package geocoder 提供逆地理编码能力（高德 + Google Maps 回退）。
// 将 GPS 坐标转换为结构化地址信息（省、市、区、街道等），
// 默认使用高德 API，海外坐标自动回退到 Google Maps。
package geocoder

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/zwh8800/phosche/internal/types"
)

// Geocoder 定义逆地理编码接口。
// 实现该接口的类型可以将 GPS 坐标转换为结构化地址信息。
type Geocoder interface {
	ReverseGeocode(ctx context.Context, lat, lon float64) (*types.GeoInfo, error)
}

// AmapGeocoder 封装高德逆地理编码 API 客户端。
// 通过高德 Web 服务 API 将经纬度坐标转换为结构化地址。
type AmapGeocoder struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewAmapGeocoder 创建一个新的 AmapGeocoder 实例。
// apiKey 为高德开放平台的 Web 服务 API Key，
// 用于身份验证和请求限流。
func NewAmapGeocoder(apiKey string) *AmapGeocoder {
	return &AmapGeocoder{
		apiKey: apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:   "https://restapi.amap.com/v3/geocode/regeo",
	}
}

// ReverseGeocode 将经纬度坐标转换为结构化地址信息。
// 返回的 GeoInfo 包含省、市、区、街道等地址层级信息。
// 如果 API 调用失败或坐标无效，返回错误。
func (g *AmapGeocoder) ReverseGeocode(ctx context.Context, lat, lon float64) (*types.GeoInfo, error) {
	if g.apiKey == "" {
		return nil, nil
	}

	url := fmt.Sprintf("%s?key=%s&location=%f,%f", g.baseURL, g.apiKey, lon, lat)

	slog.Debug("geocoder: reverse geocode request",
		"lat", lat,
		"lon", lon,
		"url", fmt.Sprintf("%s?key=***&location=%f,%f", g.baseURL, lon, lat),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// 高德 API 特性：字段有值时返回 string，无值时返回空数组 []。
	// 因此除 status/info 外，所有可能为空的字段统一用 any 接收，再通过类型断言提取。
	var result struct {
		Status    string `json:"status"`
		Info      string `json:"info"`
		Regeocode struct {
			FormattedAddress any `json:"formatted_address"`
			AddressComponent struct {
				Country  any `json:"country"`
				Province any `json:"province"`
				City     any `json:"city"`
				District any `json:"district"`
				Township any `json:"township"`
				StreetNumber struct {
					Street any         `json:"street"`
					Number interface{} `json:"number"`
				} `json:"streetNumber"`
				BusinessAreas any `json:"businessAreas"`
			} `json:"addressComponent"`
		} `json:"regeocode"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Status != "1" {
		return nil, fmt.Errorf("amap api error: %s", result.Info)
	}

	toString := func(v any) string {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}

	country := toString(result.Regeocode.AddressComponent.Country)
	province := toString(result.Regeocode.AddressComponent.Province)
	city := toString(result.Regeocode.AddressComponent.City)
	district := toString(result.Regeocode.AddressComponent.District)
	township := toString(result.Regeocode.AddressComponent.Township)
	formattedAddress := toString(result.Regeocode.FormattedAddress)
	street := toString(result.Regeocode.AddressComponent.StreetNumber.Street)
	streetNumber := ""
	if n, ok := result.Regeocode.AddressComponent.StreetNumber.Number.(string); ok {
		streetNumber = n
	}

	businessArea := ""
	if arr, ok := result.Regeocode.AddressComponent.BusinessAreas.([]interface{}); ok && len(arr) > 0 {
		if m, ok := arr[0].(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				businessArea = name
			}
		}
	}

	slog.Debug("geocoder: reverse geocode response",
		"formatted_address", formattedAddress,
		"country", country,
		"province", province,
		"city", city,
		"district", district,
		"township", township,
	)

	address := country + province + city + district + township + businessArea + street + streetNumber

	return &types.GeoInfo{
		Country:          country,
		Province:         province,
		City:             city,
		District:         district,
		Township:         township,
		BusinessArea:     businessArea,
		Street:           street,
		StreetNumber:     streetNumber,
		Address:          address,
		FormattedAddress: formattedAddress,
	}, nil
}
