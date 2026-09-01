package handlers

import (
	"html/template"
	"net/http"
)

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		renderError(w, http.StatusNotFound, "Page not found")
		return
	}
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		renderError(w, http.StatusNotFound, "Template not found")
		return
	}

	// The cards and the sidebar choices are worked out per request rather than
	// once at startup, so the page always reflects whatever is currently in the
	// package level slices. With 52 artists the cost is negligible.
	page_data := BuildHomePageData(Artists, Locations)

	if err := tmpl.Execute(w, page_data); err != nil {
		renderError(w, http.StatusInternalServerError, "Internal Server Error")
	}
}

// NotFoundHandler renders the site's 404 page. HomeHandler already produces it
// for any unknown page path, but renderError is unexported, so this is the door
// the static file server's middleware uses to reach the same page rather than
// letting http.FileServer send its own plain text "404 page not found".
func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	renderError(w, http.StatusNotFound, "Page not found")
}

func renderError(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	tmpl, err := template.ParseFiles("templates/error.html")
	if err != nil {
		http.Error(w, message, statusCode)
		return
	}
	tmpl.Execute(w, map[string]interface{}{
		"Code":    statusCode,
		"Message": message,
	})
}
