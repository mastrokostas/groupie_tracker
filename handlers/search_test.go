package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// indexedFact is the part of a SearchEntry the tests care about. Comparing the
// whole entry would only be comparing LowerLabel against ToLower of Label, which
// TestBuildSearchIndex_LowersLabels checks once on its own.
type indexedFact struct {
	label     string
	fact_type string
	artist_id int
}

// searchTestArtists is the stand-in for the live data that these tests run
// against. It is built around the subject's own example: "Phil Collins" is both
// a band and a member of another band, and "Philadelphia" is a place whose name
// begins with the same letters, so one query exercises three of the five types
// and the ranking between them at once.
//
// The locations are given out of order, and artist 3 has no entry at all, so the
// index has to look its locations up by ID rather than by position.
func searchTestArtists() ([]Artist, []Location) {
	artists := []Artist{
		{
			ID:           1,
			Name:         "Genesis",
			Members:      []string{"Phil Collins", "Tony Banks"},
			CreationDate: 1967,
			FirstAlbum:   "07-03-1969",
		},
		{
			ID:           2,
			Name:         "Phil Collins",
			Members:      []string{"Phil Collins"},
			CreationDate: 1981,
			FirstAlbum:   "13-02-1981",
		},
		{
			ID:           3,
			Name:         "Queen",
			Members:      []string{"Freddie Mercury"},
			CreationDate: 1970,
			FirstAlbum:   "13-07-1973",
		},
	}

	locations := []Location{
		{ID: 2, Locations: []string{"philadelphia-usa"}},
		{ID: 1, Locations: []string{"new_york-usa"}},
	}

	return artists, locations
}

// collectIndexedFacts reduces an index to the three fields the tests assert on.
func collectIndexedFacts(search_index []SearchEntry) []indexedFact {
	indexed_facts := make([]indexedFact, 0, len(search_index))

	for _, entry := range search_index {
		indexed_facts = append(indexed_facts, indexedFact{
			label:     entry.Label,
			fact_type: entry.Type,
			artist_id: entry.ArtistID,
		})
	}

	return indexed_facts
}

// collectSuggestedFacts reduces a list of suggestions the same way, so the two
// can be written out in the same shape.
func collectSuggestedFacts(suggestions []Suggestion) []indexedFact {
	suggested_facts := make([]indexedFact, 0, len(suggestions))

	for _, suggestion := range suggestions {
		suggested_facts = append(suggested_facts, indexedFact{
			label:     suggestion.Label,
			fact_type: suggestion.Type,
			artist_id: suggestion.ArtistID,
		})
	}

	return suggested_facts
}

func TestBuildSearchIndex(t *testing.T) {
	artists, locations := searchTestArtists()

	// All five types, in the order one artist contributes them, for each of the
	// three artists in turn. Locations appear in their readable form, and the
	// creation year as text.
	want := []indexedFact{
		{"Genesis", search_type_artist, 1},
		{"Phil Collins", search_type_member, 1},
		{"Tony Banks", search_type_member, 1},
		{"New York - USA", search_type_location, 1},
		{"07-03-1969", search_type_first_album, 1},
		{"1967", search_type_creation, 1},

		{"Phil Collins", search_type_artist, 2},
		{"Phil Collins", search_type_member, 2},
		{"Philadelphia - USA", search_type_location, 2},
		{"13-02-1981", search_type_first_album, 2},
		{"1981", search_type_creation, 2},

		// Artist 3 has no locations entry, so it contributes no location row.
		{"Queen", search_type_artist, 3},
		{"Freddie Mercury", search_type_member, 3},
		{"13-07-1973", search_type_first_album, 3},
		{"1970", search_type_creation, 3},
	}

	got := collectIndexedFacts(BuildSearchIndex(artists, locations))

	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildSearchIndex() =\n%v\nwant\n%v", got, want)
	}
}

func TestBuildSearchIndex_LowersLabels(t *testing.T) {
	artists := []Artist{{ID: 1, Name: "AC/DC", Members: []string{"Angus Young"}}}

	search_index := BuildSearchIndex(artists, nil)

	cases := []struct {
		name            string
		entry_index     int
		want_label      string
		want_lower_only string
	}{
		{"band name", 0, "AC/DC", "ac/dc"},
		{"member name", 1, "Angus Young", "angus young"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := search_index[tc.entry_index]

			if entry.Label != tc.want_label {
				t.Errorf("Label = %q, want %q", entry.Label, tc.want_label)
			}
			if entry.LowerLabel != tc.want_lower_only {
				t.Errorf("LowerLabel = %q, want %q", entry.LowerLabel, tc.want_lower_only)
			}
		})
	}
}

