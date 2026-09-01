# Groupie Tracker — Geolocalization

Complete record of the implemented feature: what was added, why, and how it behaves.

---

## 1. The problem

`templates/artist.html` renders `.R.DatesLocations`, which is a `map[string][]string`. Go's template engine iterates map keys in sorted order, so concert locations appeared **alphabetically by raw key** — `aalborg-denmark` first — not in the order the artist toured.

The tour order lives in the values (the date slices), not the keys, so map iteration throws that information away.

## 2. The solution

Three layers that barely touch each other:

| Layer | Job | Where |
|---|---|---|
| Geocoding | `north_carolina-usa` → `35.7596, -79.0193` | Go, once at startup |
| Ordering | which location was played first | Go, per request |
| Rendering | draw the pins and the route | browser JavaScript |

**Decisions:**

| Decision | Choice |
|---|---|
| Geocoding | Nominatim (OpenStreetMap) — no API key, no account, no billing |
| Map rendering | Leaflet — no API key |
| Coordinate storage | `data/coordinates.json`, committed to the repo |
| Geocode timing | at startup, only for locations missing from the file |
| Map display | numbered markers in date order + connecting route line |

The Go constraint is satisfied: geocoding is an HTTP GET plus a JSON decode using only `net/http`, `net/url`, `encoding/json` and `strconv` — the same shape as the existing `DataHandler`. Leaflet is browser-side and never enters `go.mod`, which stays dependency-free.

**Why the file cache.** There are 189 unique locations across all artists. Nominatim's usage policy allows a maximum of one request per second, so a cold geocode takes roughly 3.2 minutes. Committing `data/coordinates.json` means that cost is paid exactly once, ever. Every subsequent run, including a fresh clone, starts instantly with zero network calls. The same policy also requires that results be cached, so this is compliance as well as speed.

---

## 3. `handlers/structs.go`

### `GeocodeURL`

```go
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
```

The endpoint lives here rather than as a constant in `geocode.go` because `APIs` is a package-level `var`. A test reassigns `APIs.GeocodeURL` to an `httptest` server URL and every geocode call hits the fake instead of the real Nominatim. `structs_test.go` already uses this technique on the other four URLs.

### `ConcertStop`

```go
type ConcertStop struct {
	RawLocation       string   `json:"-"`
	FormattedLocation string   `json:"location"`
	Dates             []string `json:"dates"`
	Latitude          float64  `json:"latitude"`
	Longitude         float64  `json:"longitude"`
	HasCoordinates    bool     `json:"hasCoordinates"`
	Order             int      `json:"order"`
}

type ArtistPageData struct {
	A     Artist
	R     Relation
	Stops []ConcertStop
}
```

`ConcertStop` is `DatesLocations` flattened into a slice, so it can carry an order. A map cannot.

- `RawLocation` — the original key, `north_carolina-usa`. Used server-side for the cache lookup. `json:"-"` keeps it out of the JSON sent to the browser, which has no use for it.
- `FormattedLocation` — `North Carolina - USA`, from the existing `FormatLocation`.
- `HasCoordinates` — needed because a zero `Coordinates` is `0, 0`, a real point in the Gulf of Guinea. Zero values cannot be used to mean "missing".
- `Order` — 1, 2, 3. The number printed inside the marker.

`A` and `R` are untouched, so the existing template markup and its tests keep working. The map is purely additive.

---

## 4. `handlers/geocode.go`

New file. The entire geocoding layer.

### Types

```go
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type nominatim_result struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}
```

`Coordinates` is the internal type: floats, because you sort and compare with them. Its json tags name the keys in `data/coordinates.json`.

`nominatim_result` mirrors one element of Nominatim's response. Its fields are `string`, not `float64`, because Nominatim sends the numbers quoted:

```json
[{"lat":"35.7595731","lon":"-79.0192997"}]
```

Declaring them as `float64` makes the decode fail on every response.

### Package state

