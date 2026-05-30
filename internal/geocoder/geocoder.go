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

type Geocoder struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

func NewGeocoder(apiKey string) *Geocoder {
	return &Geocoder{
		apiKey: apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:   "https://restapi.amap.com/v3/geocode/regeo",
	}
}

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
	)

	return &types.GeoInfo{
		Country:          result.Regeocode.AddressComponent.Country,
		Province:         result.Regeocode.AddressComponent.Province,
		City:             city,
		District:         result.Regeocode.AddressComponent.District,
		FormattedAddress: result.Regeocode.FormattedAddress,
	}, nil
}
