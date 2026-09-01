# Groupie Tracker — Filters

Complete record of the implemented feature: what was added, why, and how it behaves.

---

## 1. The problem

`templates/index.html` rendered all 52 artists in one grid with no way to narrow them
down. The Filters subject requires four filters — creation date, first album date, number
of members, and locations of concerts — and at least two kinds of control: a **range**
filter and a **check box** filter.

The subject also warns about the locations: *"Seattle, Washington, USA is part of
Washington, USA."* A flat list of the 189 distinct locations would ignore that. Picking
"every American concert" would mean ticking 62 boxes one at a time.

## 2. The solution

Two layers that barely touch each other:

| Layer | Job | Where |
|---|---|---|
| Filter model | year bounds, band sizes, the country → place hierarchy | Go, per request |
| Filtering | decide which cards stay on screen | browser JavaScript |

Go renders each card with the values it can be filtered on already sitting in its data
attributes. The browser never has to re-read the visible text or ask the server anything,
so filtering costs one pass over 52 elements and the page never reloads.

**Decisions:**

| Decision | Choice |
|---|---|
| Where filtering happens | The browser — no reload, no extra requests |
| Creation date / first album | Dual-handle range sliders, handles cannot cross |
| Members | Check boxes, one per band size present in the data |
| Locations | Check boxes grouped into expandable countries |
| Country behaviour | Ticking a country ticks all its places; some ticked = `indeterminate` |
| Extras | Reset-all button, live result count, empty-state message |
| Filter state in the URL | Not kept — the filters are a view, not an address |

**Why the country grouping.** It is the direct answer to the subject's hint. Ticking
**USA** ticks all 62 American places in one click, and the matcher still only ever reads
place boxes, so "one country" and "sixty-two cities" are the same operation to it. Unticking
a single city afterwards leaves the country box in the dashed in-between state, which shows
at a glance that the country is only partly selected.

**The data this is built against** (the live API, at the time of writing):

| Fact | Value |
|---|---|
| Artists | 52, IDs 1–52 |
| Creation dates | 1958 – 2015 |
| First album years | 1963 – 2018 |
| Members per artist | 1 – 8, of which 16 artists are solo |
| Unique locations | 189 |
| Countries | 47, with 62 locations in the USA alone |
| Location slug format | always `place-country`, exactly one hyphen |

## 3. Filter semantics

- The four categories combine with **AND** — a card must pass all four.
- Inside members and locations the ticked values combine with **OR**.
- **A category with nothing ticked does not filter at all.** This is what makes the sidebar
  start out showing everything.
- The locations filter reads only the ticked **place** boxes. A country box is a bulk switch
  and a state indicator; it is never consulted by the matcher.
- A first album date that cannot be parsed yields year `0` and **passes** the album range.
  Hiding an artist because of a data problem nobody can see from the page would be worse
  than showing them.

---

## 4. New file: `handlers/filters.go`

The whole filter model. Follows the existing style: small functions, one job each.

### Types

```go
type PlaceOption struct {
    Slug string // raw API value, for example "north_carolina-usa"
    Name string // readable place name, for example "North Carolina"
}

type CountryGroup struct {
    Slug   string        // raw API country segment, for example "usa"
    Name   string        // readable country name, for example "USA"
    Places []PlaceOption // every place in this country, sorted by name
}

type FilterOptions struct {
    MinCreationYear int
    MaxCreationYear int
    MinAlbumYear    int
    MaxAlbumYear    int
    MemberCounts    []int
    Countries       []CountryGroup
}

type ArtistCard struct {
    Artist         Artist
    FirstAlbumYear int
    MemberCount    int
    LocationSlugs  string // space separated, for example "brooklyn-usa london-uk"
}

type HomePageData struct {
    Cards   []ArtistCard
    Filters FilterOptions
}
```

`LocationSlugs` is one space separated string rather than a slice, because that is exactly
what a `data-` attribute can carry and what the browser splits back apart.

### Reused rather than rewritten

| Needed | Reused from |
|---|---|
| Year out of a `dd-mm-yyyy` date | `parseConcertDate` (`handlers/artist.go`) |
| `north_carolina` → `North Carolina`, `usa` → `USA` | `formatLocationSegment` (`handlers/artist.go`) |

The first album dates use the same layout as the concert dates, so `firstAlbumYear` is a
thin wrapper that turns the zero time into `0`:

```go
func firstAlbumYear(raw_first_album string) int {
    parsed_date := parseConcertDate(raw_first_album)
    if parsed_date.IsZero() {
        return 0
    }

    return parsed_date.Year()
}
```

