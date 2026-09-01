package main

import (
	"groupie-tracker/handlers"
	"log"
	"net/http"
)

func main() {
	// load data used by handlers
	if err := handlers.FetchAll(); err != nil {
		log.Printf("Failed to fetch data: %v", err)
	}

	// Turn every concert address into coordinates for the artist page map.
	// Cached results are loaded from disk, so this is normally instant.
	if err := handlers.GeocodeAllLocations(); err != nil {
		log.Printf("Failed to geocode locations: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handlers.HomeHandler)

	mux.HandleFunc("/artist", handlers.ArtistHandler)

	// create a file server that serves static assets (CSS, images, etc.) from the "static" directory
	static_file_server := http.FileServer(http.Dir("static"))
	// register the file server under the "/static/" URL path; StripPrefix removes the leading
	// "/static/" so that a request for "/static/css/style.css" maps to the file "static/css/style.css"
	mux.Handle("/static/", http.StripPrefix("/static/", static_file_server))

	// The search box calls this on every keystroke and gets JSON back rather
	// than a page. Registered without a trailing slash so it is an exact match
	// and no other path is caught by it.
	mux.HandleFunc("/search", handlers.SearchHandler)

	log.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