```go
var coordinates_cache = map[string]Coordinates{}
var coordinates_cache_mutex sync.RWMutex
var coordinates_file_path = "data/coordinates.json"
var nominatim_request_interval = time.Second
var nominatim_user_agent = "groupie-tracker/1.0 (https://github.com/mastrokostas)"
```

- `coordinates_cache` — the lookup table, and the exact thing marshalled to disk.
- `coordinates_cache_mutex` — `net/http` runs every request in its own goroutine, so `LookupCoordinates` is called concurrently. `RWMutex` rather than `Mutex` because the pattern is write-once at startup then read-forever: many readers pass through at the same time.
- `coordinates_file_path` — a `var`, not a `const`, so a test can redirect it into `t.TempDir()`.
- `nominatim_request_interval` — the sleep between requests. Also a `var`, so tests set it to zero and don't sleep.
- `nominatim_user_agent` — sent as a header. Not a credential and no account behind it; Nominatim rejects requests carrying no User-Agent or the default one set by an HTTP library.

### Query overrides

```go
// geocode_query_overrides maps raw API locations whose country no longer
// exists onto a name Nominatim can still resolve. The Netherlands Antilles
// was dissolved in 2010 and Willemstad is now the capital of Curacao.
var geocode_query_overrides = map[string]string{
	"willemstad-netherlands_antilles": "willemstad, curacao",
}
```

Nominatim geocodes current administrative entities. The Groupie Tracker API's dataset contains at least one country that no longer exists, so the generated query matches nothing and returns an empty array. Without an override that location is retried on every single startup, forever, because a failure is never written to the cache.

### `buildGeocodeQuery`

```go
func buildGeocodeQuery(raw_location string) string {
	if override_query, has_override := geocode_query_overrides[raw_location]; has_override {
		return override_query
	}

	with_spaces := strings.ReplaceAll(raw_location, "_", " ")

	return strings.ReplaceAll(with_spaces, "-", ", ")
}
```

Pure text rewriting. Underscores become spaces, the hyphen becomes a comma:

```
north_carolina-usa  →  north carolina, usa
```

The comma matters. A geocoder reads it as "what follows contains what precedes" — city, then country. Sending `north carolina-usa` as one unbroken name returns nothing.

This is a different transformation from `FormatLocation`, which produces `North Carolina - USA` for display. Same input, two outputs, two audiences.

### `geocodeLocation`

```go
func geocodeLocation(raw_location string) (Coordinates, error) {
	query_parameters := url.Values{}
	query_parameters.Set("q", buildGeocodeQuery(raw_location))
	query_parameters.Set("format", "json")
	query_parameters.Set("limit", "1")

	request_url := APIs.GeocodeURL + "?" + query_parameters.Encode()

	request, err := http.NewRequest(http.MethodGet, request_url, nil)
	if err != nil {
		return Coordinates{}, err
	}

	request.Header.Set("User-Agent", nominatim_user_agent)

	response, err := httpClient.Do(request)
	if err != nil {
		return Coordinates{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return Coordinates{}, fmt.Errorf("unexpected status %d geocoding %s", response.StatusCode, raw_location)
	}

	var results []nominatim_result
	if err := json.NewDecoder(response.Body).Decode(&results); err != nil {
		return Coordinates{}, err
	}

	if len(results) == 0 {
		return Coordinates{}, fmt.Errorf("no result for %s", raw_location)
	}

	latitude, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return Coordinates{}, fmt.Errorf("bad latitude %q for %s", results[0].Lat, raw_location)
	}

	longitude, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return Coordinates{}, fmt.Errorf("bad longitude %q for %s", results[0].Lon, raw_location)
	}

	return Coordinates{Latitude: latitude, Longitude: longitude}, nil
}
```

The only function in the package that touches Nominatim.

**`url.Values`** builds and escapes the query string. The space becomes `+`, the comma becomes `%2C`. Hand-concatenating would send a malformed URL.

**`limit=1`** because only the best match is wanted.