### `splitLocationSlug`

Every location the API returns holds exactly one hyphen, place on the left, country on the
right — including the awkward ones such as `abu_dhabi-united_arab_emirates`. `SplitN` with
a limit of two is therefore enough, and a value with no hyphen is treated as a place with
no country:

```go
func splitLocationSlug(raw_location string) (place_segment, country_segment string) {
    segments := strings.SplitN(raw_location, "-", 2)
    if len(segments) < 2 {
        return raw_location, ""
    }

    return segments[0], segments[1]
}
```

### `buildLocationsByArtistID`

The locations endpoint is indexed by the ID it reports rather than by position, so a missing
entry cannot shift every later artist's locations onto the wrong card. Building it once also
keeps `BuildArtistCards` a single pass instead of a scan per artist.

### The bounds and the choices

- `creationYearBounds` / `albumYearBounds` — minimum and maximum, **skipping zeros**. One
  unreadable date would otherwise drag a slider's lower end down to year zero and make it
  useless.
- `collectMemberCounts` — distinct band sizes, ascending. A count of `0` produces no check
  box, since "0 members" would never be a useful thing to tick.
- `collectCountryGroups` / `buildPlaceOptions` — the hierarchy. Places are gathered into a
  set per country first, because the same location is listed again for every artist that
  played there. Both levels are sorted case-insensitively by their readable name, so `USA`
  sorts with the U's rather than ahead of every lower case letter. Each place's slug is
  rebuilt as `place + "-" + country`, which reproduces the API value exactly — that is what
  the cards carry, so the two can never drift apart.

### Entry point

```go
func BuildHomePageData(artists []Artist, locations []Location) HomePageData {
    return HomePageData{
        Cards:   BuildArtistCards(artists, locations),
        Filters: BuildFilterOptions(artists, locations),
    }
}
```

Both halves are built from the same `Locations` slice, so a place offered in the sidebar
always exists on some card. (`collectUniqueLocations` in `handlers/geocode.go` walks
`Relations` instead — that serves geocoding and is left alone.)

## 5. `handlers/home.go`

The only change: the template is handed the page data instead of the bare slice.

```go
page_data := BuildHomePageData(Artists, Locations)

if err := tmpl.Execute(w, page_data); err != nil {
    renderError(w, http.StatusInternalServerError, "Internal Server Error")
}
```

Built per request rather than once at startup, so the page always reflects whatever is in
the package level slices — which is also what keeps the handler tests, which swap those
slices out, working. With 52 artists the cost is nothing.

## 6. `templates/index.html`

The grid moves into a two column layout with the sidebar beside it. The sidebar holds, in
order: the header with the reset button, the two sliders, the member boxes, and the country
groups. The results column holds the count line, the grid, and the empty state.

Each slider is two range inputs over one track:

```html
<div class="range-track">
    <span class="range-track-fill"></span>
    <input type="range" class="range-input range-input-low"  min="…" max="…" value="…">
    <input type="range" class="range-input range-input-high" min="…" max="…" value="…">
</div>
```

Each country is a native `<details>`, so folding it open and shut costs no JavaScript at
all:

```html
<details class="country-group">
    <summary class="country-summary">
        <label class="check-option country-option">
            <input type="checkbox" class="country-checkbox" value="{{ .Slug }}">
            <span class="country-name">{{ .Name }}</span>
        </label>
        <span class="country-count">{{ len .Places }}</span>
    </summary>
    <div class="place-list">
        {{range .Places}}
        <label class="check-option">
            <input type="checkbox" class="place-checkbox" value="{{ .Slug }}">
            <span>{{ .Name }}</span>
        </label>
        {{end}}
    </div>
</details>
```

And every card carries its filterable values:

```html
<div class="card"
     data-creation-year="{{ .Artist.CreationDate }}"
     data-album-year="{{ .FirstAlbumYear }}"
     data-member-count="{{ .MemberCount }}"
     data-locations="{{ .LocationSlugs }}">
```

Readable names are worked out in Go, so this template needs no FuncMap.

## 7. New file: `static/js/filters.js`

Follows the conventions `static/js/map.js` set: `const`/`let`, snake_case, arrow functions,
and an early return if the page it belongs to is not the one being rendered.

