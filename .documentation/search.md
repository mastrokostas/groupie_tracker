# Groupie Tracker — Search Bar

Complete record of the implemented feature: what was added, why, and how it behaves.

---

## 1. The problem

The site had no search at all. The home page rendered all 52 artists and the only way to
narrow them was the filter sidebar, which answers "show me every band formed after 2000"
but never "where is Phil Collins".

The Search Bar subject asks for a search over at least five cases — **artist/band name,
members, locations, first album date, creation date** — that is **case-insensitive**, that
**suggests as you type**, and where **every suggestion says which of the five it is**. Its
own example is the awkward one: typing `phil` has to bring back both `Phil Collins -
member` and `Phil Collins - artist/band`, two rows with the same text meaning different
things.

The subject also says the program must be written in Go, with the standard library only.

## 2. The solution

Two layers, split the same way the rest of the site is split:

| Layer | Job | Where |
|---|---|---|
| Search | index the data, match a query, rank the answer | Go, behind `GET /search?q=` |
| Presentation | ask, draw the list, move the highlight, narrow the grid | browser JavaScript |

Nothing is matched in the browser. `static/js/search.js` decides *when* to ask and what to
do with the answer; every decision about *what* matches is Go.

**Decisions:**

| Decision | Choice |
|---|---|
| Where matching happens | Go, so the subject's "must be written in Go" holds for the feature itself and the whole matcher is covered by Go tests |
| Transport | One JSON endpoint, `GET /search?q=`, `encoding/json` only |
| Suggestion click | Goes to `/artist?id=` for that row |
| Typing | Narrows the artist grid as well as filling the dropdown |
| Search vs the sidebar | **The search overrides.** From the first character on, the search drives the grid and the sidebar is switched off and dimmed. An empty box → the sidebar takes it straight back |
| When the search starts | On the **first character**, and it refreshes on every one after it |
| Debounce | 150 ms, so typing a word is one request rather than six |
| Dropdown length | Capped at 10 rows |
| Grid narrowing | **Not** capped — see below |
| Search text in the URL | Not kept, for the same reason the filter state is not: it is a view, not an address |

**Why the search overrides the sidebar rather than combining with it.** Both would be
deciding the same thing — whether a card is on screen — and whichever ran last would win,
leaving the count line describing a grid that no longer exists. Handing the grid to exactly
one of them at a time removes the question. Only one piece of code ever writes
`card.hidden`, and which one it is can be read off the page: a dimmed sidebar means the
search is in charge.

**Why the dropdown is capped and the grid is not.** They answer different questions. The
dropdown answers "did you mean this?", and thirty rows of it is no more useful than ten.
The grid answers "who matched?", and cutting that to ten would hide artists that *did*
match. So one query returns two things: the best ten rows, and every artist ID the query
reached. Typing `phil` against the live data shows this exactly — the ten rows are mostly
`Philadelphia - USA`, while the grid correctly keeps 10 artists including U2, whose own
matching row never made the list.

## 3. Search semantics

- The query is **trimmed and lowered once**; every index entry already carries a lowered
  copy of its label. That is the whole of the case-insensitivity — `PHIL`, `phil`, `PhIl`
  and `  phil  ` are one and the same query.
- A match is a plain **substring**, anywhere in the label. This is what makes `1970` reach
  the first album date `24-01-1970` without a second date format ever being indexed.
- An **empty query matches nothing**, not everything. A dropdown that unfolded the moment
  the box was focused would only be in the way.
- The same text can appear under **several types at once**, which is the subject's example:
  `Phil Collins` is indexed once as an `artist/band` and again as a `member` of Genesis.
- Rows are ranked: **labels that begin with the query first**, then the fixed type order
  (artist/band, member, location, first album date, creation date), then alphabetically,
  then by the artist the row belongs to. The last two are there so the list does not
  reshuffle itself between keystrokes.
- Labels that are **blank once trimmed are never indexed** — the API leaves a first album
  date empty now and then, and a blank row would match nothing anyone could type and would
  lead nowhere if clicked. A creation year of `0` is skipped for the same reason.

## 4. New file: `handlers/search.go`

### The five types