**`http.NewRequest` instead of `httpClient.Get`.** `data.go` uses `Get`, which gives nowhere to attach a header. `NewRequest` builds the request without sending it, the User-Agent is set, then `httpClient.Do` sends it — the same client from `data.go`, inheriting its 10-second timeout rather than declaring a second one.

**Four failure paths.** The empty-array case is the one that actually fires in production: Nominatim replies `200 OK` with `[]` when it does not recognise a place. Without that check, `results[0]` panics on an empty slice and takes the server down at startup.

### `loadCoordinatesFile` / `saveCoordinatesFile`

```go
func loadCoordinatesFile() error {
	file_contents, err := os.ReadFile(coordinates_file_path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	loaded_coordinates := map[string]Coordinates{}
	if err := json.Unmarshal(file_contents, &loaded_coordinates); err != nil {
		return err
	}

	coordinates_cache_mutex.Lock()
	defer coordinates_cache_mutex.Unlock()
	coordinates_cache = loaded_coordinates

	return nil
}

func saveCoordinatesFile() error {
	if err := os.MkdirAll(filepath.Dir(coordinates_file_path), 0o755); err != nil {
		return err
	}

	coordinates_cache_mutex.RLock()
	file_contents, err := json.MarshalIndent(coordinates_cache, "", "  ")
	coordinates_cache_mutex.RUnlock()

	if err != nil {
		return err
	}

	return os.WriteFile(coordinates_file_path, file_contents, 0o644)
}
```

**A missing file is not an error.** Three situations produce one: the first run ever, a clone before the file is committed, and deleting it deliberately to test. All three should leave an empty cache and a program that keeps going. Any other read error is returned normally.

The unmarshal targets a fresh local map, then replaces the cache under the write lock. Decoding straight into the shared map would leave it half-populated if the JSON turned out to be broken partway through.

`os.MkdirAll` is required because the repo has no `data/` directory. It returns nil when the directory already exists.

`MarshalIndent` because the file is committed and read by humans. `encoding/json` writes map keys sorted alphabetically, always, so the same set of locations produces byte-identical output and a git diff shows only genuinely new entries.

What lands on disk:

```json
{
  "aalborg-denmark": {
    "latitude": 57.0488195,
    "longitude": 9.921747
  },
  "north_carolina-usa": {
    "latitude": 35.6729639,
    "longitude": -79.0392919
  }
}
```

### `collectUniqueLocations`

```go
func collectUniqueLocations() []string {
	seen_locations := map[string]bool{}

	for _, relation := range Relations {
		for raw_location := range relation.DatesLocations {
			seen_locations[raw_location] = true
		}
	}

	unique_locations := make([]string, 0, len(seen_locations))
	for raw_location := range seen_locations {
		unique_locations = append(unique_locations, raw_location)
	}

	sort.Strings(unique_locations)

	return unique_locations
}
```

Across the whole dataset there are roughly a thousand key occurrences but only 189 distinct strings — a dozen artists may all have played the same city. Geocoding each one repeatedly would waste a dozen seconds and send identical queries, which the usage policy flags as faulty behaviour.

`map[string]bool` is used as a set, because Go has no set type. The `bool` is never read. The alternative, scanning a slice before every append, would be 1000 scans of up to 189 items.

`make([]string, 0, len(seen_locations))` — length zero, capacity 189. Appending 189 times never reallocates.

Sorting matters because Go deliberately randomises map iteration order. Sorted output means the startup log reads the same way every run and a test can assert on an exact expected slice.

### `GeocodeAllLocations`

```go
func GeocodeAllLocations() error {
	if err := loadCoordinatesFile(); err != nil {
		return err
	}

	unique_locations := collectUniqueLocations()
	newly_geocoded_count := 0

	for _, raw_location := range unique_locations {
		if _, already_cached := LookupCoordinates(raw_location); already_cached {
			continue
		}

		coordinates, err := geocodeLocation(raw_location)
		if err != nil {
			log.Printf("Failed to geocode %s: %v", raw_location, err)
			time.Sleep(nominatim_request_interval)

			continue
		}

		coordinates_cache_mutex.Lock()
		coordinates_cache[raw_location] = coordinates
		coordinates_cache_mutex.Unlock()

		newly_geocoded_count++

		time.Sleep(nominatim_request_interval)
	}

	if newly_geocoded_count == 0 {
		return nil
	}

	log.Printf("Geocoded %d new locations", newly_geocoded_count)

	return saveCoordinatesFile()
}
```

