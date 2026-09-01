package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// DataHandler fetches JSON from the given URL and decodes it into dest.
// dest must be a pointer to the desired value (e.g. &[]Artist{}).
func DataHandler(url string, dest interface{}) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	//if the Api anwers with 404 or 500 won'treturn an error,so we handle it manually
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}

// Fetch decodes a JSON array at the given URL into a slice of T and returns it.
func Fetch[T any](url string) ([]T, error) {
	var out []T
	if err := DataHandler(url, &out); err != nil {
		return nil, err
	}
	return out, nil
}
