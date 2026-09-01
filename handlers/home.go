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
