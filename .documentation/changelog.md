# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The
project carries no version tags yet, so changes are grouped by the date they
landed rather than by release, and each entry names the commit it came from.

---

## [Unreleased] — 2026-09-01

A security hardening pass over the HTTP layer, a Go toolchain bump, and a
restructuring of the server entry point. The application's behaviour for a
normal visitor is unchanged; everything here is about what the server sends
alongside a page and what it refuses to serve.

### Security

- **Content-Security-Policy on every response.** Each directive starts from
  `'self'` and then names the exact outside hosts the pages genuinely load from,
  so injected markup has nowhere to pull code or styling from. The allowlist is
  deliberately narrow (`29f71b6`, `cb6d56c`):

  | Directive | Allowed beyond `'self'` | Why |
  |---|---|---|
  | `script-src` | `unpkg.com` | Leaflet, used by the artist page map |
  | `style-src` | `unpkg.com`, `fonts.googleapis.com` | Leaflet stylesheet, Figtree stylesheet |
  | `font-src` | `fonts.gstatic.com` | the Figtree `.woff2` files |
  | `img-src` | `unpkg.com`, `*.tile.openstreetmap.org`, `groupietrackers.herokuapp.com` | Leaflet marker icons, map tiles, artist photos |
  | `connect-src` | — | the search box only calls this site's own `/search` |

  `form-action 'self'`, `frame-ancestors 'none'`, `base-uri 'self'` and
  `object-src 'none'` round out the policy. No `'unsafe-inline'` and no nonce is
  used anywhere: the site has no inline `<script>`, no `<style>` block and no
  `style="..."` attribute, so neither is needed.

- **Five hardening response headers** (`29f71b6`):
  `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  `Referrer-Policy: strict-origin-when-cross-origin`, and
  `Permissions-Policy: geolocation=(), microphone=(), camera=()`.
  `Strict-Transport-Security` is also sent, but only when `ENABLE_HSTS=true`, so
  that running over plain HTTP in development cannot pin `localhost` to HTTPS in
  a developer's browser.

- **Directory listings disabled.** `NoListFileSystem` reports a directory with
  no `index.html` as missing, so `/static/js/` can no longer be used to browse
  the raw file tree (`9e3e31d`).

- **Connection timeouts.** `http.ListenAndServe` applies none at all, which
  leaves a slow client free to hold a connection open indefinitely. Replaced
  with a configured `http.Server` carrying `ReadTimeout` 5s, `WriteTimeout` 10s
  and `IdleTimeout` 120s (`f23aff4`).

### Added

- `middleware` package (`ecb1b2d`), holding:
  - `headers.go` — `SecureHeaders`, the CSP and hardening headers.
  - `filesystem.go` — `NoListFileSystem` and `ServeStaticOrNotFound`.
- `handlers.NotFoundHandler`, an exported entry point onto the existing
  `error.html` page, so middleware outside the package can render it
  (`86200cd`).
- `PORT` environment variable to override the default `:8080` (`f23aff4`).
- `.documentation/deployment-plan.md` (`cf78fd9`).
- `.documentation/changelog.md`, this file (`ecb1b2d`).

### Changed

- **Go language version 1.22.2 → 1.26.2** (`ecb1b2d`). This changes only the
  `go` directive; the compiler in use was already 1.26.x. Bumping it opts the
  module into current defaults for the ~26 version-gated `GODEBUG` settings,
  the practical effect of which is tighter outbound TLS. Nothing in this
  codebase touches the other gated behaviours: no `math/rand`, no cookies, no
  custom `rand.Reader`, no timer channels.
- **`main()` restructured into a table of contents** (`f23aff4`) — the body is
  now four calls (`initializeData`, `setUpCustomMultiplexer`,
  `setUpCustomServer`, `runServer`) with the detail moved into named functions.
- **Static 404s now render the site's own error page.** A bad `/static/...` URL
  previously produced `http.FileServer`'s plain text `404 page not found`; it
  now returns the same `templates/error.html` as any other unknown path, byte
  for byte (`86200cd`).
- `middleware/middleware.go` renamed to `middleware/headers.go` once the package
  grew a second file (`9e3e31d`).

### Fixed

- **`SecureHeaders` never called the next handler** (`cb6d56c`). It set the
  headers and returned, so every request would have answered 200 with an empty
  body once wired up — a site-wide blank page. Not caught by the compiler,
  because an unused struct field is legal Go.
- **The CSP blocked most of the site** (`cb6d56c`). The initial policy was
  `'self'` for every directive, which would have killed the map, both
  stylesheets, the web fonts, and every artist image. Replaced with the
  allowlist above.
- **`NoListFileSystem` built a malformed index path** (`86200cd`).
  `http.FileServer` passes an already-cleaned path with no trailing slash, so
  concatenating `"index.html"` produced `/jsindex.html` rather than
  `/js/index.html`. Listings were still suppressed, but for the wrong reason,
  and a directory that did hold an `index.html` would have 404ed.
- **`NoListFileSystem.Open` recursed into itself** instead of calling the
  underlying `Fs.Open`, re-running the stat and directory branch on every
  lookup (`86200cd`).
- **`StripPrefix` was applied twice** to the static route (`9e3e31d`). Since
  `StripPrefix` replies 404 when its prefix does not match, the second pass
  would have 404ed every CSS and JS file, leaving the site unstyled and
  scriptless.
- `setUpCustomServer` returned a redundant `port` alongside the server, when
  `server.Addr` already carries it (`aa62ee0`).
- Comment typos: `Initalizes` (`aa62ee0`), and `MINE-sniffing` / `deligates` /
  `aftersetting` in the middleware.

### Notes

- `InterceptNotFound`, a response-writer wrapper that watched for a 404 and
  swapped the body, was written and then replaced. `ServeStaticOrNotFound` does
  the same job by checking whether the file exists *before* handing off to the
  file server — about 20 lines instead of 90, and it sidesteps the stale
  `Content-Type: text/plain` that `http.FileServer` leaves behind on its own
  404s.
- The `middleware` package has no tests yet, while `handlers` has 54. The
  missing next-handler call above is exactly the kind of defect a five-line test
  would have caught.

---

## 2026-09-01 — Initial commit (`abd17f0`)

Groupie Tracker as first published: a Go web application that renders musical
artists, their members, first album, and concert dates and locations, from the
public [Groupie Trackers API](https://groupietrackers.herokuapp.com/api).

- Server-side rendering with `html/template`; no external Go dependencies.
- Home page artist grid with client-side filtering by creation date, first
  album date, member count and concert location.
- Search across artist names, members, locations, first album dates and
  creation dates, behind a JSON endpoint at `GET /search?q=`.
- Artist detail page with a chronologically sorted concert list and a Leaflet
  map, backed by a Nominatim geocoding pass cached to `data/coordinates.json`.
- Shared error page for 404 and 500 responses.
- 54 tests across the `handlers` package.
