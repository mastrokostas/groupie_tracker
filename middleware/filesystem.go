package middleware

import (
	"net/http"
	"os"
	"path"
	"strings"
)

// NoListFileSystem wraps an http.FileSystem to disable automatic directory
// listings. When a directory is requested it is only served if that directory
// contains an index.html file; otherwise the request is treated as "not found"
// so visitors can never browse the raw file tree.
type NoListFileSystem struct {
	// Fs is the underlying file system that files are actually served from.
	Fs http.FileSystem
}

func (no_list_file_system NoListFileSystem) Open(requested_path string) (http.File, error) {
	// Try to open whatever was requested (file or directory).
	opened_file, open_error := no_list_file_system.Fs.Open(requested_path)
	if open_error != nil {
		return nil, open_error
	}

	// Inspect the entry so we can tell files and folders apart.
	file_info, stat_error := opened_file.Stat()
	if stat_error != nil {
		opened_file.Close()
		return nil, stat_error
	}

	// For a directory, only allow access when it holds an index.html file;
	// otherwise report "not found" to suppress the directory listing.
	if file_info.IsDir() {
		index_file_path := strings.TrimSuffix(requested_path, "/") + "/index.html"
		index_file, index_open_error := no_list_file_system.Fs.Open(index_file_path)
		if index_open_error != nil {
			opened_file.Close()
			return nil, os.ErrNotExist
		}

		// The index file was only opened to confirm it exists; close it
		// immediately so its handle is not leaked on every directory request.
		index_file.Close()
	}

	return opened_file, nil
}

// ServeStaticOrNotFound serves files from file_system and renders the site's own
// 404 page for anything that is not there, so a bad /static/... URL produces the
// same error page as any other unknown path rather than http.FileServer's plain
// text "404 page not found".
//
// The check is done before the file server runs rather than by inspecting its
// response afterwards, which keeps this to a single Open. Directories need no
// special case here: NoListFileSystem above already reports a directory with no
// index.html as missing, so it lands on the same 404 path as a missing file and
// the raw file tree stays unbrowsable.
func ServeStaticOrNotFound(file_system http.FileSystem, not_found_handler http.HandlerFunc) http.Handler {
	file_server := http.FileServer(file_system)

	return http.HandlerFunc(func(response_writer http.ResponseWriter, request *http.Request) {
		// Normalised exactly the way http.FileServer normalises it internally, so
		// that this check and the file server always agree on which path is being
		// asked for.
		requested_path := path.Clean("/" + strings.TrimPrefix(request.URL.Path, "/"))

		opened_file, open_error := file_system.Open(requested_path)
		if open_error != nil {
			not_found_handler(response_writer, request)
			return
		}

		// The file was only opened to confirm it exists; close it straight away so
		// no handle is leaked, and let the file server open and serve it properly.
		opened_file.Close()

		file_server.ServeHTTP(response_writer, request)
	})
}
