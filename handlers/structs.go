package handlers

// APIConfig keeps the main URLs of the API
type APIConfig struct {
	ArtistsURL   string
	LocationsURL string
	DatesURL     string
	RelationURL  string
	GeocodeURL   string
}

var APIs = APIConfig{
	ArtistsURL:   "https://groupietrackers.herokuapp.com/api/artists",
	LocationsURL: "https://groupietrackers.herokuapp.com/api/locations",
	DatesURL:     "https://groupietrackers.herokuapp.com/api/dates",
	RelationURL:  "https://groupietrackers.herokuapp.com/api/relation",
	GeocodeURL:   "https://nominatim.openstreetmap.org/search",
}

// Artist keep the information of the artists
type Artist struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Image        string   `json:"image"`
	Members      []string `json:"members"`
	CreationDate int      `json:"creationDate"`
	FirstAlbum   string   `json:"firstAlbum"`
}

var Artists []Artist

// Location keep the information of the locations
// Wrapper struct for the locations API response
type LocationsResponse struct {
	Index []Location `json:"index"`
}
type Location struct {
	ID        int      `json:"id"`
	Locations []string `json:"locations"`
	Dates     string   `json:"dates"`
}

var Locations []Location

// Dates κeep the information of the dates
// Wrapper struct for the dates API response
type DatesResponse struct {
	Index []Dates `json:"index"`
}
type Dates struct {
	ID    int      `json:"id"`
	Dates []string `json:"dates"`
}

var DatesData []Dates

// Relation keep the information of the relation between artists, locations and dates
// Wrapper struct for the relation API response
type RelationResponse struct {
	Index []Relation `json:"index"`
}
type Relation struct {
	ID             int                 `json:"id"`
	DatesLocations map[string][]string `json:"datesLocations"`
}

// ConcertStop is one location on an artist's tour, flattened out of the
// DatesLocations map so it can carry a chronological order and its
// coordinates. It is handed to both the template and, as JSON, to the map
// script in the browser.
type ConcertStop struct {
	RawLocation       string   `json:"-"`
	FormattedLocation string   `json:"location"`
	Dates             []string `json:"dates"`
	Latitude          float64  `json:"latitude"`
	Longitude         float64  `json:"longitude"`
	HasCoordinates    bool     `json:"hasCoordinates"`
	Order             int      `json:"order"`
}

var Relations []Relation

type ArtistPageData struct {
	A     Artist
	R     Relation
	Stops []ConcertStop
}

// FetchAll fetches artists, locations, dates and relations from the API
// and stores them in the corresponding package-level variables.
//
// Note: the /artists endpoint returns a plain JSON array, so it can use
// the generic Fetch[T] helper directly. The /locations, /dates and
// /relation endpoints wrap their array inside an "index" field, so those
// are decoded into their respective *Response wrapper structs first, and
// then the .Index slice is extracted.
func FetchAll() error {
	var err error

	if Artists, err = Fetch[Artist](APIs.ArtistsURL); err != nil {
		return err
	}

	var locResp LocationsResponse
	if err = DataHandler(APIs.LocationsURL, &locResp); err != nil {
		return err
	}
	Locations = locResp.Index

	var datesResp DatesResponse
	if err = DataHandler(APIs.DatesURL, &datesResp); err != nil {
		return err
	}
	DatesData = datesResp.Index

	var relResp RelationResponse
	if err = DataHandler(APIs.RelationURL, &relResp); err != nil {
		return err
	}
	Relations = relResp.Index

	return nil
}
