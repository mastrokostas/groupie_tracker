package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// abbreviation_max_length is the longest a location segment can be while
// still being treated as an abbreviation such as "usa", "uk" or "uae".
// The shortest real country names ("chad", "cuba", "iran", "peru", ...) are
// all four letters, so nothing that is a real name is caught by this.
const abbreviation_max_length = 3

// capitalizeWord returns the word with its first letter in upper case.
func capitalizeWord(word string) string {
	if word == "" {
		return ""
	}

	// Working on runes rather than bytes keeps non ASCII first letters intact
	letters := []rune(word)
	letters[0] = []rune(strings.ToUpper(string(letters[0])))[0]

	return string(letters)
}

// formatLocationSegment formats one side of the hyphen, so either the city
// or the country. "new_york" becomes "New York" and "usa" becomes "USA".
func formatLocationSegment(raw_segment string) string {
	// A short segment with no underscore is an abbreviation, so the whole
	// thing goes to upper case. The underscore check is what stops a city
	// such as "new_york" from being read as an abbreviation, since "new" is
	// itself only three letters long.
	if !strings.Contains(raw_segment, "_") && len(raw_segment) <= abbreviation_max_length {
		return strings.ToUpper(raw_segment)
	}

	// Otherwise every underscore separated word is capitalised in place
	words := strings.Split(raw_segment, "_")
	for word_index, word := range words {
		words[word_index] = capitalizeWord(word)
	}

	return strings.Join(words, " ")
}

// FormatLocation turns a raw API location into a readable one, for example
// "new_york-usa" into "New York - USA". The API joins the city and the
// country with a hyphen, so each side is formatted separately and the two
// are put back together with spaces around the hyphen.
func FormatLocation(raw_location string) string {
	segments := strings.Split(raw_location, "-")
	for segment_index, segment := range segments {
		segments[segment_index] = formatLocationSegment(segment)
	}

	return strings.Join(segments, " - ")
}

// template_functions holds the transformations the templates can call on the
// values handed to them.
var template_functions = template.FuncMap{
	"formatLocation": FormatLocation,
	"mapDataJSON":    mapDataJSON,
}

// parseConcertDate turns a single API date string into a time.Time value.
func parseConcertDate(raw_date string) time.Time {
	parsed_date, _ := time.Parse("02-01-2006", raw_date)

	return parsed_date
}

// sortDatesLocations sorts, in place, the date slice of every location so the
// template renders each location's concert dates in chronological order
// (oldest first).
func sortDatesLocations(relation Relation) {
	for _, dates_of_location := range relation.DatesLocations {
		sort.Slice(dates_of_location, func(first_index, second_index int) bool {
			first_date := parseConcertDate(dates_of_location[first_index])
			second_date := parseConcertDate(dates_of_location[second_index])

			return first_date.Before(second_date)
		})
	}
}

func ArtistHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		renderError(w, http.StatusBadRequest, "Invalid Artist ID")
		return
	}
	if id < 1 || id > len(Artists) {
		renderError(w, http.StatusNotFound, "Artist not found")
		return
	}
	pageData := ArtistPageData{
		A: Artists[id-1],
		R: Relations[id-1],
	}

	// Put every location's dates in chronological order before rendering
	sortDatesLocations(pageData.R)
	pageData.Stops = buildConcertStops(pageData.R)

	// The function map has to be attached before the file is parsed, and the
	// template has to be named after the file so that Execute finds it.
	tmpl, err := template.New("artist.html").Funcs(template_functions).ParseFiles("templates/artist.html")
	if err != nil {
		renderError(w, http.StatusNotFound, "Template not found")
		return
	}
	if err := tmpl.Execute(w, pageData); err != nil {
		renderError(w, http.StatusInternalServerError, "Internal Server Error")
	}
}

// earliestConcertDate returns the earliest date in a location's date slice.
// sortDatesLocations has already put them in order, so the first element is
// the earliest, but the slice is checked in case a location has no dates.
func earliestConcertDate(dates_of_location []string) time.Time {
	earliest_date := time.Time{}

	for _, raw_date := range dates_of_location {
		parsed_date := parseConcertDate(raw_date)
		if earliest_date.IsZero() || parsed_date.Before(earliest_date) {
			earliest_date = parsed_date
		}
	}

	return earliest_date
}

// buildConcertStops flattens an artist's DatesLocations map into a slice
// ordered by when each location was first played, numbered from one, with
// the cached coordinates attached.
func buildConcertStops(relation Relation) []ConcertStop {
	concert_stops := make([]ConcertStop, 0, len(relation.DatesLocations))

	for raw_location, dates_of_location := range relation.DatesLocations {
		coordinates, has_coordinates := LookupCoordinates(raw_location)

		concert_stops = append(concert_stops, ConcertStop{
			RawLocation:       raw_location,
			FormattedLocation: FormatLocation(raw_location),
			Dates:             dates_of_location,
			Latitude:          coordinates.Latitude,
			Longitude:         coordinates.Longitude,
			HasCoordinates:    has_coordinates,
		})
	}

	sort.Slice(concert_stops, func(first_index, second_index int) bool {
		first_date := earliestConcertDate(concert_stops[first_index].Dates)
		second_date := earliestConcertDate(concert_stops[second_index].Dates)

		return first_date.Before(second_date)
	})

	for stop_index := range concert_stops {
		concert_stops[stop_index].Order = stop_index + 1
	}

	return concert_stops
}

// mapDataJSON marshals the concert stops for the browser. The result is
// returned as template.JS so html/template inserts it verbatim instead of
// escaping the quotes, which is safe because json.Marshal already escapes
// the characters that could close the surrounding script tag.
func mapDataJSON(concert_stops []ConcertStop) (template.JS, error) {
	encoded_stops, err := json.Marshal(concert_stops)
	if err != nil {
		return template.JS("[]"), err
	}

	return template.JS(encoded_stops), nil
}