**The `continue` on the cached check is the entire payoff.** First run: 189 misses, 189 requests, ~3.2 minutes. Second run: 189 hits, zero requests, zero sleeps, instant startup.

**The sleep is also in the error branch.** A failed request still reached their server. The rate limit counts requests, not successes. Skipping the sleep on failures would fire far faster than the policy allows and get the IP blocked.

**Failures are never fatal.** One unresolvable location logs and continues. That location gets no entry, `HasCoordinates` ends up false, and `map.js` filters it out.

**`newly_geocoded_count`** prevents rewriting an unchanged file on every boot.

### `LookupCoordinates`

```go
func LookupCoordinates(raw_location string) (Coordinates, bool) {
	coordinates_cache_mutex.RLock()
	defer coordinates_cache_mutex.RUnlock()

	coordinates, found := coordinates_cache[raw_location]

	return coordinates, found
}
```

Called in two places: the already-cached check in `GeocodeAllLocations`, and `buildConcertStops` on every artist page request. The read lock is what makes the second one safe under concurrent requests.

---

## 5. `handlers/artist.go`

### `earliestConcertDate` and `buildConcertStops`

```go
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
```

Three phases:

1. **Build** — range over the map, one `ConcertStop` per location. Order is meaningless here, map iteration is randomised. `RawLocation` is assigned by hand and used immediately for the cache lookup.
2. **Sort** — `sort.Slice` by earliest date. This is the step that replaces alphabetical with chronological.
3. **Number** — `Order` is position + 1, assigned after the sort. Before it, the numbering would be random.

`earliestConcertDate` is a separate helper rather than `dates_of_location[0]` because indexing element zero panics on an empty slice, and it keeps the sort function readable.

The lookup happens per request rather than once at startup because stops are built per request. It is a locked map read — microseconds, no network. The expensive part already happened at boot.

### `mapDataJSON` and the FuncMap

```go
func mapDataJSON(concert_stops []ConcertStop) (template.JS, error) {
	encoded_stops, err := json.Marshal(concert_stops)
	if err != nil {
		return template.JS("[]"), err
	}

	return template.JS(encoded_stops), nil
}
```

```go
var template_functions = template.FuncMap{
	"formatLocation": FormatLocation,
	"mapDataJSON":    mapDataJSON,
}
```

Output:

```json
[{"location":"Zurich - Switzerland","dates":["03-02-2019"],"latitude":47.3769,"longitude":8.5417,"hasCoordinates":true,"order":1}]
```

**`template.JS`, not `string`.** `html/template` escapes everything by default; a plain string would render as `&#34;location&#34;`, unparseable. `template.JS` marks the value as verified-safe and inserts it as-is.

That is safe because `json.Marshal` escapes `<`, `>` and `&` into `\u003c`, `\u003e` and `\u0026` by default. An artist name containing `</script>` comes out as `\u003c/script\u003e` and cannot break out of the script tag.

**The error return.** A FuncMap function may return `(value, error)`; a non-nil error aborts template execution, which `ArtistHandler` already converts to a 500. The `"[]"` fallback means even a partial render leaves the browser parsing an empty array.

### `ArtistHandler`

One line added after the existing sort call:

```go
	sortDatesLocations(pageData.R)
	pageData.Stops = buildConcertStops(pageData.R)
```

Order matters: `earliestConcertDate` reads the date slices, and they need to be sorted first. Everything else in the handler, including all error paths, is unchanged.

**Known, not fixed here:** `sortDatesLocations` mutates the shared global `Relations` in place on every request, a pre-existing data race. `buildConcertStops` only reads, so it adds no new races.

---

## 6. `main.go`

