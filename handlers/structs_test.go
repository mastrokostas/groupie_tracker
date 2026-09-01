package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// withMockAPIs spins up one httptest server per endpoint, points the
// package-level APIs config at them, and restores everything afterwards.
func withMockAPIs(t *testing.T, artistsBody, locationsBody, datesBody, relationBody string, statusOverride map[string]int) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/artists", func(w http.ResponseWriter, r *http.Request) {
		if code, ok := statusOverride["artists"]; ok {
			w.WriteHeader(code)
			return
		}
		w.Write([]byte(artistsBody))
	})
	mux.HandleFunc("/locations", func(w http.ResponseWriter, r *http.Request) {
		if code, ok := statusOverride["locations"]; ok {
			w.WriteHeader(code)
			return
		}
		w.Write([]byte(locationsBody))
	})
	mux.HandleFunc("/dates", func(w http.ResponseWriter, r *http.Request) {
		if code, ok := statusOverride["dates"]; ok {
			w.WriteHeader(code)
			return
		}
		w.Write([]byte(datesBody))
	})
	mux.HandleFunc("/relation", func(w http.ResponseWriter, r *http.Request) {
		if code, ok := statusOverride["relation"]; ok {
			w.WriteHeader(code)
			return
		}
		w.Write([]byte(relationBody))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	oldAPIs := APIs
	oldArtists, oldLocations, oldDates, oldRelations := Artists, Locations, DatesData, Relations
	t.Cleanup(func() {
		APIs = oldAPIs
		Artists, Locations, DatesData, Relations = oldArtists, oldLocations, oldDates, oldRelations
	})

	APIs = APIConfig{
		ArtistsURL:   server.URL + "/artists",
		LocationsURL: server.URL + "/locations",
		DatesURL:     server.URL + "/dates",
		RelationURL:  server.URL + "/relation",
	}
}

func TestFetchAll_Success(t *testing.T) {
	withMockAPIs(t,
		`[{"id":1,"name":"Queen"}]`,
		`{"index":[{"id":1,"locations":["paris-france"],"dates":"https://example.com/dates/1"}]}`,
		`{"index":[{"id":1,"dates":["15-06-2022"]}]}`,
		`{"index":[{"id":1,"datesLocations":{"paris-france":["15-06-2022"]}}]}`,
		nil,
	)

	if err := FetchAll(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(Artists) != 1 || Artists[0].Name != "Queen" {
		t.Errorf("Artists = %+v, unexpected", Artists)
	}
	if len(Locations) != 1 || Locations[0].Locations[0] != "paris-france" {
		t.Errorf("Locations = %+v, unexpected", Locations)
	}
	if len(DatesData) != 1 || DatesData[0].Dates[0] != "15-06-2022" {
		t.Errorf("DatesData = %+v, unexpected", DatesData)
	}
	if len(Relations) != 1 || Relations[0].DatesLocations["paris-france"][0] != "15-06-2022" {
		t.Errorf("Relations = %+v, unexpected", Relations)
	}
}

func TestFetchAll_ArtistsFailureStopsEarly(t *testing.T) {
	withMockAPIs(t,
		``, ``, ``, ``,
		map[string]int{"artists": http.StatusInternalServerError},
	)

	if err := FetchAll(); err == nil {
		t.Fatal("expected an error when the artists endpoint fails, got nil")
	}
}

func TestFetchAll_LocationsFailurePropagates(t *testing.T) {
	withMockAPIs(t,
		`[{"id":1,"name":"Queen"}]`,
		``, ``, ``,
		map[string]int{"locations": http.StatusServiceUnavailable},
	)

	if err := FetchAll(); err == nil {
		t.Fatal("expected an error when the locations endpoint fails, got nil")
	}
}

func TestFetchAll_RelationFailurePropagates(t *testing.T) {
	withMockAPIs(t,
		`[{"id":1,"name":"Queen"}]`,
		`{"index":[{"id":1,"locations":["paris-france"],"dates":"x"}]}`,
		`{"index":[{"id":1,"dates":["15-06-2022"]}]}`,
		``,
		map[string]int{"relation": http.StatusServiceUnavailable},
	)

	if err := FetchAll(); err == nil {
		t.Fatal("expected an error when the relation endpoint fails, got nil")
	}
}
