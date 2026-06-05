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

// GoogleGeocoder 封装 Google Maps Geocoding API 客户端。
// 通过 Google Maps Geocoding API 将经纬度坐标转换为结构化地址。
// 用于海外坐标的逆地理编码回退。
type GoogleGeocoder struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewGoogleGeocoder 创建一个新的 GoogleGeocoder 实例。
// apiKey 为 Google Maps Platform 的 API Key。
func NewGoogleGeocoder(apiKey string) *GoogleGeocoder {
	return &GoogleGeocoder{
		apiKey: apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:   "https://geocode.googleapis.com/v4/geocode/location",
	}
}

// ReverseGeocode 将经纬度坐标转换为结构化地址信息。
// 返回的 GeoInfo 包含省、市、区、街道等地址层级信息。
// 如果坐标无效或无结果，返回 (nil, nil)。
// 如果 API 调用失败，返回错误。
func (g *GoogleGeocoder) ReverseGeocode(ctx context.Context, lat, lon float64) (*types.GeoInfo, error) {
	if g.apiKey == "" {
		return nil, nil
	}

	url := fmt.Sprintf("%s/%f,%f?languageCode=zh-CN&key=%s", g.baseURL, lat, lon, g.apiKey)

	slog.Debug("geocoder: google reverse geocode request",
		"lat", lat,
		"lon", lon,
		"url", fmt.Sprintf("%s/%f,%f?languageCode=zh-CN&key=***", g.baseURL, lat, lon),
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
		// Try to parse v4 error body for better message
		var errBody struct {
			Error struct {
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		errDetail := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if json.NewDecoder(resp.Body).Decode(&errBody) == nil && errBody.Error.Message != "" {
			errDetail = errBody.Error.Message
		}
		return nil, fmt.Errorf("google api error: %s", errDetail)
	}

	var result struct {
		Results []struct {
			FormattedAddress  string `json:"formattedAddress"`
			AddressComponents []struct {
				LongText  string   `json:"longText"`
				ShortText string   `json:"shortText"`
				Types     []string `json:"types"`
			} `json:"addressComponents"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Results) == 0 {
		return nil, nil
	}

	components := result.Results[0].AddressComponents

	country := ""
	province := ""
	city := ""
	district := ""
	township := ""
	street := ""
	streetNumber := ""

	for _, c := range components {
		for _, t := range c.Types {
			switch t {
			case "country":
				country = c.LongText
			case "administrative_area_level_1":
				province = c.LongText
			// locality 是最精确的城市级类型，优先使用
			// 海外地址通常没有 locality，改用 administrative_area_level_2
			case "locality":
				city = c.LongText
			case "administrative_area_level_2":
				if city == "" {
					city = c.LongText
				}
			// sublocality 优先，海外地址退回到 administrative_area_level_3
			case "sublocality":
				district = c.LongText
			case "administrative_area_level_3":
				if district == "" {
					district = c.LongText
				}
			// sublocality_level_2 优先，海外地址退回到 administrative_area_level_4
			case "sublocality_level_2":
				township = c.LongText
			case "administrative_area_level_4":
				if township == "" {
					township = c.LongText
				}
			case "route":
				street = c.LongText
			case "street_number":
				streetNumber = c.LongText
			}
		}
	}

	address := country + province + city + district + township + street + streetNumber

	slog.Debug("geocoder: google reverse geocode response",
		"formatted_address", result.Results[0].FormattedAddress,
		"country", country,
		"province", province,
		"city", city,
		"district", district,
		"township", township,
		"street", street,
		"streetNumber", streetNumber,
	)

	return &types.GeoInfo{
		Country:          country,
		Province:         province,
		City:             city,
		District:         district,
		Township:         township,
		Street:           street,
		StreetNumber:     streetNumber,
		Address:          address,
		FormattedAddress: result.Results[0].FormattedAddress,
	}, nil
}
