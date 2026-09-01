# Groupie Tracker — Geolocalization

## Context

The project's next stage requires mapping every concert location of an artist. Today `templates/artist.html` renders `.R.DatesLocations` as a plain nested list, and because Go's `html/template` sorts map keys, locations appear **alphabetically by raw key** (`aalborg-denmark` first), not by when the concerts happened.

This change converts each raw address (`north_carolina-usa`) into geographic coordinates (`35.7596, -79.0193`) and plots them on an interactive map on the artist page, with markers numbered in the order the artist actually toured and a line tracing the route between them.

**Decisions made:**

| Decision | Choice |
|---|---|
| Geocoding | Nominatim (OpenStreetMap) — no API key, no billing |
| Map rendering | Leaflet — no API key |
| Coordinate storage | `data/coordinates.json`, committed to the repo |
| Geocode timing | At startup, only for locations missing from the file |
| Map display | Numbered markers in date order + connecting route line |

Both the Go rule and the brief are satisfied: geocoding is an HTTP GET + JSON decode using only `net/http`, `encoding/json` and `strconv` — the same shape as the existing `DataHandler`. Leaflet is browser-side JavaScript and never enters `go.mod`, which stays dependency-free. The brief explicitly permits choosing a Map API.

**Why the file cache:** there are 189 unique locations across all artists. Nominatim's usage policy allows 1 request/second, so a cold geocode takes ~3.2 minutes. Committing `data/coordinates.json` means that cost is paid exactly once, ever — every subsequent run, including a fresh clone by whoever audits the project, starts instantly with zero network calls. It also lands the brief's "JSON files and format" and "Manipulation and storage of data" goals.

---

## 1. New file: `handlers/geocode.go`

The whole geocoding layer. Follows the existing `data.go` style: small functions, explicit error returns.

**Types and package state**

```go
// Coordinates holds one geographic point on the map.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// nominatim_result mirrors one element of the Nominatim search response.
// Nominatim returns the coordinates as strings rather than numbers, so
// they have to be parsed after decoding.
type nominatim_result struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}
```

Package-level state, mirroring how `Artists`/`Relations` already work:

- `coordinates_cache map[string]Coordinates` — raw location key to point.
- `coordinates_cache_mutex sync.RWMutex` — guards the cache, since handlers read it concurrently.
- `coordinates_file_path = "data/coordinates.json"` — a **`var`**, not a const, so tests can redirect it into `t.TempDir()`.
- `nominatim_request_interval = time.Second` — also a `var`, so tests set it to zero and don't sleep.
- `nominatim_user_agent` — a descriptive identifier with a contact address. Nominatim **rejects requests without one**, so this is required, not optional.

Add `GeocodeURL: "https://nominatim.openstreetmap.org/search"` to the existing `APIConfig` struct and `APIs` var in `handlers/structs.go`, matching how the other four endpoints are configured — this is what lets tests repoint geocoding at an `httptest` server.

**Functions**

- `buildGeocodeQuery(raw_location string) string` — turns the API's key format into a human address. Underscores become spaces, the hyphen becomes a comma: `north_carolina-usa` → `north carolina, usa`. Nominatim resolves this form reliably.

- `geocodeLocation(raw_location string) (Coordinates, error)` — builds the request URL with `net/url`'s `url.Values` (`q`, `format=json`, `limit=1`), constructs an `http.NewRequest` so the `User-Agent` header can be set, sends it through the existing `httpClient` in `data.go` (reusing its 10s timeout rather than making a second client), and returns an error for: non-200 status, undecodable JSON, an empty result array, or a lat/lon that `strconv.ParseFloat` rejects.

- `loadCoordinatesFile() error` — reads `data/coordinates.json` into the cache. A missing file is **not an error** (`os.IsNotExist` → return nil), so the very first run works.

- `saveCoordinatesFile() error` — `os.MkdirAll` on the directory, then `json.MarshalIndent` and write. `encoding/json` sorts map keys automatically, so the committed file has stable, review-friendly diffs.

- `collectUniqueLocations() []string` — walks every `Relation.DatesLocations` key into a deduplicated, sorted slice.

- `GeocodeAllLocations() error` — the entry point called from `main`. Loads the file, then for each location **not already cached**, geocodes it, stores it, and sleeps `nominatim_request_interval` before the next one. A failure on one location is **logged and skipped, never fatal** — that location simply gets no marker. Saves the file at the end only if anything new was added.

- `LookupCoordinates(raw_location string) (Coordinates, bool)` — read-locked cache lookup for the handler.

## 2. `handlers/structs.go` — new page data

```go
// ConcertStop is one location on an artist's tour, ready for the template
// and for the map.
type ConcertStop struct {
	RawLocation       string      `json:"-"`
	FormattedLocation string      `json:"location"`
	Dates             []string    `json:"dates"`
	Latitude          float64     `json:"latitude"`
	Longitude         float64     `json:"longitude"`
	HasCoordinates    bool        `json:"hasCoordinates"`
	Order             int         `json:"order"`
}
```

`ArtistPageData` gains a `Stops []ConcertStop` field. The existing `A` and `R` fields stay untouched so the current template markup and its tests keep working.

## 3. `handlers/artist.go` — ordering and template plumbing