Written as constants, so the index that produces them, the ranking that orders them and the
tests that assert on them cannot drift apart:

| Constant | Value | Built from | Label example |
|---|---|---|---|
| `search_type_artist` | `artist/band` | `Artist.Name` | `Queen` |
| `search_type_member` | `member` | each `Artist.Members` entry | `Freddie Mercury` |
| `search_type_location` | `location` | each raw location for that artist | `New York - USA` |
| `search_type_first_album` | `first album date` | `Artist.FirstAlbum` | `12-07-1973` |
| `search_type_creation` | `creation date` | `Artist.CreationDate` | `1970` |

### Types

```go
type SearchEntry struct {
    Label      string
    LowerLabel string
    Type       string
    ArtistID   int
    ArtistName string
}

type Suggestion struct {
    Label      string `json:"label"`
    Type       string `json:"type"`
    ArtistID   int    `json:"artistID"`
    ArtistName string `json:"artistName"`
}

type SearchResponse struct {
    Suggestions []Suggestion `json:"suggestions"`
    ArtistIDs   []int        `json:"artistIDs"`
}
```

`LowerLabel` is stored rather than computed, so a query is lowered once and every one of the
several hundred comparisons after it is a plain `strings.Index`.

`ArtistName` is carried because a member or a location row means little on its own. It is
what tells the subject's two `Phil Collins` rows apart on screen: one says *Genesis*
underneath it, the other says *Phil Collins*.

`SearchResponse` is the two answers, capped differently — section 2 above.

### Reused rather than rewritten

| Needed | Reused from |
|---|---|
| `new_york-usa` → `New York - USA` | `FormatLocation` (`handlers/artist.go`) |
| Artist ID → that artist's raw locations | `buildLocationsByArtistID` (`handlers/filters.go`) |

Locations are indexed in their **readable** form, because that is the form the rest of the
site shows and therefore the form a visitor would type. Nobody searches for `new_york-usa`.

### Functions

| Function | Job |
|---|---|
| `appendSearchEntry` | Adds one fact, skipping anything blank once trimmed |
| `BuildSearchIndex` | One pass over the artists, producing every searchable fact |
| `findMatches` | Trims and lowers the query, keeps every entry containing it, ranks them |
| `sortMatchedEntries` | The four-step ranking of section 3 |
| `buildSuggestions` | The best `limit` matches, as dropdown rows |
| `collectMatchedArtistIDs` | The distinct artists behind **every** match, ascending |
| `Search` | The two together, as one `SearchResponse` |
| `SearchHandler` | `GET /search?q=` |

`matchedEntry` keeps each match beside the position the query was found at. That position is
the whole of the "starts with beats contains" rule, so `phil` lists **Phil Collins** above
**Philadelphia**.

### The endpoint

```
GET /search?q=phil
Content-Type: application/json

{"suggestions":[{"label":"Phil Collins","type":"artist/band","artistID":14,"artistName":"Phil Collins"}, …],
 "artistIDs":[2,13,14,29,30,31,33,36,39,44]}
```

Two details:

- The index is built **per request**, for the same reason `HomeHandler` builds its page data
  per request: the answer always reflects whatever is in the package-level slices, which is
  what keeps the handler tests, which swap those slices out, working. With 52 artists the
  cost is nothing.
- Both slices are `make`d rather than declared, and the response is **marshalled in full
  before anything is written**. The first means no match encodes as `[]` and never as
  `null`, which the browser would fall over on. The second means a marshal failure can still
  become an error page instead of landing halfway into a committed response body — the same
  care `mapDataJSON` takes on the artist page.

## 5. `main.go`

One line:

```go
mux.HandleFunc("/search", handlers.SearchHandler)
```

Registered without a trailing slash, so it is an exact match and catches nothing else.

## 6. `templates/index.html`

The search block sits between the heading and the two column layout:

```html
<div class="search" id="search_box">
    <input type="search" class="search-input" id="search_input"
           placeholder="Search artists, members, locations or dates"
           autocomplete="off"
           role="combobox" aria-expanded="false"
           aria-controls="search_suggestions" aria-autocomplete="list">
    <ul class="search-suggestions" id="search_suggestions" role="listbox" hidden></ul>
</div>
```

