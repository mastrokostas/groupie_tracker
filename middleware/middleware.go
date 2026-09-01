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

	// Keeps all resources first-party ('self'), no injected third-party script can run. Site only loads from /static/js. No inline <script> runs.
	response_writer.Header().Set("Content-Security-Policy",
		"default-src 'self';"+
			"script-src 'self';"+
			"style-src 'self';"+
			"font-src 'self';"+
			"img-src 'self';"+
			"connect-src 'self';"+
			"form-action 'self';"+
			"frame-ancestors 'none';"+
			"base-uri 'self';"+
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
