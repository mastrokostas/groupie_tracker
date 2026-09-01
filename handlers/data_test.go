package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDataHandler_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":1,"name":"Queen"},{"id":2,"name":"Adele"}]`))
	}))
	defer server.Close()

	var dest []Artist
	if err := DataHandler(server.URL, &dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(dest) != 2 {
		t.Fatalf("got %d artists, want 2", len(dest))
	}
	if dest[0].Name != "Queen" || dest[1].Name != "Adele" {
		t.Errorf("unexpected artist names: %+v", dest)
	}
}

func TestDataHandler_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var dest []Artist
	err := DataHandler(server.URL, &dest)
	if err == nil {
		t.Fatal("expected an error for a 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error message %q does not mention the status code", err.Error())
	}
}

func TestDataHandler_NotFoundStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	var dest []Artist
	err := DataHandler(server.URL, &dest)
	if err == nil {
		t.Fatal("expected an error for a 404 response, got nil")
	}
}

func TestDataHandler_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`this is not json`))
	}))
	defer server.Close()

	var dest []Artist
	err := DataHandler(server.URL, &dest)
	if err == nil {
		t.Fatal("expected a JSON decoding error, got nil")
	}
}

func TestDataHandler_NetworkError(t *testing.T) {
	// Nothing is listening on this URL, so the HTTP GET itself should fail.
	var dest []Artist
	err := DataHandler("http://127.0.0.1:0", &dest)
	if err == nil {
		t.Fatal("expected a network error, got nil")
	}
}

func TestFetch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":1,"name":"Muse"}]`))
	}))
	defer server.Close()

	got, err := Fetch[Artist](server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Muse" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestFetch_PropagatesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	got, err := Fetch[Artist](server.URL)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected nil slice on error, got %+v", got)
	}
}
