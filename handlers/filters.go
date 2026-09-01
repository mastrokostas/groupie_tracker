package handlers

import (
	"sort"
	"strings"
)

// PlaceOption is a single concert location as the location filter shows it:
// the raw API value the browser matches on, and a readable label for the check
// box beside it.
type PlaceOption struct {
	Slug string // raw API value, for example "north_carolina-usa"
	Name string // readable place name, for example "North Carolina"
}

// CountryGroup is one country in the location filter together with every place
// inside it. This grouping is what answers the subject's hint that a city is
// part of the country above it: ticking the country ticks every place under it.
type CountryGroup struct {
	Slug   string        // raw API country segment, for example "usa"
	Name   string        // readable country name, for example "USA"
	Places []PlaceOption // every place in this country, sorted by name
}

// FilterOptions carries everything the sidebar controls need in order to draw
// themselves: the two ends of each range slider, the band sizes that actually
// occur in the data, and the country and place hierarchy.
type FilterOptions struct {
	MinCreationYear int
	MaxCreationYear int
	MinAlbumYear    int
	MaxAlbumYear    int
	MemberCounts    []int
	Countries       []CountryGroup
}

// ArtistCard is one artist as the home page needs it: the artist itself plus
// the values its filters compare against, worked out once here rather than in
// the template or in the browser.
type ArtistCard struct {
	Artist         Artist
	FirstAlbumYear int
	MemberCount    int
	LocationSlugs  string // space separated, for example "brooklyn-usa london-uk"
}

// HomePageData is the single value templates/index.html is executed with.
type HomePageData struct {
	Cards   []ArtistCard
	Filters FilterOptions
}

// firstAlbumYear pulls the year out of an artist's first album date. The API
// writes those dates in the same "dd-mm-yyyy" shape as the concert dates, so
// the parser already written for the artist page is reused rather than a second
// one added. A date that cannot be read yields 0, which the browser treats as
// "unknown" and lets through the range filter instead of hiding the artist.
func firstAlbumYear(raw_first_album string) int {
	parsed_date := parseConcertDate(raw_first_album)
	if parsed_date.IsZero() {
		return 0
	}

	return parsed_date.Year()
}

// splitLocationSlug splits a raw API location into its place and its country.
// Every value the API returns holds exactly one hyphen, with the place on the
// left and the country on the right, for example
// "abu_dhabi-united_arab_emirates". A value with no hyphen at all is treated as
// a place with no country.
func splitLocationSlug(raw_location string) (place_segment, country_segment string) {
	segments := strings.SplitN(raw_location, "-", 2)
	if len(segments) < 2 {
		return raw_location, ""
	}

	return segments[0], segments[1]
}

// buildLocationsByArtistID indexes the locations endpoint by artist ID. The
// lookup is built once so that building the cards stays a single pass, and it
// is keyed by the ID the API reports rather than by position, so a missing
// entry cannot shift every later artist's locations onto the wrong card.
func buildLocationsByArtistID(locations []Location) map[int][]string {
	locations_by_artist_id := make(map[int][]string, len(locations))

	for _, location_entry := range locations {
		locations_by_artist_id[location_entry.ID] = location_entry.Locations
	}

	return locations_by_artist_id
}

// BuildArtistCards turns the raw artists and locations into the cards the home
// page renders, each one carrying the values its filters compare against.
func BuildArtistCards(artists []Artist, locations []Location) []ArtistCard {
	locations_by_artist_id := buildLocationsByArtistID(locations)

	cards := make([]ArtistCard, 0, len(artists))
	for _, artist := range artists {
		artist_locations := locations_by_artist_id[artist.ID]

		cards = append(cards, ArtistCard{
			Artist:         artist,
			FirstAlbumYear: firstAlbumYear(artist.FirstAlbum),
			MemberCount:    len(artist.Members),
			// One space separated string rather than a slice, because the
			// browser reads it straight out of a data attribute.
			LocationSlugs: strings.Join(artist_locations, " "),
		})
	}

	return cards
}

// creationYearBounds returns the earliest and the latest creation year in the
// data, which is where the creation date slider starts and ends.
func creationYearBounds(artists []Artist) (minimum_year, maximum_year int) {
	for _, artist := range artists {
		// A missing creation year would otherwise pull the lower end of the
		// slider down to zero.
		if artist.CreationDate <= 0 {
			continue
		}

		if minimum_year == 0 || artist.CreationDate < minimum_year {
			minimum_year = artist.CreationDate
		}
		if artist.CreationDate > maximum_year {
			maximum_year = artist.CreationDate
		}
	}

	return minimum_year, maximum_year
}

