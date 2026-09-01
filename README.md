<div style="display:flex; justify-content:center;">
<pre style="overflow-x:auto; overflow-y:hidden; white-space:pre; max-width:100%;"><font color="#1DB954">        
                             _           _                  _             
  __ _ _ __ ___  _   _ _ __ (_) ___     | |_ _ __ __ _  ___| | _____ _ __ 
 / _` | '__/ _ \| | | | '_ \| |/ _ \    | __| '__/ _` |/ __| |/ / _ \ '__|
| (_| | | | (_) | |_| | |_) | |  __/    | |_| | | (_| | (__|   <  __/ |   
 \__, |_|  \___/ \__,_| .__/|_|\___|     \__|_|  \__,_|\___|_|\_\___|_|   
 |___/                |_|                                              </font></pre>
</div>


# Groupie Tracker

A web application, built in Go, that displays information about musical artists and bands: their members, creation date, first album, and the dates and locations of their concerts. The data is pulled at startup from the [Groupie Trackers API](https://groupietrackers.herokuapp.com/api) and rendered server-side with Go's `html/template` package. The interface follows a dark, Spotify-inspired design.

## Features

- **Home page**: a grid of every artist, each card showing the artist's image, name, creation date and first album.
- **Filters**: a sidebar on the home page narrows the grid by creation date, first album date, number of members and concert location, in the browser and without reloading. The two date filters are dual-handle range sliders; members and locations are check boxes, with the locations grouped into countries so that one tick selects every city inside. Full write-up in [`.documentation/filters.md`](.documentation/filters.md).
- **Search**: a search bar above the grid that looks through artist and band names, members, concert locations, first album dates and creation dates at once. It is case-insensitive, starts suggesting from the first character and refreshes on every one after it, and each suggestion says which of the five it is (`Phil Collins — member`, `Phil Collins — artist/band`), with the band it belongs to underneath. Typing also narrows the grid; picking a suggestion opens that artist's page. The matching is done in Go behind a JSON endpoint, `GET /search?q=`. Full write-up in [`.documentation/search.md`](.documentation/search.md).
- **Artist page**: a detail page for a single artist, showing the full member list and every concert location with its dates, sorted chronologically.
- **Error page**: a consistent error page for bad requests, missing artists (404), and server errors (500).
- Location strings from the API (e.g. `new_york-usa`) are reformatted into readable text (e.g. `New York - USA`), with country abbreviations kept in upper case.

## Project structure

```
groupie-tracker/
├── go.mod
├── main.go                     # entry point, sets up routes and starts the server
├── data/
│   └── coordinates.json        # cached coordinates for every concert location
├── handlers/
│   ├── structs.go              # data structures + FetchAll (loads data from the API at startup)
│   ├── data.go                 # generic HTTP/JSON fetch helpers
│   ├── home.go                 # HomeHandler (/) and renderError
│   ├── artist.go               # ArtistHandler (/artist), location formatting, date sorting
│   ├── filters.go              # home page filter model: year bounds, band sizes, country hierarchy
│   ├── search.go               # search index, matching and the /search JSON endpoint
│   └── geocode.go              # geocoding and the coordinates cache
├── templates/
│   ├── index.html              # home page template: filter sidebar + artist grid
│   ├── artist.html             # artist detail page template
│   └── error.html              # error page template
└── static/
    ├── css/
    │   └── style.css           # shared stylesheet for all pages
    └── js/
        ├── filters.js          # home page filtering
        ├── search.js           # home page search bar and its suggestions
        └── map.js              # artist page concert map
```

## How it works

1. On startup, `main.go` calls `handlers.FetchAll()`, which fetches and decodes four endpoints from the Groupie Trackers API into memory: artists, locations, dates, and the relations between them. If any request fails, the server does not start.
2. Four routes are registered:
   - `/` → `HomeHandler`, renders the artist grid.
   - `/artist?id={id}` → `ArtistHandler`, renders the detail page for the artist at that ID.
   - `/search?q={text}` → `SearchHandler`, the only route that answers with JSON rather than a page.
   - `/static/` → serves files from the local `static/` directory (CSS, etc.).
3. Templates are parsed and executed per request. `artist.html` uses a custom template function, `formatLocation`, to turn raw API location strings into readable text, and `ArtistHandler` sorts each location's concert dates chronologically before rendering.
4. `HomeHandler` hands `index.html` a `HomePageData` value built by `handlers/filters.go`: the artist cards, each stamped with its creation year, first album year, member count and concert locations, plus the choices the sidebar offers (the two sliders' bounds, the band sizes present in the data, and every location grouped under its country). `static/js/filters.js` reads those data attributes and hides the cards that do not match, so filtering costs no request and no reload.
5. `SearchHandler` builds an index of every searchable fact — each artist's name, members, concert locations, first album date and creation date — matches the query against it case-insensitively, and answers with the ten best suggestions plus every artist ID the query reached. `static/js/search.js` draws the suggestions and narrows the grid to those artists; from the first character on the search drives the grid and the filter sidebar is switched off, and emptying the box hands it straight back.
6. Invalid or out-of-range artist IDs, and any template errors, are routed through `renderError`, which renders `error.html` with the relevant HTTP status code and message.

## Requirements

- Go **1.22.2** or later
- An internet connection (the app fetches data from the Groupie Trackers API at startup — there is no local database)

## Running the project

From the project root:

```bash
go run main.go
```

The server starts on port `8080`. Once running, open:

```
http://localhost:8080
```

To build a binary instead:

```bash
go build -o groupie-tracker .
./groupie-tracker
```

## Data source

All artist, location, date and relation data comes from the public Groupie Trackers API:

- `https://groupietrackers.herokuapp.com/api/artists`
- `https://groupietrackers.herokuapp.com/api/locations`
- `https://groupietrackers.herokuapp.com/api/dates`
- `https://groupietrackers.herokuapp.com/api/relation`

## Notes

- The app has no persistence layer; all data is held in memory and re-fetched only on restart.
- Routing is done with the standard library's `net/http.ServeMux`, with no external web framework or router.

## ✍️ Authors

- Konstantinos Koletsis
- Stergios Fourlataras