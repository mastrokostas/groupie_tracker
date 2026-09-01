package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Coordinates holds one geographic point on the map.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// nominatim_result mirrors one element of the Nominatim search response.
// Nominatim returns the coordinates as quoted strings rather than numbers,
// so they have to be parsed with strconv after decoding.
type nominatim_result struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// coordinates_cache maps a raw API location such as "north_carolina-usa"
// to its point. Filled once at startup, read on every artist page request.
var coordinates_cache = map[string]Coordinates{}

// coordinates_cache_mutex guards the cache. Every request runs in its own
// goroutine, so LookupCoordinates is called concurrently.
var coordinates_cache_mutex sync.RWMutex

// coordinates_file_path is where the cache is persisted between runs.
// A var rather than a const so tests can redirect it into t.TempDir().
var coordinates_file_path = "data/coordinates.json"

// nominatim_request_interval is the pause between geocoding requests.
// Nominatim's usage policy allows a maximum of one request per second.
// A var so tests can set it to zero and not sleep.
var nominatim_request_interval = time.Second

// nominatim_user_agent identifies this application to Nominatim, which
// rejects requests that carry no User-Agent or the default one set by
// Go's http package.
var nominatim_user_agent = "groupie-tracker/1.0 (https://github.com/mastrokostas)"

// geocode_query_overrides maps raw API locations whose country no longer
// exists onto a name Nominatim can still resolve. The Netherlands Antilles
// was dissolved in 2010 and Willemstad is now the capital of Curacao.
var geocode_query_overrides = map[string]string{
	"willemstad-netherlands_antilles": "willemstad, curacao",
}

// buildGeocodeQuery turns a raw API location into an address Nominatim can
// resolve. Underscores become spaces and the hyphen separating the city
// from the country becomes a comma, so "north_carolina-usa" becomes
// "north carolina, usa".
func buildGeocodeQuery(raw_location string) string {
	override_query, has_override := geocode_query_overrides[raw_location]
	if has_override {
		return override_query
	}

	with_spaces := strings.ReplaceAll(raw_location, "_", " ")

	return strings.ReplaceAll(with_spaces, "-", ", ")
}

// geocodeLocation asks Nominatim for the coordinates of one raw API
// location. It returns an error rather than a zero value whenever the
// place cannot be resolved, so the caller can log it and move on.
func geocodeLocation(raw_location string) (Coordinates, error) {
	query_parameters := url.Values{}
	query_parameters.Set("q", buildGeocodeQuery(raw_location))
	query_parameters.Set("format", "json")
	query_parameters.Set("limit", "1")

	request_url := APIs.GeocodeURL + "?" + query_parameters.Encode()

	request, err := http.NewRequest(http.MethodGet, request_url, nil)
	if err != nil {
		return Coordinates{}, err
	}

	// Nominatim rejects requests that do not identify the application.
	request.Header.Set("User-Agent", nominatim_user_agent)

	response, err := httpClient.Do(request)
	if err != nil {
		return Coordinates{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return Coordinates{}, fmt.Errorf("unexpected status %d geocoding %s", response.StatusCode, raw_location)
	}

	var results []nominatim_result
	if err := json.NewDecoder(response.Body).Decode(&results); err != nil {
		return Coordinates{}, err
	}

	if len(results) == 0 {
		return Coordinates{}, fmt.Errorf("no result for %s", raw_location)
	}

	latitude, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return Coordinates{}, fmt.Errorf("bad latitude %q for %s", results[0].Lat, raw_location)
	}

	longitude, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return Coordinates{}, fmt.Errorf("bad longitude %q for %s", results[0].Lon, raw_location)
	}

	return Coordinates{Latitude: latitude, Longitude: longitude}, nil
}

// loadCoordinatesFile reads the persisted cache from disk into memory.
// A missing file is not an error: on the very first run it does not exist
// yet, and the program simply starts with an empty cache.
func loadCoordinatesFile() error {
	file_contents, err := os.ReadFile(coordinates_file_path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	loaded_coordinates := map[string]Coordinates{}
	if err := json.Unmarshal(file_contents, &loaded_coordinates); err != nil {
		return err
	}

	coordinates_cache_mutex.Lock()
	defer coordinates_cache_mutex.Unlock()
	coordinates_cache = loaded_coordinates

	return nil
}

// saveCoordinatesFile writes the in memory cache back to disk, creating the
// directory if it does not exist yet.
func saveCoordinatesFile() error {
	if err := os.MkdirAll(filepath.Dir(coordinates_file_path), 0o755); err != nil {
		return err
	}

	coordinates_cache_mutex.RLock()
	file_contents, err := json.MarshalIndent(coordinates_cache, "", "  ")
	coordinates_cache_mutex.RUnlock()

	if err != nil {
		return err
	}

	return os.WriteFile(coordinates_file_path, file_contents, 0o644)
}

// collectUniqueLocations walks every artist's DatesLocations and returns
// each distinct raw location once, sorted. Sorting makes the geocoding
// order deterministic, since Go randomises map iteration order.
func collectUniqueLocations() []string {
	seen_locations := map[string]bool{}

	for _, relation := range Relations {
		for raw_location := range relation.DatesLocations {
			seen_locations[raw_location] = true
		}
	}

	unique_locations := make([]string, 0, len(seen_locations))
	for raw_location := range seen_locations {
		unique_locations = append(unique_locations, raw_location)
	}

	sort.Strings(unique_locations)

	return unique_locations
}

// GeocodeAllLocations fills the cache for every location in the dataset.
// It loads whatever was persisted from previous runs first, so only
// locations that have never been geocoded cost a network request. A
// location that cannot be resolved is logged and skipped, never fatal:
// it simply gets no marker on the map.
func GeocodeAllLocations() error {
	if err := loadCoordinatesFile(); err != nil {
		return err
	}

	unique_locations := collectUniqueLocations()
	newly_geocoded_count := 0

	for i, raw_location := range unique_locations {
		if _, already_cached := LookupCoordinates(raw_location); already_cached {
			continue
		}

		coordinates, err := geocodeLocation(raw_location)
		if err != nil {
			log.Printf("Failed to geocode (%d/189)%s: %v", i, raw_location, err)
			time.Sleep(nominatim_request_interval)

			continue
		} else {
			log.Printf("Decoded Location Successfully (%d/189): %s", i+1, raw_location)
		}

		coordinates_cache_mutex.Lock()
		coordinates_cache[raw_location] = coordinates
		coordinates_cache_mutex.Unlock()

		newly_geocoded_count++

		time.Sleep(nominatim_request_interval)
	}

	if newly_geocoded_count == 0 {
		return nil
	}

	log.Printf("Geocoded %d new locations", newly_geocoded_count)

	return saveCoordinatesFile()
}

// LookupCoordinates returns the cached point for a raw location, and
// whether one was found at all.
func LookupCoordinates(raw_location string) (Coordinates, bool) {
	coordinates_cache_mutex.RLock()
	defer coordinates_cache_mutex.RUnlock()

	coordinates, found := coordinates_cache[raw_location]

	return coordinates, found
}