func TestBuildSearchIndex_SkipsUnusableValues(t *testing.T) {
	// A blank first album date, a first album date that is nothing but spaces,
	// a creation year the API never filled in, and a member with no name: none
	// of them can be searched for, and a blank row would lead nowhere.
	artists := []Artist{
		{ID: 1, Name: "No Dates", Members: []string{"Someone", "   "}, CreationDate: 0, FirstAlbum: ""},
		{ID: 2, Name: "Blank Album", Members: nil, CreationDate: 1990, FirstAlbum: "   "},
	}

	want := []indexedFact{
		{"No Dates", search_type_artist, 1},
		{"Someone", search_type_member, 1},

		{"Blank Album", search_type_artist, 2},
		{"1990", search_type_creation, 2},
	}

	got := collectIndexedFacts(BuildSearchIndex(artists, nil))

	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildSearchIndex() =\n%v\nwant\n%v", got, want)
	}
}

func TestSearch_TypeLabels(t *testing.T) {
	artists, locations := searchTestArtists()
	search_index := BuildSearchIndex(artists, locations)

	// The subject's own example: typing "phil" has to bring back both the band
	// and the member, each saying which of the two it is. The order is the
	// ranking at work — every one of these begins with the query, so they fall
	// into type order, and the two identical member labels are settled by the
	// band each belongs to.
	want := []indexedFact{
		{"Phil Collins", search_type_artist, 2},
		{"Phil Collins", search_type_member, 1},
		{"Phil Collins", search_type_member, 2},
		{"Philadelphia - USA", search_type_location, 2},
	}

	got := collectSuggestedFacts(Search(search_index, "phil", suggestion_limit).Suggestions)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Search(%q) suggestions =\n%v\nwant\n%v", "phil", got, want)
	}
}

func TestSearch_IsCaseInsensitive(t *testing.T) {
	artists, locations := searchTestArtists()
	search_index := BuildSearchIndex(artists, locations)

	want := Search(search_index, "phil", suggestion_limit)

	cases := []struct {
		name  string
		query string
	}{
		{"all upper case", "PHIL"},
		{"mixed case", "PhIl"},
		{"surrounded by spaces", "  phil  "},
		{"upper case and spaces", "  PHIL "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Search(search_index, tc.query, suggestion_limit)

			if !reflect.DeepEqual(got, want) {
				t.Errorf("Search(%q) =\n%v\nwant the same as Search(%q) =\n%v",
					tc.query, got, "phil", want)
			}
		})
	}
}

func TestSearch_Dates(t *testing.T) {
	artists, locations := searchTestArtists()
	search_index := BuildSearchIndex(artists, locations)

	// A year reaches both date types: it is the whole of a creation date and the
	// tail of a first album date. The creation date comes first because its
	// label begins with the query while the other only contains it.
	want := []indexedFact{
		{"1981", search_type_creation, 2},
		{"13-02-1981", search_type_first_album, 2},
	}

	got := collectSuggestedFacts(Search(search_index, "1981", suggestion_limit).Suggestions)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Search(%q) suggestions =\n%v\nwant\n%v", "1981", got, want)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	artists, locations := searchTestArtists()
	search_index := BuildSearchIndex(artists, locations)

	cases := []struct {
		name  string
		query string
	}{
		{"empty string", ""},
		{"spaces only", "   "},
		{"a tab", "\t"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Search(search_index, tc.query, suggestion_limit)

			if len(got.Suggestions) != 0 {
				t.Errorf("Search(%q) returned %d suggestions, want none", tc.query, len(got.Suggestions))
			}
			if len(got.ArtistIDs) != 0 {
				t.Errorf("Search(%q) returned %d artist IDs, want none", tc.query, len(got.ArtistIDs))
			}

			// Empty and not nil, so the endpoint answers [] rather than null.
			if got.Suggestions == nil || got.ArtistIDs == nil {
				t.Error("Search() returned a nil slice, want an empty one")
			}
		})
	}
}