```go
	// load data used by handlers
	if err := handlers.FetchAll(); err != nil {
		log.Printf("Failed to fetch data: %v", err)
	}

	// Turn every concert address into coordinates for the artist page map.
	// Cached results are loaded from disk, so this is normally instant.
	if err := handlers.GeocodeAllLocations(); err != nil {
		log.Printf("Failed to geocode locations: %v", err)
	}
```

After `FetchAll`, because `collectUniqueLocations` reads `Relations` and `FetchAll` is what fills it. Run it first and it finds an empty slice and geocodes nothing.

`log.Printf`, not `log.Fatal` — the same treatment `FetchAll` already gets. A geocoding failure must not stop the server.

---

## 7. `templates/artist.html`

Four lines added, nothing removed.

In `<head>`:

```html
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css">
```

After the locations list, before the back button:

```html
<div id="concert-map"></div>
<script id="concert-map-data" type="application/json">{{ mapDataJSON .Stops }}</script>
```

Before `</body>`:

```html
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
<script src="/static/js/map.js"></script>
```

**The map container must be a `<div>`, never a `<ul>`.** `style.css` targets `.artist-card > ul:nth-of-type(1)` for the members list and `:nth-of-type(2)` for the locations list. Those selectors count `<ul>` children by position. A third `<ul>` in that card shifts the counting and breaks the location styling silently, with no error.

**`type="application/json"`** means the browser parses the tag but does not execute its contents. `map.js` reads them with `textContent` and `JSON.parse`. Data, not code.

**Load order:** Leaflet before `map.js`, which uses the global `L` that Leaflet defines.

---

## 8. `static/js/map.js`

```js
const map_container = document.getElementById("concert-map");
const map_data_element = document.getElementById("concert-map-data");

let concert_stops = [];
try {
    concert_stops = JSON.parse(map_data_element.textContent).filter(
        (concert_stop) => concert_stop.hasCoordinates
    );
} catch (parse_error) {
    concert_stops = [];
}

if (concert_stops.length === 0) {
    map_container.textContent = "Map unavailable for this artist.";
    map_container.classList.add("map-unavailable");
} else {
    const concert_map = L.map("concert-map");

    L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
        attribution: "&copy; OpenStreetMap contributors",
        maxZoom: 19,
    }).addTo(concert_map);

    const route_points = [];

    concert_stops.forEach((concert_stop) => {
        const stop_position = [concert_stop.latitude, concert_stop.longitude];
        route_points.push(stop_position);

        const numbered_icon = L.divIcon({
            className: "concert-marker",
            html: String(concert_stop.order),
            iconSize: [28, 28],
            iconAnchor: [14, 14],
        });

        L.marker(stop_position, { icon: numbered_icon })
            .addTo(concert_map)
            .bindPopup(
                "<strong>" + concert_stop.location + "</strong><br>" +
                concert_stop.dates.join("<br>")
            );
    });

    L.polyline(route_points, { color: "#1db954", weight: 2 }).addTo(concert_map);

    concert_map.fitBounds(route_points, { padding: [30, 30] });
}
```

**The `hasCoordinates` filter** is the only way to distinguish "off the coast of Africa" from "we don't know". An ungeocoded stop still carries `latitude: 0, longitude: 0` in the JSON.

**The `try/catch`** covers a missing or malformed data tag. Without it a parse error kills the script and leaves a broken page instead of a message.

**The empty case** is the client-side error handling: a message, not an empty grey box.

**Attribution is required.** OpenStreetMap's data licence obliges crediting contributors wherever their data is displayed.

**`L.divIcon`** is a marker made of HTML rather than an image, so the order number goes straight into it. `iconAnchor: [14, 14]` is half of `iconSize`, centring the box on the actual coordinate instead of hanging it below and to the right.

**`route_points`** accumulates in loop order, which is the sorted order from `buildConcertStops`, so the polyline runs 1 → 2 → 3.

**`fitBounds`** supplies both centre and zoom, which is why `L.map()` is created without either. Every marker is visible on load.

