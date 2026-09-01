package middleware

import (
	"net/http"
	"os"
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