Each card gains one attribute, which is how the grid is narrowed to what a query matched:

```html
<div class="card" data-artist-id="{{ .Artist.ID }}" …>
```

`search.js` is loaded after `filters.js`. Deferred scripts run in the order they are
written, so the hook `filters.js` publishes is there by the time `search.js` looks for it.

## 7. New file: `static/js/search.js`

Same conventions as `filters.js` and `map.js`: `const`/`let`, snake_case, arrow functions,
and an early return if the page it belongs to is not the one being rendered.

| Function | Job |
|---|---|
| `handle_search_input` | Every keystroke: an empty box → hand the grid back at once; otherwise take the grid and start the debounce |
| `request_suggestions` | Fetches one query's answer, numbered so a stale reply is dropped |
| `render_suggestions` | Redraws the list, or the single "No matches" line |
| `build_suggestion_row` | One row: label, type pill, and the band underneath for anything that is not an artist |
| `highlight_suggestion` | Moves the highlight, and points `aria-activedescendant` at it |
| `move_active_suggestion` | Arrow keys, wrapping at both ends |
| `open_suggestion` | `/artist?id=` for the highlighted or clicked row |
| `close_suggestions` | Escape, a click outside, or an empty box |
| `apply_search_to_grid` | Keeps the matched artists, updates the count line and the empty state |
| `take_grid_from_filters` / `release_grid_to_filters` | The two ends of the handover |

**Five details worth keeping in mind.**

*The elements are looked up twice.* `filters.js` already holds the grid, the count line and
the empty state under its own names. Both files are classic scripts sharing one global
lexical scope, so a second `const artist_grid` would be a redeclaration and would stop the
page dead. They are looked up again here under `searched_` names.

*Answers can arrive out of order.* Every request is numbered and only the newest one's reply
is used, otherwise a slow answer to `ph` could land after the answer to `phil` and leave the
page showing the wrong query:

```js
latest_request_number += 1;
const this_request_number = latest_request_number;
…
if (this_request_number !== latest_request_number) {
    return;
}
```

Emptying the box bumps that number too, so a reply still in flight cannot arrive afterwards
and narrow a grid that has already been handed back.

*An empty box is answered at once, and that is not debounced.* It leaves the page as it was
before anything was typed: no list, and the sidebar back in charge. That branch is answered
immediately rather than after the 150 ms, so the grid comes back the instant the last
character is deleted, whether by one deletion or by clearing the box outright. It is
`search.js` that decides *when* to ask, so the rule lives here rather than in Go — an empty
`q` answers nothing there either.

*Clicks are taken on `mousedown`.* The box losing focus would otherwise close the list out
from under the pointer before a `click` ever landed.

*A failed request is not an empty result.* The `catch` folds the list away rather than
drawing "No matches", so a network problem never reads as a query that matched nothing.

## 8. `static/js/filters.js` — the handover

Three small additions, nothing else changed:

- A `filters_drive_the_grid` flag, checked at the top of `apply_filters`, which returns
  immediately while it is false. Nothing can reach the cards behind the search's back.
- `set_filters_enabled`, which flips that flag, toggles `filters-disabled` on the sidebar and
  sets `disabled` on every slider and check box — so a sidebar that is not driving the grid
  also emits no change events at all. **The reset button is deliberately left alive**, since
  it is the one way out of any state.
- `reset_all_filters` now empties the search box as well, and announces it:

```js
if (search_input_element !== null && search_input_element.value !== "") {
    search_input_element.value = "";
    search_input_element.dispatchEvent(new Event("input"));
}
```

Dispatched rather than done quietly, so the search hears it through the same event a visitor
deleting the text would raise and hands the grid back through its own normal path.

And one published hook, the only thing either file offers the other:

```js
window.groupie_filters = {
    apply: apply_filters,
    set_enabled: set_filters_enabled,
};
```

## 9. `static/css/style.css`

