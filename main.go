package main

import (
	"groupie-tracker/handlers"
	"groupie-tracker/middleware"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {

	logFile := initLog()
	defer logFile.Close()

	initializeData()
	mux := setUpCustomMultiplexer()
	server := setUpCustomServer(mux)
	runServer(server)

}

// Initializes required data.
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

	// create a file server that serves static assets (CSS, images, etc.) from the
	// "static" directory. NoListFileSystem stops the raw file tree being browsed,
	// and ServeStaticOrNotFound renders the site's own 404 page for a missing
	// asset instead of http.FileServer's plain text one.
	static_file_server := middleware.ServeStaticOrNotFound(
		middleware.NoListFileSystem{Fs: http.Dir("static")},
		handlers.NotFoundHandler,
	)
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
func setUpCustomServer(mux *http.ServeMux) *http.Server {

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
	return server
}

// Runs the custom server and logs error
func runServer(server *http.Server) {
	// server.Addr already holds ":<port>", so it doubles as the logged URL.
	log.Println("Listening on http://localhost" + server.Addr)
	server_error := server.ListenAndServe()
	if server_error != nil {
		log.Printf("server stopped: %v", server_error)
		os.Exit(1)
	}
}

func initLog() *os.File {
	// Check the logs directory
	err := os.MkdirAll("logs", 0o775)
	if err != nil {
		log.Fatalf("Error opening the log directory: %v", err)
	}
	// Open (or create) the log file in the append mode so past records are kept.
	logFile, err := os.OpenFile("logs/groupie-tracker_server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}
	// Write every log line to groupie-tracker_server.log & terminal
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime)

	return logFile
}
