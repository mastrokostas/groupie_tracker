package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildGeocodeQuery(t *testing.T) {
	test_cases := []struct {
		name           string
		raw_location   string
		expected_query string
	}{
		{"underscore and hyphen", "north_carolina-usa", "north carolina, usa"},
		{"single word city", "zurich-switzerland", "zurich, switzerland"},
		{"abbreviation country", "london-uk", "london, uk"},
	}

	for _, test_case := range test_cases {
		t.Run(test_case.name, func(t *testing.T) {
			result := buildGeocodeQuery(test_case.raw_location)
			if result != test_case.expected_query {
				t.Errorf("got %q, want %q", result, test_case.expected_query)
			}
		})
	}
}

func TestGeocodeLocation_Success(t *testing.T) {
	received_user_agent := ""

	fake_server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received_user_agent = r.Header.Get("User-Agent")
		w.Write([]byte(`[{"lat":"35.7596","lon":"-79.0193"}]`))
	}))
	defer fake_server.Close()

	original_url := APIs.GeocodeURL
	APIs.GeocodeURL = fake_server.URL
	t.Cleanup(func() { APIs.GeocodeURL = original_url })

	coordinates, err := geocodeLocation("north_carolina-usa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coordinates.Latitude != 35.7596 || coordinates.Longitude != -79.0193 {
		t.Errorf("got %v, want 35.7596 / -79.0193", coordinates)
	}
	if received_user_agent != nominatim_user_agent {
		t.Errorf("user agent not sent, got %q", received_user_agent)
	}
}

func TestGeocodeLocation_Failures(t *testing.T) {
	test_cases := []struct {
		name          string
		status_code   int
		response_body string
	}{
		{"empty result array", http.StatusOK, `[]`},
		{"non ok status", http.StatusInternalServerError, `[]`},
		{"invalid json", http.StatusOK, `not json`},
		{"unparseable latitude", http.StatusOK, `[{"lat":"north","lon":"-79.0193"}]`},
		{"unparseable longitude", http.StatusOK, `[{"lat":"35.7596","lon":"west"}]`},
	}

	for _, test_case := range test_cases {
		t.Run(test_case.name, func(t *testing.T) {
			fake_server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test_case.status_code)
				w.Write([]byte(test_case.response_body))
			}))
			defer fake_server.Close()

			original_url := APIs.GeocodeURL
			APIs.GeocodeURL = fake_server.URL
			t.Cleanup(func() { APIs.GeocodeURL = original_url })

			if _, err := geocodeLocation("somewhere-nowhere"); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestCoordinatesFileRoundTrip(t *testing.T) {
	original_path := coordinates_file_path
	original_cache := coordinates_cache
	coordinates_file_path = filepath.Join(t.TempDir(), "coordinates.json")
	coordinates_cache = map[string]Coordinates{
		"zurich-switzerland": {Latitude: 47.3769, Longitude: 8.5417},
	}
	t.Cleanup(func() {
		coordinates_file_path = original_path
		coordinates_cache = original_cache
	})

	if err := saveCoordinatesFile(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	saved_cache := coordinates_cache
	coordinates_cache = map[string]Coordinates{}

	if err := loadCoordinatesFile(); err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !reflect.DeepEqual(coordinates_cache, saved_cache) {
		t.Errorf("got %v, want %v", coordinates_cache, saved_cache)
	}
}

func TestLoadCoordinatesFile_MissingFileIsNotAnError(t *testing.T) {
	original_path := coordinates_file_path
	coordinates_file_path = filepath.Join(t.TempDir(), "does_not_exist.json")
	t.Cleanup(func() { coordinates_file_path = original_path })

	if err := loadCoordinatesFile(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCollectUniqueLocations(t *testing.T) {
	original_relations := Relations
	Relations = []Relation{
		{DatesLocations: map[string][]string{
			"zurich-switzerland": {"03-02-2019"},
			"aalborg-denmark":    {"20-11-2019"},
		}},
		{DatesLocations: map[string][]string{
			"zurich-switzerland": {"11-09-2019"},
			"london-uk":          {"01-01-2020"},
		}},
	}
	t.Cleanup(func() { Relations = original_relations })

	expected_locations := []string{"aalborg-denmark", "london-uk", "zurich-switzerland"}

	if result := collectUniqueLocations(); !reflect.DeepEqual(result, expected_locations) {
		t.Errorf("got %v, want %v", result, expected_locations)
	}
}

func TestGeocodeAllLocations_SkipsCachedLocations(t *testing.T) {
	request_count := 0

	fake_server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request_count++
		w.Write([]byte(`[{"lat":"1.0","lon":"2.0"}]`))
	}))
	defer fake_server.Close()

	original_url := APIs.GeocodeURL
	original_path := coordinates_file_path
	original_interval := nominatim_request_interval
	original_relations := Relations
	original_cache := coordinates_cache

	APIs.GeocodeURL = fake_server.URL
	coordinates_file_path = filepath.Join(t.TempDir(), "coordinates.json")
	nominatim_request_interval = 0
	Relations = []Relation{
		{DatesLocations: map[string][]string{
			"zurich-switzerland": {"03-02-2019"},
			"london-uk":          {"01-01-2020"},
		}},
	}
	coordinates_cache = map[string]Coordinates{
		"zurich-switzerland": {Latitude: 47.3769, Longitude: 8.5417},
	}

	t.Cleanup(func() {
		APIs.GeocodeURL = original_url
		coordinates_file_path = original_path
		nominatim_request_interval = original_interval
		Relations = original_relations
		coordinates_cache = original_cache
	})

	if err := GeocodeAllLocations(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request_count != 1 {
		t.Errorf("got %d requests, want 1 — the cached location should be skipped", request_count)
	}

	written_file, err := os.ReadFile(coordinates_file_path)
	if err != nil {
		t.Fatalf("file was not written: %v", err)
	}

	written_cache := map[string]Coordinates{}
	if err := json.Unmarshal(written_file, &written_cache); err != nil {
		t.Fatalf("written file is not valid json: %v", err)
	}
	if len(written_cache) != 2 {
		t.Errorf("got %d entries, want 2", len(written_cache))
	}
}
