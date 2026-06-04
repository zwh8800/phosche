// Package geocoder 提供基于高德 API 的逆地理编码能力。
// 将 GPS 坐标转换为结构化地址信息（省、市、区、街道等）。
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

// Geocoder 封装高德逆地理编码 API 客户端。
// 通过高德 Web 服务 API 将经纬度坐标转换为结构化地址。
type Geocoder struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewGeocoder 创建一个新的 Geocoder 实例。
// apiKey 为高德开放平台的 Web 服务 API Key，
// 用于身份验证和请求限流。
func NewGeocoder(apiKey string) *Geocoder {
	return &Geocoder{
		apiKey: apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:   "https://restapi.amap.com/v3/geocode/regeo",
	}
}

// ReverseGeocode 将经纬度坐标转换为结构化地址信息。
// 返回的 GeoInfo 包含省、市、区、街道等地址层级信息。
// 如果 API 调用失败或坐标无效，返回错误。
func (g *Geocoder) ReverseGeocode(ctx context.Context, lat, lon float64) (*types.GeoInfo, error) {
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

	var result struct {
		Status    string `json:"status"`
		Info      string `json:"info"`
		Regeocode struct {
			FormattedAddress string `json:"formatted_address"`
			AddressComponent struct {
				Country  string `json:"country"`
				Province string `json:"province"`
				City     any    `json:"city"`
				District string `json:"district"`
				Township string `json:"township"`
				StreetNumber struct {
					Street string      `json:"street"`
					Number interface{} `json:"number"`
				} `json:"streetNumber"`
				BusinessAreas []struct {
					Name string `json:"name"`
				} `json:"businessAreas"`
			} `json:"addressComponent"`
		} `json:"regeocode"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Status != "1" {
		return nil, fmt.Errorf("amap api error: %s", result.Info)
	}

	city := ""
	if c, ok := result.Regeocode.AddressComponent.City.(string); ok {
		city = c
	}

	slog.Debug("geocoder: reverse geocode response",
		"formatted_address", result.Regeocode.FormattedAddress,
		"country", result.Regeocode.AddressComponent.Country,
		"province", result.Regeocode.AddressComponent.Province,
		"city", city,
		"district", result.Regeocode.AddressComponent.District,
		"township", result.Regeocode.AddressComponent.Township,
	)

	country := result.Regeocode.AddressComponent.Country
	province := result.Regeocode.AddressComponent.Province
	district := result.Regeocode.AddressComponent.District
	township := result.Regeocode.AddressComponent.Township

	street := result.Regeocode.AddressComponent.StreetNumber.Street
	streetNumber := ""
	if n, ok := result.Regeocode.AddressComponent.StreetNumber.Number.(string); ok {
		streetNumber = n
	}

	businessArea := ""
	if len(result.Regeocode.AddressComponent.BusinessAreas) > 0 {
		businessArea = result.Regeocode.AddressComponent.BusinessAreas[0].Name
	}

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
		FormattedAddress: result.Regeocode.FormattedAddress,
	}, nil
}
