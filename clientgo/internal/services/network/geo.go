package network

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/codevault-llc/xenomorph-client/pkg/types"
)



func GetGeographicLocation() (types.Geographic, error) {
	resp, err := http.Get("https://ipinfo.io/json")
	if err != nil {
		return types.Geographic{}, fmt.Errorf("failed to get geographic location: %w", err)
	}

	defer resp.Body.Close()
	var geo types.Geographic
	if err := json.NewDecoder(resp.Body).Decode(&geo); err != nil {
		return types.Geographic{}, fmt.Errorf("failed to decode geographic location: %w", err)
	}

	if geo.IP == "" {
		return types.Geographic{}, fmt.Errorf("geographic location data is incomplete")
	}

	return geo, nil
}