func TestSearch_NoMatches(t *testing.T) {
	artists, locations := searchTestArtists()
	search_index := BuildSearchIndex(artists, locations)

	got := Search(search_index, "zzzz", suggestion_limit)

	if len(got.Suggestions) != 0 || len(got.ArtistIDs) != 0 {
		t.Errorf("Search(%q) = %v, want nothing at all", "zzzz", got)
	}
	if got.Suggestions == nil || got.ArtistIDs == nil {
		t.Error("Search() returned a nil slice, want an empty one")
	}
}

func TestSearch_LimitCapsSuggestionsOnly(t *testing.T) {
	artists, locations := searchTestArtists()
	search_index := BuildSearchIndex(artists, locations)

	// "phil" matches four entries across both artist 1 and artist 2. With room
	// for one row only, the dropdown shows the best of them — but the grid must
	// still be told about artist 1, whose only matching rows were cut.
	got := Search(search_index, "phil", 1)

	want_suggestions := []indexedFact{
		{"Phil Collins", search_type_artist, 2},
	}
	if !reflect.DeepEqual(collectSuggestedFacts(got.Suggestions), want_suggestions) {
		t.Errorf("suggestions =\n%v\nwant\n%v", collectSuggestedFacts(got.Suggestions), want_suggestions)
	}

	want_artist_ids := []int{1, 2}
	if !reflect.DeepEqual(got.ArtistIDs, want_artist_ids) {
		t.Errorf("ArtistIDs = %v, want %v", got.ArtistIDs, want_artist_ids)
	}
}

func TestSearch_ArtistIDsAreDistinctAndSorted(t *testing.T) {
	artists, locations := searchTestArtists()
	search_index := BuildSearchIndex(artists, locations)

	// Artist 2 matches through four of its own facts at once, and artist 1
	// through one. The grid wants each artist named once.
	got := Search(search_index, "phil", suggestion_limit).ArtistIDs

	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ArtistIDs = %v, want %v", got, want)
	}
}

func TestSearchHandler(t *testing.T) {
	artists, locations := searchTestArtists()

	old_artists, old_locations := Artists, Locations
	Artists, Locations = artists, locations
	t.Cleanup(func() { Artists, Locations = old_artists, old_locations })

	req := httptest.NewRequest(http.MethodGet, "/search?q=PHIL", nil)
	rec := httptest.NewRecorder()

	SearchHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	content_type := rec.Header().Get("Content-Type")
	if content_type != "application/json" {
		t.Errorf("Content-Type = %q, want %q", content_type, "application/json")
	}

	var search_response SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &search_response); err != nil {
		t.Fatalf("body is not valid JSON: %v, body=%s", err, rec.Body.String())
	}

	want_suggestions := []indexedFact{
		{"Phil Collins", search_type_artist, 2},
		{"Phil Collins", search_type_member, 1},
		{"Phil Collins", search_type_member, 2},
		{"Philadelphia - USA", search_type_location, 2},
	}
	if !reflect.DeepEqual(collectSuggestedFacts(search_response.Suggestions), want_suggestions) {
		t.Errorf("suggestions =\n%v\nwant\n%v",
			collectSuggestedFacts(search_response.Suggestions), want_suggestions)
	}

	if !reflect.DeepEqual(search_response.ArtistIDs, []int{1, 2}) {
		t.Errorf("ArtistIDs = %v, want %v", search_response.ArtistIDs, []int{1, 2})
	}

	// The band each row belongs to is what tells the two identical member labels
	// apart, so it has to survive the round trip.
	if search_response.Suggestions[1].ArtistName != "Genesis" {
		t.Errorf("second suggestion's artistName = %q, want %q",
			search_response.Suggestions[1].ArtistName, "Genesis")
	}
}

func TestSearchHandler_EmptyResultsAreArraysNotNull(t *testing.T) {
	artists, locations := searchTestArtists()

	old_artists, old_locations := Artists, Locations
	Artists, Locations = artists, locations
	t.Cleanup(func() { Artists, Locations = old_artists, old_locations })

	// A body of null would make the browser script fall over on .length, so the
	// exact text is asserted rather than the decoded value.
	want_body := `{"suggestions":[],"artistIDs":[]}`

	cases := []struct {
		name   string
		target string
	}{
		{"no matches", "/search?q=zzzz"},
		{"empty query", "/search?q="},
		{"no query at all", "/search"},
		{"spaces only", "/search?q=%20%20"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			rec := httptest.NewRecorder()

			SearchHandler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if rec.Body.String() != want_body {
				t.Errorf("body = %s, want %s", rec.Body.String(), want_body)
			}
		})
	}
}
