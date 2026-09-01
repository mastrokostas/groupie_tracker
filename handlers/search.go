package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// The five search cases the subject asks for. They are written out as constants
// so that the index that produces them, the order they are ranked in, and the
// tests that assert on them all read the same strings and cannot drift apart.
const (
	search_type_artist      = "artist/band"
	search_type_member      = "member"
	search_type_location    = "location"
	search_type_first_album = "first album date"
	search_type_creation    = "creation date"
)

// search_type_ranks fixes the order the types appear in when two suggestions are
// otherwise equally good a match, so the dropdown always lists an artist above a
// member above a location, rather than in whatever order the index was walked.
var search_type_ranks = map[string]int{
	search_type_artist:      0,
	search_type_member:      1,
	search_type_location:    2,
	search_type_first_album: 3,
	search_type_creation:    4,
}

// suggestion_limit is how many rows the dropdown is allowed to show. A query of
// one or two letters matches hundreds of entries, and a list that long is no
// more useful than a list of ten while being far harder to read.
const suggestion_limit = 10

// SearchEntry is one searchable fact about one artist: their name, one of their
// members, one place they played, or one of their two dates. LowerLabel is kept
// beside Label so that a query is lowered once and every comparison after that
// is a plain substring check, which is what makes the search case-insensitive.
type SearchEntry struct {
	Label      string
	LowerLabel string
	Type       string
	ArtistID   int
	ArtistName string
}

// Suggestion is one row of the dropdown, in the shape the browser receives it.
// ArtistName is carried alongside the label because a member or a location row
// means little on its own: searching "phil" returns two Phil Collins rows, and
// the band each of them belongs to is what tells them apart.
type Suggestion struct {
	Label      string `json:"label"`
	Type       string `json:"type"`
	ArtistID   int    `json:"artistID"`
	ArtistName string `json:"artistName"`
}

// SearchResponse is the whole answer to one query: the rows the dropdown lists,
// and the artists the grid is narrowed to.
//
// The two are separate because they are capped differently. The dropdown shows
// a handful of rows, while the grid should keep every artist the query reached —
// typing a single letter matches hundreds of entries, and cutting the grid down
// to the ten artists that happened to rank highest would hide artists that do
// match.
type SearchResponse struct {
	Suggestions []Suggestion `json:"suggestions"`
	ArtistIDs   []int        `json:"artistIDs"`
}

// matchedEntry is an index entry that matched a query, kept together with where
// in its label the query was found. That position is what lets the ranking put
// the labels that begin with what was typed ahead of the ones that merely
// contain it somewhere in the middle.
type matchedEntry struct {
	entry       SearchEntry
	match_index int
}

// appendSearchEntry adds one searchable fact to the index. A label that is empty
// once trimmed is skipped rather than indexed: the API leaves a first album date
// blank now and then, and a blank row in the dropdown could match nothing a
// visitor could sensibly type and would lead nowhere useful if clicked.
func appendSearchEntry(search_index []SearchEntry, label, entry_type string, artist Artist) []SearchEntry {
	trimmed_label := strings.TrimSpace(label)
	if trimmed_label == "" {
		return search_index
	}

	return append(search_index, SearchEntry{
		Label:      trimmed_label,
		LowerLabel: strings.ToLower(trimmed_label),
		Type:       entry_type,
		ArtistID:   artist.ID,
		ArtistName: artist.Name,
	})
}

// BuildSearchIndex flattens the artists and their locations into one list of
// searchable facts, one entry per fact per artist. The same label can appear
// more than once under different types, which is exactly what the subject's own
// example asks for: "Phil Collins" is both a band and a member, so it produces
// one row of each.
//
// Locations are indexed in their readable form, "New York - USA" rather than
// "new_york-usa", because that is the form the rest of the site shows and so the
// form a visitor would type.
func BuildSearchIndex(artists []Artist, locations []Location) []SearchEntry {
	locations_by_artist_id := buildLocationsByArtistID(locations)

	// Each artist contributes their name, their members, their locations and
	// their two dates, so the index ends up several times the artist count.
	search_index := make([]SearchEntry, 0, len(artists)*8)

	for _, artist := range artists {
		search_index = appendSearchEntry(search_index, artist.Name, search_type_artist, artist)

		for _, member_name := range artist.Members {
			search_index = appendSearchEntry(search_index, member_name, search_type_member, artist)
		}

		for _, raw_location := range locations_by_artist_id[artist.ID] {
			search_index = appendSearchEntry(
				search_index,
				FormatLocation(raw_location),
				search_type_location,
				artist,
			)
		}

		// The first album date is indexed exactly as the card displays it,
		// "12-07-1973". A substring match already makes that reachable by
		// typing the year on its own, so no second form of the date is added.
		search_index = appendSearchEntry(search_index, artist.FirstAlbum, search_type_first_album, artist)

		// A creation year of zero means the API had none. Rendered it would
		// read as the year "0", which is not something anyone would search for.
		if artist.CreationDate > 0 {
			search_index = appendSearchEntry(
				search_index,
				strconv.Itoa(artist.CreationDate),
				search_type_creation,
				artist,
			)
		}
	}

	return search_index
}

