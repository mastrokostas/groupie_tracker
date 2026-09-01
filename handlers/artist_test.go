package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// writeTempTemplate creates templates/<name> with the given content and
// registers a cleanup that removes the whole templates directory again,
// so tests are self-contained even though the handlers hard-code the
// "templates/" path.
func writeTempTemplate(t *testing.T, name, content string) {
	t.Helper()

	if err := os.MkdirAll("templates", 0o755); err != nil {
		t.Fatalf("could not create templates dir: %v", err)
	}
	path := "templates/" + name
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", path, err)
	}
	t.Cleanup(func() {
		os.RemoveAll("templates")
	})
}

func TestCapitalizeWord(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"single letter", "a", "A"},
		{"already capitalized", "New", "New"},
		{"all lower case", "york", "York"},
		{"unicode first letter", "évidence", "Évidence"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := capitalizeWord(tc.input)
			if got != tc.want {
				t.Errorf("capitalizeWord(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatLocationSegment(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"short abbreviation", "usa", "USA"},
		{"short abbreviation uk", "uk", "UK"},
		{"four letter country not abbreviated", "chad", "Chad"},
		{"single word city", "paris", "Paris"},
		{"two word city with underscore", "new_york", "New York"},
		{"short segment with underscore is not abbreviated", "le_puy", "Le Puy"},
		{"empty segment", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatLocationSegment(tc.input)
			if got != tc.want {
				t.Errorf("formatLocationSegment(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatLocation(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"city and abbreviated country", "new_york-usa", "New York - USA"},
		{"city and full country name", "paris-france", "Paris - France"},
		{"single segment no hyphen", "chad", "Chad"},
		{"multi word country", "san_francisco-united_states", "San Francisco - United States"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLocation(tc.input)
			if got != tc.want {
				t.Errorf("FormatLocation(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseConcertDate(t *testing.T) {
	t.Run("valid date", func(t *testing.T) {
		got := parseConcertDate("15-06-2022")
		if got.IsZero() {
			t.Fatalf("expected a valid time, got zero value")
		}
		if got.Day() != 15 || got.Month().String() != "June" || got.Year() != 2022 {
			t.Errorf("parseConcertDate(%q) = %v, unexpected day/month/year", "15-06-2022", got)
		}
	})

	t.Run("invalid date returns zero value", func(t *testing.T) {
		got := parseConcertDate("not-a-date")
		if !got.IsZero() {
			t.Errorf("expected zero time for invalid input, got %v", got)
		}
	})
}

func TestSortDatesLocations(t *testing.T) {
	relation := Relation{
		ID: 1,
		DatesLocations: map[string][]string{
			"paris-france": {"15-06-2022", "01-01-2020", "20-03-2021"},
			"new_york-usa": {"10-10-2019"},
		},
	}

	sortDatesLocations(relation)

	wantParis := []string{"01-01-2020", "20-03-2021", "15-06-2022"}
	gotParis := relation.DatesLocations["paris-france"]
	for i := range wantParis {
		if gotParis[i] != wantParis[i] {
			t.Errorf("paris-france[%d] = %q, want %q (full slice: %v)", i, gotParis[i], wantParis[i], gotParis)
		}
	}

	wantNY := []string{"10-10-2019"}
	gotNY := relation.DatesLocations["new_york-usa"]
	if len(gotNY) != 1 || gotNY[0] != wantNY[0] {
		t.Errorf("new_york-usa = %v, want %v", gotNY, wantNY)
	}
}

func TestArtistHandler_InvalidID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/artist?id=notanumber", nil)
	rec := httptest.NewRecorder()

	ArtistHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestArtistHandler_IDOutOfRange(t *testing.T) {
	// Give the package-level Artists/Relations slices known, small content
	// so the boundary checks are deterministic regardless of test order.
	oldArtists, oldRelations := Artists, Relations
	Artists = []Artist{{ID: 1, Name: "Test Artist"}}
	Relations = []Relation{{ID: 1, DatesLocations: map[string][]string{}}}
	t.Cleanup(func() {
		Artists, Relations = oldArtists, oldRelations
	})

	cases := []struct {
		name string
		id   string
	}{
		{"zero id", "0"},
		{"negative id", "-1"},
		{"id beyond length", "99"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/artist?id="+tc.id, nil)
			rec := httptest.NewRecorder()

			ArtistHandler(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestArtistHandler_Success(t *testing.T) {
	// Minimal template exercising both the custom formatLocation function
	// and the fact that dates should already be sorted by the handler.
	writeTempTemplate(t, "artist.html", `
		{{define "artist.html"}}
		Name: {{.A.Name}}
		{{range $loc, $dates := .R.DatesLocations}}
		{{formatLocation $loc}}: {{range $dates}}{{.}} {{end}}
		{{end}}
		{{end}}
	`)

	oldArtists, oldRelations := Artists, Relations
	Artists = []Artist{{ID: 1, Name: "Queen"}}
	Relations = []Relation{{
		ID: 1,
		DatesLocations: map[string][]string{
			"new_york-usa": {"15-06-2022", "01-01-2020"},
		},
	}}
	t.Cleanup(func() {
		Artists, Relations = oldArtists, oldRelations
	})

	req := httptest.NewRequest(http.MethodGet, "/artist?id=1", nil)
	rec := httptest.NewRecorder()

	ArtistHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Queen") {
		t.Errorf("body does not contain artist name: %s", body)
	}
	if !strings.Contains(body, "New York - USA") {
		t.Errorf("body does not contain formatted location: %s", body)
	}
	// The older date must appear before the newer one once sorted.
	if idx1, idx2 := strings.Index(body, "01-01-2020"), strings.Index(body, "15-06-2022"); idx1 == -1 || idx2 == -1 || idx1 > idx2 {
		t.Errorf("expected dates to be sorted chronologically in body: %s", body)
	}
}

func TestBuildConcertStops_OrdersByEarliestDate(t *testing.T) {
	original_cache := coordinates_cache
	coordinates_cache = map[string]Coordinates{}
	t.Cleanup(func() { coordinates_cache = original_cache })

	relation := Relation{DatesLocations: map[string][]string{
		"aalborg-denmark":    {"20-11-2019"},
		"zurich-switzerland": {"03-02-2019"},
		"london-uk":          {"14-06-2019"},
	}}

	concert_stops := buildConcertStops(relation)

	expected_order := []string{"zurich-switzerland", "london-uk", "aalborg-denmark"}

	for stop_index, expected_location := range expected_order {
		if concert_stops[stop_index].RawLocation != expected_location {
			t.Errorf("position %d: got %s, want %s",
				stop_index, concert_stops[stop_index].RawLocation, expected_location)
		}
		if concert_stops[stop_index].Order != stop_index+1 {
			t.Errorf("position %d: got order %d, want %d",
				stop_index, concert_stops[stop_index].Order, stop_index+1)
		}
	}
}

func TestBuildConcertStops_MarksUngeocodedLocations(t *testing.T) {
	original_cache := coordinates_cache
	coordinates_cache = map[string]Coordinates{
		"zurich-switzerland": {Latitude: 47.3769, Longitude: 8.5417},
	}
	t.Cleanup(func() { coordinates_cache = original_cache })

	relation := Relation{DatesLocations: map[string][]string{
		"zurich-switzerland": {"03-02-2019"},
		"nowhere-atlantis":   {"14-06-2019"},
	}}

	concert_stops := buildConcertStops(relation)

	if !concert_stops[0].HasCoordinates {
		t.Error("zurich should have coordinates")
	}
	if concert_stops[1].HasCoordinates {
		t.Error("nowhere should not have coordinates")
	}
	if concert_stops[1].Latitude != 0 || concert_stops[1].Longitude != 0 {
		t.Error("an ungeocoded stop should carry zero coordinates")
	}
}

func TestMapDataJSON(t *testing.T) {
	concert_stops := []ConcertStop{{
		RawLocation:       "zurich-switzerland",
		FormattedLocation: "Zurich - Switzerland",
		Dates:             []string{"03-02-2019"},
		Latitude:          47.3769,
		Longitude:         8.5417,
		HasCoordinates:    true,
		Order:             1,
	}}

	encoded_stops, err := mapDataJSON(concert_stops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded_stops := []map[string]interface{}{}
	if err := json.Unmarshal([]byte(encoded_stops), &decoded_stops); err != nil {
		t.Fatalf("output is not valid json: %v", err)
	}

	if decoded_stops[0]["location"] != "Zurich - Switzerland" {
		t.Errorf("got %v, want Zurich - Switzerland", decoded_stops[0]["location"])
	}
	if decoded_stops[0]["hasCoordinates"] != true {
		t.Error("hasCoordinates missing or false")
	}
	if _, present := decoded_stops[0]["RawLocation"]; present {
		t.Error("RawLocation should be excluded by its json:\"-\" tag")
	}
}
