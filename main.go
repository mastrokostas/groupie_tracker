package main

import (
	"groupie-tracker/handlers"
	"groupie-tracker/middleware"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {

	initializeData()
	mux := setUpCustomMultiplexer()
	server, port := setUpCustomServer(mux)
	runServer(server, port)

}

// Initalizes required data.
func initializeData() {
	// load data used by handlers
	if err := handlers.FetchAll(); err != nil {
		log.Printf("Failed to fetch data: %v", err)
	}

	// Turn every concert address into coordinates for the artist page map.
	// Cached results are loaded from disk, so this is normally instant.
	if err := handlers.GeocodeAllLocations(); err != nil {
		log.Printf("Failed to geocode locations: %v", err)
	}
}

// Sets up custom multiplexer (mux)
func setUpCustomMultiplexer() *http.ServeMux {
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
	return mux
}

// Sets up custom server
func setUpCustomServer(mux *http.ServeMux) (*http.Server, string) {

	// TCP default port, can be overriden by PORT env variable
	port := "8080"
	port_override := os.Getenv("PORT")
	if port_override != "" {
		port = port_override
	}

	// Custom server as opposed to http generic server
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      middleware.SecureHeaders(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return server, port
}

func runServer(server *http.Server, port string) {
	log.Println("Listening on http://localhost:" + port)
	server_error := server.ListenAndServe()
	if server_error != nil {
		log.Printf("server stopped: %v", server_error)
		os.Exit(1)
	}
}
