package middleware

import (
	"net/http"
	"os"
)

// Holds the handler that requests are passed to aftersetting the security headers.
type secure_headers_handler struct {
	next_handler http.Handler
}

// Sets the security-related response headers and deligates to the wrapped handler.
func (handler secure_headers_handler) ServeHTTP(response_writer http.ResponseWriter, request *http.Request) {

	// SECURE HEADERS

	// Stop MINE-sniffing away from their declared type.
	response_writer.Header().Set("X-Content-Type-Options", "nosniff")
	// Forbid frame embedding (clickjacking defense).
	response_writer.Header().Set("X-Frame-Options", "DENY")
	// Send only the origin (base address) as the referer when navigating to another site.
	response_writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	// Disable browser features not used in this site.
	response_writer.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
	// Tell browser to only reach over HTTPS.
	// Enable (env variable) only where SSL is terminated in front of it.
	if os.Getenv("ENABLE_HSTS") == "true" {
		response_writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}

	// CSP

	// Every directive starts from 'self' and then names the exact outside hosts
	// the pages genuinely load from. Anything not listed is blocked by the
	// browser, so injected markup has nowhere to pull code or styling from.
	//
	// The site has no inline <script>, no <style> block and no style="..."
	// attribute, so no 'unsafe-inline' and no nonce is needed anywhere here.
	// Every remote host below is a real dependency: remove one and the feature
	// beside it stops working.
	response_writer.Header().Set("Content-Security-Policy",
		// Fallback for any resource type not named below.
		"default-src 'self'; "+
			// unpkg.com serves the Leaflet library the artist page map runs on.
			"script-src 'self' https://unpkg.com; "+
			// unpkg.com serves Leaflet's stylesheet, and fonts.googleapis.com the
			// Figtree stylesheet that all three templates link.
			"style-src 'self' https://unpkg.com https://fonts.googleapis.com; "+
			// That Figtree stylesheet points at the actual font files, which sit
			// on a second Google host.
			"font-src 'self' https://fonts.gstatic.com; "+
			// unpkg.com holds Leaflet's marker icons, tile.openstreetmap.org the
			// map tiles, and herokuapp.com the artist photos the API points at.
			"img-src 'self' https://unpkg.com https://*.tile.openstreetmap.org https://groupietrackers.herokuapp.com; "+
			// The search box only ever calls this site's own /search endpoint.
			"connect-src 'self'; "+
			// Forms may only submit back to this site.
			"form-action 'self'; "+
			// Nobody may embed this site in a frame; matches X-Frame-Options above.
			"frame-ancestors 'none'; "+
			// A <base> tag cannot re-point this page's relative URLs elsewhere.
			"base-uri 'self'; "+
			// No <object>, <embed> or <applet> at all.
			"object-src 'none'")

	// Hand the request on to the wrapped handler. The headers set above are
	// already on the response writer, so they travel with whatever it writes.
	handler.next_handler.ServeHTTP(response_writer, request)
}

// SecureHeaders wraps next_handler and attaches a set of security-related HTTP
// response headers to every response before delegating to the wrapped handler.
func SecureHeaders(next_handler http.Handler) http.Handler {
	return secure_headers_handler{next_handler: next_handler}
}
