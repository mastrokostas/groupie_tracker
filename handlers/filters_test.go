package handlers

import (
	"reflect"
	"testing"
)

func TestFirstAlbumYear(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{"valid date", "22-10-1976", 1976},
		{"earliest date in the data", "01-01-1963", 1963},
		{"unparsable string", "not-a-date", 0},
		{"empty string", "", 0},
		{"year first is not the API layout", "1976-10-22", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := firstAlbumYear(tc.input)
			if got != tc.want {
				t.Errorf("firstAlbumYear(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestSplitLocationSlug(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		want_place   string
		want_country string
	}{
		{"city and country", "brooklyn-usa", "brooklyn", "usa"},
		{"multi word place", "north_carolina-usa", "north_carolina", "usa"},
		{"multi word country", "abu_dhabi-united_arab_emirates", "abu_dhabi", "united_arab_emirates"},
		{"no hyphen at all", "chad", "chad", ""},
		{"empty string", "", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got_place, got_country := splitLocationSlug(tc.input)
			if got_place != tc.want_place || got_country != tc.want_country {
				t.Errorf("splitLocationSlug(%q) = (%q, %q), want (%q, %q)",
					tc.input, got_place, got_country, tc.want_place, tc.want_country)
			}
		})
	}
}

func TestBuildArtistCards(t *testing.T) {
	artists := []Artist{
		{ID: 1, Name: "Queen", Members: []string{"a", "b", "c", "d"}, CreationDate: 1970, FirstAlbum: "13-07-1973"},
		{ID: 2, Name: "Solo", Members: []string{"a"}, CreationDate: 1990, FirstAlbum: "not-a-date"},
		{ID: 3, Name: "Unlisted", Members: []string{"a", "b"}, CreationDate: 2000, FirstAlbum: "01-01-2001"},
	}

	// Deliberately out of order, and with no entry at all for artist 3, so that
	// the lookup has to go by ID rather than by position in the slice.
	locations := []Location{
		{ID: 2, Locations: []string{"paris-france"}},
		{ID: 1, Locations: []string{"london-uk", "new_york-usa"}},
	}

	cards := BuildArtistCards(artists, locations)

	if len(cards) != len(artists) {
		t.Fatalf("got %d cards, want %d", len(cards), len(artists))
	}

	expectations := []struct {
		want_name           string
		want_member_count   int
		want_album_year     int
		want_location_slugs string
	}{
		{"Queen", 4, 1973, "london-uk new_york-usa"},
		{"Solo", 1, 0, "paris-france"},
		{"Unlisted", 2, 2001, ""},
	}

	for card_index, want := range expectations {
		card := cards[card_index]

		if card.Artist.Name != want.want_name {
			t.Errorf("card %d name = %q, want %q", card_index, card.Artist.Name, want.want_name)
		}
		if card.MemberCount != want.want_member_count {
			t.Errorf("%s member count = %d, want %d", want.want_name, card.MemberCount, want.want_member_count)
		}
		if card.FirstAlbumYear != want.want_album_year {
			t.Errorf("%s first album year = %d, want %d", want.want_name, card.FirstAlbumYear, want.want_album_year)
		}
		if card.LocationSlugs != want.want_location_slugs {
			t.Errorf("%s location slugs = %q, want %q", want.want_name, card.LocationSlugs, want.want_location_slugs)
		}
	}
}

func TestBuildFilterOptions(t *testing.T) {
	artists := []Artist{
		{ID: 1, Members: []string{"a", "b", "c", "d"}, CreationDate: 1970, FirstAlbum: "13-07-1973"},
		// An unreadable first album date must not drag the album slider down to 0.
		{ID: 2, Members: []string{"a"}, CreationDate: 1958, FirstAlbum: "not-a-date"},
		// The same band size as artist 1, so it must produce only one check box.
		{ID: 3, Members: []string{"a", "b", "c", "d"}, CreationDate: 2015, FirstAlbum: "01-01-2018"},
	}

	locations := []Location{
		{ID: 1, Locations: []string{"new_york-usa", "london-uk"}},
		// new_york-usa again, so the hierarchy has to deduplicate it.
		{ID: 2, Locations: []string{"new_york-usa", "brooklyn-usa"}},
		{ID: 3, Locations: []string{"abu_dhabi-united_arab_emirates"}},
	}

	options := BuildFilterOptions(artists, locations)

	if options.MinCreationYear != 1958 || options.MaxCreationYear != 2015 {
		t.Errorf("creation bounds = %d-%d, want 1958-2015",
			options.MinCreationYear, options.MaxCreationYear)
	}

	if options.MinAlbumYear != 1973 || options.MaxAlbumYear != 2018 {
		t.Errorf("album bounds = %d-%d, want 1973-2018",
			options.MinAlbumYear, options.MaxAlbumYear)
	}

	want_member_counts := []int{1, 4}
	if !reflect.DeepEqual(options.MemberCounts, want_member_counts) {
		t.Errorf("member counts = %v, want %v", options.MemberCounts, want_member_counts)
	}

	// Countries alphabetically by their readable name, each with its places
	// deduplicated and sorted the same way, and every slug put back together
	// exactly as the API wrote it.
	want_countries := []CountryGroup{
		{
			Slug:   "uk",
			Name:   "UK",
			Places: []PlaceOption{{Slug: "london-uk", Name: "London"}},
		},
		{
			Slug:   "united_arab_emirates",
			Name:   "United Arab Emirates",
			Places: []PlaceOption{{Slug: "abu_dhabi-united_arab_emirates", Name: "Abu Dhabi"}},
		},
		{
			Slug: "usa",
			Name: "USA",
			Places: []PlaceOption{
				{Slug: "brooklyn-usa", Name: "Brooklyn"},
				{Slug: "new_york-usa", Name: "New York"},
			},
		},
	}
	if !reflect.DeepEqual(options.Countries, want_countries) {
		t.Errorf("countries = %+v, want %+v", options.Countries, want_countries)
	}
}

func TestBuildFilterOptions_SkipsUnusableValues(t *testing.T) {
	artists := []Artist{
		// No members at all, so no "0 members" check box may appear.
		{ID: 1, Members: nil, CreationDate: 1990, FirstAlbum: "01-01-1991"},
		// No creation year and no album date, so neither may reach the bounds.
		{ID: 2, Members: []string{"a"}, CreationDate: 0, FirstAlbum: ""},
	}

	// A location with no country cannot be filed under a country heading.
	locations := []Location{{ID: 1, Locations: []string{"chad"}}}

	options := BuildFilterOptions(artists, locations)

	want_member_counts := []int{1}
	if !reflect.DeepEqual(options.MemberCounts, want_member_counts) {
		t.Errorf("member counts = %v, want %v", options.MemberCounts, want_member_counts)
	}

	if options.MinCreationYear != 1990 || options.MaxCreationYear != 1990 {
		t.Errorf("creation bounds = %d-%d, want 1990-1990",
			options.MinCreationYear, options.MaxCreationYear)
	}

	if options.MinAlbumYear != 1991 || options.MaxAlbumYear != 1991 {
		t.Errorf("album bounds = %d-%d, want 1991-1991",
			options.MinAlbumYear, options.MaxAlbumYear)
	}

	if len(options.Countries) != 0 {
		t.Errorf("countries = %+v, want none", options.Countries)
	}
}

func TestBuildHomePageData_CombinesCardsAndFilters(t *testing.T) {
	artists := []Artist{
		{ID: 1, Name: "Queen", Members: []string{"a", "b"}, CreationDate: 1970, FirstAlbum: "13-07-1973"},
	}
	locations := []Location{{ID: 1, Locations: []string{"london-uk"}}}

	page_data := BuildHomePageData(artists, locations)

	if len(page_data.Cards) != 1 || page_data.Cards[0].Artist.Name != "Queen" {
		t.Errorf("cards = %+v, want one card for Queen", page_data.Cards)
	}
	if page_data.Filters.MinCreationYear != 1970 || page_data.Filters.MaxCreationYear != 1970 {
		t.Errorf("creation bounds = %d-%d, want 1970-1970",
			page_data.Filters.MinCreationYear, page_data.Filters.MaxCreationYear)
	}
	if len(page_data.Filters.Countries) != 1 || page_data.Filters.Countries[0].Name != "UK" {
		t.Errorf("countries = %+v, want just UK", page_data.Filters.Countries)
	}
}

func TestBuildHomePageData_EmptyInput(t *testing.T) {
	page_data := BuildHomePageData(nil, nil)

	if len(page_data.Cards) != 0 {
		t.Errorf("cards = %+v, want none", page_data.Cards)
	}
	if page_data.Filters.MinCreationYear != 0 || page_data.Filters.MaxCreationYear != 0 {
		t.Errorf("creation bounds = %d-%d, want 0-0",
			page_data.Filters.MinCreationYear, page_data.Filters.MaxCreationYear)
	}
	if page_data.Filters.MinAlbumYear != 0 || page_data.Filters.MaxAlbumYear != 0 {
		t.Errorf("album bounds = %d-%d, want 0-0",
			page_data.Filters.MinAlbumYear, page_data.Filters.MaxAlbumYear)
	}
	if len(page_data.Filters.MemberCounts) != 0 {
		t.Errorf("member counts = %v, want none", page_data.Filters.MemberCounts)
	}
	if len(page_data.Filters.Countries) != 0 {
		t.Errorf("countries = %+v, want none", page_data.Filters.Countries)
	}
}