- `buildConcertStops(relation Relation) []ConcertStop` — the core of "chronological order". It reuses the existing `parseConcertDate` helper to find each location's **earliest** date, sorts the stops by that date with `sort.Slice`, assigns `Order` starting at 1, fills in coordinates via `LookupCoordinates`, and formats the display name with the existing `FormatLocation`.

- `mapDataJSON(stops []ConcertStop) (template.JS, error)` — marshals the stops for the browser. `json.Marshal` escapes `<`, `>` and `&` by default, so the payload cannot break out of the `<script>` block. Registered in the existing `template_functions` map alongside `formatLocation`.

- `ArtistHandler` — one added line building `pageData.Stops` after the existing `sortDatesLocations` call. Everything else, including the error paths, is unchanged.

**Note, not fixed here:** `sortDatesLocations` already mutates the shared global `Relations` in place on every request, which is a pre-existing data race. `buildConcertStops` only reads, so it adds no new races. Fixing the existing one is outside this task's scope — say the word if you want it included.

## 4. `main.go` — one call

After the existing `FetchAll()` block:

```go
// Turn every concert address into coordinates for the artist page map.
// Cached results are loaded from disk, so this is normally instant.
if err := handlers.GeocodeAllLocations(); err != nil {
	log.Printf("Failed to geocode locations: %v", err)
}
```

Non-fatal, matching the `log.Printf` treatment `FetchAll` already gets (commit `19b3e83`) — a geocoding problem must not stop the site from serving.

## 5. `templates/artist.html`

- Leaflet's CSS in `<head>` and its JS before `</body>`, from unpkg — the same CDN pattern the page already uses for the Figtree font.
- A `<div id="concert-map">` inserted **after** the locations list and before the back button.
- The stop data as `<script id="concert-map-data" type="application/json">{{ mapDataJSON .Stops }}</script>`.
- `<script src="/static/js/map.js"></script>`, served by the existing static file server in `main.go`.

**Critical constraint:** `style.css` targets `.artist-card > ul:nth-of-type(1)` and `:nth-of-type(2)` positionally. The map container must be a `<div>`, never a `<ul>`, or the existing member/location styling silently breaks.

## 6. New file: `static/js/map.js`

Reads and parses `#concert-map-data`, then:

- Keeps only stops where `hasCoordinates` is true.
- If none remain, replaces the map container with a short "map unavailable" message rather than showing an empty grey box — this is the website-error handling the brief asks for, on the client side.
- Places an `L.divIcon` marker per stop labelled with its `order`, styled by a CSS class so the numbers use the existing green accent.
- Binds a popup per marker showing the formatted location and its dates.
- Draws an `L.polyline` through the stops in order, tracing the tour route.
- Calls `fitBounds` so every marker is visible without manual zooming.

## 7. `static/css/style.css`

Appends only — no existing rules change. A sized, rounded `#concert-map` matching the card styling, and the numbered marker class, both driven by the existing `--color_accent` / `--color_surface` custom properties so the map fits the current dark theme.

## 8. Tests

New `handlers/geocode_test.go`, following the established patterns exactly: table-driven subtests, `httptest.NewServer` fakes, and `t.Cleanup` save/restore of every global touched.

- `buildGeocodeQuery` across underscore, hyphen and abbreviation cases.
- `geocodeLocation`: success, empty result array, non-200 status, invalid JSON, unparseable lat/lon, and an assertion that the `User-Agent` header actually arrives at the server.
- `loadCoordinatesFile` / `saveCoordinatesFile` round-trip in a `t.TempDir()`, plus the missing-file case returning nil.
- `collectUniqueLocations` deduplicating and sorting.
- `GeocodeAllLocations` skipping already-cached locations — verified with a request counter on the mock server.

Additions to `handlers/artist_test.go`:

- `buildConcertStops` ordering stops by earliest date (not alphabetically) and numbering them from 1.
- `buildConcertStops` marking `HasCoordinates` false for an ungeocoded location.
- `mapDataJSON` producing the expected shape.

---

## Verification

```bash
cd /home/kostas/Desktop/Coding/zone01/groupie-tracker
gofmt -l .            # expect no output
go vet ./...
go test ./... -v
```

**First run** (populates the cache — takes ~3.2 minutes, once):

```bash
go run .
```

Watch the log for geocoding progress, then confirm the file exists and looks right:

```bash
git status                                  # data/coordinates.json is new
head -20 data/coordinates.json
grep -c latitude data/coordinates.json      # expect close to 189
```

**Second run** must start instantly — this proves the cache works.

**In the browser** at `http://localhost:8080`:

1. Open any artist. The map renders below the locations list with numbered markers.
2. Marker `1` is the **earliest** concert, not the alphabetically first location — cross-check against the dates in the list above the map.
3. The route line connects markers in ascending order.
4. Clicking a marker shows the formatted location (`North Carolina - USA`) and its dates.
5. All markers fit in view on load.

**Error handling:**

- Delete `data/coordinates.json`, disconnect the network, and run. The server must still start and artist pages must still render the text list, with the map area showing its unavailable message instead of a broken box.
- `http://localhost:8080/artist?id=abc` → 400; `?id=9999` → 404; an unknown path → 404. All unchanged from today.

Finally, commit `data/coordinates.json` so the cache ships with the repo.