| Added | Why |
|---|---|
| `.search` | Positioning context, so the list can hang over the layout instead of pushing it down |
| `.search-input` | A pill on the same raised surface as a card, with the accent as its focus border |
| `.search-suggestions` | Absolutely positioned, own `max-height` and thin scrollbar, sitting above the grid on `z-index` |
| `.search-suggestion` | A two column grid: what matched on the left, the type on the right, the band underneath |
| `.suggestion-type` | The type set apart as a small upper case pill — it is the point of the row, not an afterthought |
| `.search-suggestion-empty` | The "No matches" line, which takes neither the pointer nor the highlight |
| `.filters.filters-disabled` | Dimmed while the search owns the grid; the controls are already `disabled` in JavaScript, so this only has to say so |
| `@media (max-width: 900px)` | Full width above the folded sidebar |

## 10. Tests

`handlers/search_test.go`, table-driven in the same shape as the existing tests. The fixture
is built around the subject's own example: Genesis with Phil Collins in it, Phil Collins the
band, and Philadelphia as a place starting with the same letters — so one query exercises
three types and the ranking between them at once. The locations are given out of order and
one artist has no entry, so the ID lookup is genuinely exercised.

| Test | Covers |
|---|---|
| `TestBuildSearchIndex` | All five types, in order, for three artists; locations formatted; creation year as text; the artist with no locations |
| `TestBuildSearchIndex_LowersLabels` | `Label` kept as written, `LowerLabel` lowered |
| `TestBuildSearchIndex_SkipsUnusableValues` | Blank and whitespace-only first album dates, a zero creation year, a blank member name |
| `TestSearch_TypeLabels` | The subject's example, including the order the four rows come back in |
| `TestSearch_IsCaseInsensitive` | `PHIL`, `PhIl`, `  phil  ` and `  PHIL ` all equal to `phil` |
| `TestSearch_Dates` | `1981` reaching both a creation date and a first album date, prefix ranked ahead of mid-string |
| `TestSearch_EmptyQuery` | Empty, spaces and a tab return nothing — and empty slices, not nil |
| `TestSearch_NoMatches` | Same, for a query that simply matches nothing |
| `TestSearch_LimitCapsSuggestionsOnly` | With room for one row, the grid is still told about the artist whose rows were cut |
| `TestSearch_ArtistIDsAreDistinctAndSorted` | An artist matching through four of its facts is named once |
| `TestSearchHandler` | Status, `Content-Type`, the decoded rows and IDs, and `artistName` surviving the round trip |
| `TestSearchHandler_EmptyResultsAreArraysNotNull` | The exact body `{"suggestions":[],"artistIDs":[]}` for no match, an empty `q`, no `q` at all, and spaces |

## 11. Verification

`gofmt -l .` clean, `go vet ./...` clean, `go test ./...` passing. Against the live data with
the server running:

| Request | Result |
|---|---|
| `/search?q=phil` | 10 rows: `Phil Collins` as `artist/band`, `Phil Collins` as `member` twice (Genesis and Phil Collins), five `Philadelphia - USA` locations, then `Jacob Hemphill` and `Manila - Philippines`. `artistIDs` `[2,13,14,29,30,31,33,36,39,44]` |
| `/search?q=PHIL` | Byte-identical to the above |
| `/search?q=1970` | `1970` as a `creation date` for Aerosmith and Queen, then `24-01-1970` as a `first album date` for Deep Purple |
| `/search?q=zzzz` | `{"suggestions":[],"artistIDs":[]}` |
| `/search?q=` | The same |
| Content type | `application/json` |

Driven in a real browser at `localhost:8080`:

| Action | Result |
|---|---|
| Type `phil` | Suggestions appear as typed, each labelled with its type and, where it is not the artist itself, the band underneath |
| The grid | 10 of 52 artists, including U2 — whose own matching row fell outside the ten the dropdown had room for |
| The sidebar | Dimmed, every slider and box `disabled`, the reset button still live |
| `↓` `↓` | Highlight on the second row, `aria-activedescendant` following it |
| `Enter` | Opens the highlighted artist — `queen` then `↓` then `Enter` lands on `/artist?id=1`, Queen |
| Click a row | `Phil Collins - member - Genesis` lands on `/artist?id=13`, Genesis |
| Console | Clean, apart from the pre-existing `favicon.ico` 404 |