// sortMatchedEntries puts the matches into the order the dropdown lists them:
// the closest match first, and beyond that an order that does not shuffle
// itself between keystrokes.
func sortMatchedEntries(matched_entries []matchedEntry) {
	sort.SliceStable(matched_entries, func(first_index, second_index int) bool {
		first_match := matched_entries[first_index]
		second_match := matched_entries[second_index]

		// A label that begins with what was typed is the closer answer, so
		// typing "phil" lists Phil Collins above Philadelphia.
		first_starts_with_query := first_match.match_index == 0
		second_starts_with_query := second_match.match_index == 0
		if first_starts_with_query != second_starts_with_query {
			return first_starts_with_query
		}

		// Then the fixed order of the five types.
		first_type_rank := search_type_ranks[first_match.entry.Type]
		second_type_rank := search_type_ranks[second_match.entry.Type]
		if first_type_rank != second_type_rank {
			return first_type_rank < second_type_rank
		}

		// Then alphabetically. Compared in lower case so that a label written
		// in capitals does not sort ahead of every lower case one.
		if first_match.entry.LowerLabel != second_match.entry.LowerLabel {
			return first_match.entry.LowerLabel < second_match.entry.LowerLabel
		}

		// And finally by the artist the row belongs to, so that two identical
		// labels from two different artists keep a settled order.
		return first_match.entry.ArtistName < second_match.entry.ArtistName
	})
}

// findMatches returns every entry whose label contains the query, already
// ranked. The query is trimmed and lowered once here, and every entry already
// carries its own lowered label, which is what makes the whole search
// case-insensitive without lowering anything twice.
//
// An empty query matches nothing rather than everything, since a dropdown that
// unfolded the moment the box was focused would only be in the way.
func findMatches(search_index []SearchEntry, raw_query string) []matchedEntry {
	matched_entries := make([]matchedEntry, 0)

	lowered_query := strings.ToLower(strings.TrimSpace(raw_query))
	if lowered_query == "" {
		return matched_entries
	}

	for _, entry := range search_index {
		match_index := strings.Index(entry.LowerLabel, lowered_query)
		if match_index < 0 {
			continue
		}

		matched_entries = append(matched_entries, matchedEntry{
			entry:       entry,
			match_index: match_index,
		})
	}

	sortMatchedEntries(matched_entries)

	return matched_entries
}

// buildSuggestions turns the best of the ranked matches into the rows the
// dropdown shows, at most limit of them.
func buildSuggestions(matched_entries []matchedEntry, limit int) []Suggestion {
	// Made rather than declared, so that no matches is encoded as an empty JSON
	// array instead of null.
	suggestions := make([]Suggestion, 0)
	if limit <= 0 {
		return suggestions
	}

	for _, matched_entry := range matched_entries {
		if len(suggestions) == limit {
			break
		}

		suggestions = append(suggestions, Suggestion{
			Label:      matched_entry.entry.Label,
			Type:       matched_entry.entry.Type,
			ArtistID:   matched_entry.entry.ArtistID,
			ArtistName: matched_entry.entry.ArtistName,
		})
	}

	return suggestions
}

// collectMatchedArtistIDs returns every distinct artist behind the matches,
// ascending. This is the whole match set and not just the part the dropdown had
// room for, because it is what the grid is narrowed to.
func collectMatchedArtistIDs(matched_entries []matchedEntry) []int {
	// The same artist matches through several of their facts at once — their
	// name, two of their members and four of their locations, say — so the IDs
	// are gathered as a set first.
	seen_artist_ids := make(map[int]bool, len(matched_entries))
	for _, matched_entry := range matched_entries {
		seen_artist_ids[matched_entry.entry.ArtistID] = true
	}

	matched_artist_ids := make([]int, 0, len(seen_artist_ids))
	for artist_id := range seen_artist_ids {
		matched_artist_ids = append(matched_artist_ids, artist_id)
	}
	sort.Ints(matched_artist_ids)

	return matched_artist_ids
}

// Search answers one query: the dropdown rows and the artists to keep on screen.
func Search(search_index []SearchEntry, raw_query string, limit int) SearchResponse {
	matched_entries := findMatches(search_index, raw_query)

	return SearchResponse{
		Suggestions: buildSuggestions(matched_entries, limit),
		ArtistIDs:   collectMatchedArtistIDs(matched_entries),
	}
}

// SearchHandler answers GET /search?q= with the suggestions for that query and
// the artists it matched, as JSON. It is the only endpoint on the site that
// returns data rather than a page, and it is what the search box calls as the
// visitor types.
func SearchHandler(w http.ResponseWriter, r *http.Request) {
	raw_query := r.URL.Query().Get("q")

	// The index is built per request rather than once at start up, for the same
	// reason HomeHandler builds its page data per request: the answer always
	// reflects whatever is currently in the package level slices, which is also
	// what keeps the handler tests, which swap those slices out, working. With
	// 52 artists the cost is negligible.
	search_index := BuildSearchIndex(Artists, Locations)
	search_response := Search(search_index, raw_query, suggestion_limit)

	// Marshalled in full before anything is written, so that a failure can still
	// be reported as an error rather than landing halfway into a response body
	// that has already been committed.
	encoded_response, err := json.Marshal(search_response)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(encoded_response)
}