| Function | Job |
|---|---|
| `read_active_filters` | One snapshot of the whole sidebar, so every card is measured against the same state |
| `card_matches_filters` | Applies the four categories to one card |
| `apply_filters` | Runs every card past that state, updates the count and the empty state |
| `refresh_range_slider` | Readout, filled track segment, and the z-index lift |
| `setup_range_slider` | Stops either handle being dragged past the other |
| `refresh_country_checkbox` | Ticked / cleared / `indeterminate` for one country |
| `setup_country_group` | Wires a country to its places in both directions |
| `reset_all_filters` | Sliders back to full width, every box cleared |

**Three details worth keeping in mind.**

*Handles that cannot cross.* Whichever handle moved is pushed back to its neighbour rather
than the two swapping places, so a drag simply stops:

```js
if (Number(low_input.value) > Number(high_input.value)) {
    low_input.value = high_input.value;
}
```

*Handles that cannot be buried.* Both inputs are stretched over the same track, so once the
low handle is dragged into the right hand half it can end up underneath the high one, where
it could no longer be grabbed. It is lifted above at that point:

```js
low_input.style.zIndex = low_percentage > 50 ? "5" : "3";
```

*Ticking a country must not fold it shut.* A click anywhere on a `<summary>` toggles the
`<details>`, and the country's check box sits on that line, so its clicks are stopped:

```js
country_option_label.addEventListener("click", (click_event) => {
    click_event.stopPropagation();
});
```

## 8. `static/css/style.css`

| Added | Why |
|---|---|
| `[hidden] { display: none !important; }` | The filters hide by setting the `hidden` attribute, and `.grid { display: grid }` would otherwise beat it |
| `.home-layout` | 280px sidebar + the rest, aligned to the top so the sidebar can stick |
| `.filters` | Sticky panel with its own scroll, plus `color-scheme: dark` so the browser stops drawing unticked boxes as solid white squares |
| `.range-track` / `.range-input` | The visible groove is a real element; the inputs are transparent, with `pointer-events` switched off except on the thumbs so the one on top does not swallow drags meant for the other |
| `.country-summary::before` | A triangle of our own, since the default marker is removed in both engines, rotated when the group is open |
| `@media (max-width: 900px)` | One column, sidebar folded behind the Filters button |

## 9. Tests

`handlers/filters_test.go`, table-driven in the same shape as the existing tests:

| Test | Covers |
|---|---|
| `TestFirstAlbumYear` | Valid date, unparsable string, empty string, wrong layout |
| `TestSplitLocationSlug` | Normal slug, multi-word place, multi-word country, no hyphen, empty |
| `TestBuildArtistCards` | Member count, album year, joined slugs, an artist with no locations entry, and a locations slice given out of order so the ID lookup is actually exercised |
| `TestBuildFilterOptions` | Bounds, deduplicated member counts, and the whole country hierarchy compared with `reflect.DeepEqual` — sorting, deduplication, naming and slug reconstruction in one assertion |
| `TestBuildFilterOptions_SkipsUnusableValues` | No "0 members" box, a zero creation year and an unreadable album date left out of the bounds, a country-less location left out of the hierarchy |
| `TestBuildHomePageData_CombinesCardsAndFilters` | Both halves come back together |
| `TestBuildHomePageData_EmptyInput` | No panic and no nonsense bounds when there is no data |

`handlers/home_test.go` gains `TestHomeHandler_IncludesFilterData`, which renders a stub
template and asserts the bounds, the member counts and the country hierarchy all reach it.
`TestHomeHandler_Success` was updated for the new shape (`.Cards` / `.Artist.Name`).

## 10. Verification

`gofmt -l .` clean, `go vet ./...` clean, `go test ./...` passing. Driven in a real browser
against the live data:

| Action | Result |
|---|---|
| Page load | 52 of 52 artists, 47 countries, 189 places, member boxes 1–8, sliders 1958–2015 and 1963–2018 |
| Tick **USA** | All 62 places tick, 37 of 52 artists remain, every one of them plays in the USA, and the group does not fold shut |
| Untick **California** | USA goes `indeterminate`, 33 of 52 remain, matching an independent recount |
| Tick **Members: 1** | 16 of 52 — exactly the solo artists |
| Add **Members: 2** | 18 of 52, confirming OR inside a category |
| Add **creation ≥ 2000** | 14 of 52, every one matching both, confirming AND across categories |
| Drag either handle past the other | It stops at its neighbour, in both directions |
| Creation window 1959–1961 | 0 of 52, grid `display: none`, "No artists match these filters." |
| **Reset all** | 52 of 52, every box cleared, both sliders back to full width |
| Below 900px | Single column, sidebar folded behind the Filters button, `aria-expanded` tracking it |
| Artist page | Unaffected: Leaflet map, its 8 numbered markers and the back button all still work |