---

## 9. `static/css/style.css`

Appended only.

```css
#concert-map {
    height: 380px;
    margin: 1rem 0;
    border: 1px solid var(--color_hairline);
    border-radius: 8px;
    background-color: var(--color_surface);
}

#concert-map.map-unavailable {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 120px;
    color: var(--color_text_muted);
}

.concert-marker {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: 2px solid var(--color_surface);
    border-radius: 50%;
    background-color: var(--color_accent);
    color: var(--color_text);
    font-weight: 700;
    font-size: 0.85rem;
}
```

**`height: 380px` is mandatory.** Leaflet measures its container to decide what to render. A div with no height computes to zero and the map is invisible — no error, just nothing.

**`.concert-marker`** matches `className` in the `divIcon`. Leaflet supplies no styling for a divIcon at all; without these rules the order number renders as bare text on the map. The 28×28 here must match `iconSize` in `map.js`, or the circle and the anchor point disagree.

All colours come from the custom properties already defined at the top of the file, so the map matches the dark theme.

---

## 10. Tests

New `handlers/geocode_test.go`, following the established patterns: table-driven subtests, `httptest.NewServer` fakes, and `t.Cleanup` save/restore of every global touched.

- `buildGeocodeQuery` across underscore, hyphen and abbreviation cases.
- `geocodeLocation` success, plus an assertion that the User-Agent header actually arrives at the server.
- `geocodeLocation` failures: empty result array, non-200 status, invalid JSON, unparseable latitude, unparseable longitude.
- `loadCoordinatesFile` / `saveCoordinatesFile` round-trip in a `t.TempDir()`.
- `loadCoordinatesFile` returning nil for a missing file.
- `collectUniqueLocations` deduplicating and sorting.
- `GeocodeAllLocations` skipping already-cached locations, proven with a request counter on the mock server.

Additions to `handlers/artist_test.go`:

- `buildConcertStops` ordering stops by earliest date, not alphabetically, and numbering from 1.
- `buildConcertStops` marking `HasCoordinates` false for an ungeocoded location.
- `mapDataJSON` producing the expected keys, and excluding `RawLocation`.

Every test that touches `APIs.GeocodeURL`, `coordinates_file_path`, `nominatim_request_interval`, `Relations` or `coordinates_cache` saves the original and restores it in `t.Cleanup`. Without that, one test leaves the next pointed at a dead server.

`coordinates_file_path` and `nominatim_request_interval` are `var` rather than `const` specifically so these tests are possible.

---

## 11. Verification

```bash
gofmt -l .            # expect no output
go vet ./...
go test ./...
```

**First run**, populates the cache, roughly 3.2 minutes, once:

```bash
go run .
```

```bash
git status                                  # data/coordinates.json is new
head -20 data/coordinates.json
grep -c latitude data/coordinates.json      # expect close to 189
```

**Second run must start instantly.** That proves the cache.

**In the browser** at `http://localhost:8080`:

1. Open any artist. The map renders below the locations list with numbered markers.
2. Marker `1` is the earliest concert, not the alphabetically first location. Cross-check against the dates in the list above the map.
3. The route line connects markers in ascending order.
4. Clicking a marker shows the formatted location and its dates.
5. All markers fit in view on load.

**Error handling.** Delete `data/coordinates.json` and point `GeocodeURL` at a dead address such as `http://127.0.0.1:1/search`. Every geocode fails and is logged, the server still starts, artist pages still render the text list, and the map area shows its unavailable message instead of a broken box.

Note that killing the network entirely does not test this path: `FetchAll` fails first, `Artists` stays empty, and every artist ID returns 404 before the template is ever reached.

Setting `nominatim_request_interval` to zero temporarily makes the all-fail run finish in about a second instead of 3.2 minutes.

**Unchanged behaviour:**

- `http://localhost:8080/artist?id=abc` → 400
- `http://localhost:8080/artist?id=9999` → 404
- an unknown path → 404

**Finally, commit `data/coordinates.json`** so the cache ships with the repo.