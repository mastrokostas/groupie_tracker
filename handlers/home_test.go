package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHomeHandler_WrongPath(t *testing.T) {
	writeTempTemplate(t, "error.html", `{{define "error.html"}}{{.Code}}: {{.Message}}{{end}}`)

	req := httptest.NewRequest(http.MethodGet, "/not-home", nil)
	rec := httptest.NewRecorder()

	HomeHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if !strings.Contains(rec.Body.String(), "Page not found") {
		t.Errorf("body = %q, want it to contain %q", rec.Body.String(), "Page not found")
	}
}

func TestHomeHandler_Success(t *testing.T) {
	// The handler now hands the template a HomePageData value rather than the
	// bare artist slice, so the cards live under .Cards.
	writeTempTemplate(t, "index.html", `{{define "index.html"}}{{range .Cards}}{{.Artist.Name}} {{end}}{{end}}`)

	oldArtists := Artists
	Artists = []Artist{{ID: 1, Name: "Queen"}, {ID: 2, Name: "Muse"}}
	t.Cleanup(func() { Artists = oldArtists })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	HomeHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Queen") || !strings.Contains(body, "Muse") {
		t.Errorf("body = %q, missing expected artist names", body)
	}
}

func TestHomeHandler_IncludesFilterData(t *testing.T) {
	// A stub standing in for the sidebar: the two year sliders' bounds and the
	// first country of the location hierarchy.
	writeTempTemplate(t, "index.html", `{{define "index.html"}}
		creation={{.Filters.MinCreationYear}}-{{.Filters.MaxCreationYear}}
		album={{.Filters.MinAlbumYear}}-{{.Filters.MaxAlbumYear}}
		{{range .Filters.MemberCounts}}members={{.}} {{end}}
		{{range .Filters.Countries}}country={{.Name}} {{range .Places}}place={{.Slug}} {{end}}{{end}}
	{{end}}`)

	oldArtists, oldLocations := Artists, Locations
	Artists = []Artist{
		{ID: 1, Name: "Queen", Members: []string{"a", "b"}, CreationDate: 1970, FirstAlbum: "13-07-1973"},
		{ID: 2, Name: "Muse", Members: []string{"a", "b", "c"}, CreationDate: 1994, FirstAlbum: "03-09-1999"},
	}
	Locations = []Location{
		{ID: 1, Locations: []string{"london-uk"}},
		{ID: 2, Locations: []string{"new_york-usa"}},
	}
	t.Cleanup(func() {
		Artists, Locations = oldArtists, oldLocations
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	HomeHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := rec.Body.String()
	wanted_fragments := []string{
		"creation=1970-1994",
		"album=1973-1999",
		"members=2",
		"members=3",
		"country=UK",
		"place=london-uk",
		"country=USA",
		"place=new_york-usa",
	}

	for _, wanted_fragment := range wanted_fragments {
		if !strings.Contains(body, wanted_fragment) {
			t.Errorf("body is missing %q, got: %s", wanted_fragment, body)
		}
	}
}

func TestHomeHandler_MissingTemplate(t *testing.T) {
	// Deliberately do not create templates/index.html, so ParseFiles fails
	// and the handler must fall back to its own error response. We still
	// need templates/error.html to exist for renderError to succeed.
	writeTempTemplate(t, "error.html", `{{define "error.html"}}{{.Code}}: {{.Message}}{{end}}`)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	HomeHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if !strings.Contains(rec.Body.String(), "Template not found") {
		t.Errorf("body = %q, want it to contain %q", rec.Body.String(), "Template not found")
	}
}

func TestRenderError_WithTemplate(t *testing.T) {
	writeTempTemplate(t, "error.html", `{{define "error.html"}}Error {{.Code}}: {{.Message}}{{end}}`)

	rec := httptest.NewRecorder()
	renderError(rec, http.StatusBadRequest, "Invalid Artist ID")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "400") || !strings.Contains(body, "Invalid Artist ID") {
		t.Errorf("body = %q, missing expected code/message", body)
	}
}

func TestRenderError_WithoutTemplate(t *testing.T) {
	// No templates directory at all: renderError must fall back to
	// http.Error with the plain message instead of failing silently.
	rec := httptest.NewRecorder()
	renderError(rec, http.StatusInternalServerError, "boom")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("body = %q, want it to contain %q", rec.Body.String(), "boom")
	}
}