// albumYearBounds returns the earliest and the latest first album year, which
// is where the first album slider starts and ends. Dates that could not be
// parsed come back as 0 and are skipped, for the same reason as above.
func albumYearBounds(artists []Artist) (minimum_year, maximum_year int) {
	for _, artist := range artists {
		album_year := firstAlbumYear(artist.FirstAlbum)
		if album_year == 0 {
			continue
		}

		if minimum_year == 0 || album_year < minimum_year {
			minimum_year = album_year
		}
		if album_year > maximum_year {
			maximum_year = album_year
		}
	}

	return minimum_year, maximum_year
}

// collectMemberCounts returns every distinct band size in the data, smallest
// first, so that the members filter only offers counts that can actually match
// something. An artist with an empty member list contributes no check box,
// since a "0 members" option would never be a useful thing to tick.
func collectMemberCounts(artists []Artist) []int {
	seen_counts := make(map[int]bool)
	for _, artist := range artists {
		member_count := len(artist.Members)
		if member_count == 0 {
			continue
		}

		seen_counts[member_count] = true
	}

	member_counts := make([]int, 0, len(seen_counts))
	for member_count := range seen_counts {
		member_counts = append(member_counts, member_count)
	}
	sort.Ints(member_counts)

	return member_counts
}

// buildPlaceOptions turns one country's set of place segments into the sorted
// check box options shown underneath that country.
func buildPlaceOptions(country_segment string, place_segments map[string]bool) []PlaceOption {
	place_options := make([]PlaceOption, 0, len(place_segments))
	for place_segment := range place_segments {
		place_options = append(place_options, PlaceOption{
			// Putting the two halves back together gives exactly the value the
			// API used, which is what the cards carry in their data attribute.
			Slug: place_segment + "-" + country_segment,
			Name: formatLocationSegment(place_segment),
		})
	}

	sort.Slice(place_options, func(first_index, second_index int) bool {
		return strings.ToLower(place_options[first_index].Name) <
			strings.ToLower(place_options[second_index].Name)
	})

	return place_options
}

// collectCountryGroups turns the flat list of raw locations into the country
// and place hierarchy the location filter renders. Every location appears once
// however many artists played there, and both levels come out sorted so the
// sidebar reads alphabetically.
func collectCountryGroups(locations []Location) []CountryGroup {
	// Places are gathered per country as a set first, because the same location
	// is listed again for every artist that played there.
	places_by_country := make(map[string]map[string]bool)

	for _, location_entry := range locations {
		for _, raw_location := range location_entry.Locations {
			place_segment, country_segment := splitLocationSlug(raw_location)

			// A location with no country cannot be placed in the hierarchy, so
			// it is left out rather than filed under an empty heading.
			if country_segment == "" {
				continue
			}

			if places_by_country[country_segment] == nil {
				places_by_country[country_segment] = make(map[string]bool)
			}
			places_by_country[country_segment][place_segment] = true
		}
	}

	country_groups := make([]CountryGroup, 0, len(places_by_country))
	for country_segment, place_segments := range places_by_country {
		country_groups = append(country_groups, CountryGroup{
			Slug:   country_segment,
			Name:   formatLocationSegment(country_segment),
			Places: buildPlaceOptions(country_segment, place_segments),
		})
	}

	// Compared in lower case so that an upper case abbreviation such as "USA"
	// sorts with the U's instead of ahead of every lower case letter.
	sort.Slice(country_groups, func(first_index, second_index int) bool {
		return strings.ToLower(country_groups[first_index].Name) <
			strings.ToLower(country_groups[second_index].Name)
	})

	return country_groups
}

// BuildFilterOptions works out the choices the sidebar offers: the ends of the
// two year sliders, every band size that occurs in the data, and the countries
// with their places.
func BuildFilterOptions(artists []Artist, locations []Location) FilterOptions {
	minimum_creation_year, maximum_creation_year := creationYearBounds(artists)
	minimum_album_year, maximum_album_year := albumYearBounds(artists)

	return FilterOptions{
		MinCreationYear: minimum_creation_year,
		MaxCreationYear: maximum_creation_year,
		MinAlbumYear:    minimum_album_year,
		MaxAlbumYear:    maximum_album_year,
		MemberCounts:    collectMemberCounts(artists),
		Countries:       collectCountryGroups(locations),
	}
}

// BuildHomePageData assembles everything templates/index.html needs: the cards
// to render, and the filter choices in the sidebar beside them.
func BuildHomePageData(artists []Artist, locations []Location) HomePageData {
	return HomePageData{
		Cards:   BuildArtistCards(artists, locations),
		Filters: BuildFilterOptions(artists, locations),
	}
}
