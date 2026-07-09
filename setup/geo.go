package setup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// GeoResult is a LinkedIn location the user can pick. ID is the geoId the
// scraper needs; DisplayName is human-readable (e.g. "Zurich, Switzerland").
type GeoResult struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// ResolveLocation turns a free-text city/region query into LinkedIn geoIds
// using LinkedIn's public jobs-guest typeahead endpoint — the same one their
// website uses — so users never have to dig a geoId out of DevTools.
func ResolveLocation(query string) ([]GeoResult, error) {
	endpoint := "https://www.linkedin.com/jobs-guest/api/typeaheadHits?" + url.Values{
		"query":         {query},
		"typeaheadType": {"GEO"},
	}.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// A browser-like User-Agent avoids being served an error page.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin typeahead returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var hits []struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		DisplayName string `json:"displayName"`
	}
	if err := json.Unmarshal(body, &hits); err != nil {
		return nil, fmt.Errorf("could not parse typeahead response: %w", err)
	}

	results := make([]GeoResult, 0, len(hits))
	seen := map[string]bool{}
	for _, hit := range hits {
		if hit.ID == "" || hit.DisplayName == "" || seen[hit.ID] {
			continue
		}
		seen[hit.ID] = true
		results = append(results, GeoResult{ID: hit.ID, DisplayName: hit.DisplayName})
	}
	return results, nil
}
