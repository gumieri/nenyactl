package install

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchLatestVersionWithHTTP(t *testing.T) {
	t.Run("returns tag name on success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := Release{TagName: "v1.2.3"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		savedURL := githubAPIURL
		githubAPIURL = server.URL
		defer func() { githubAPIURL = savedURL }()

		tag, err := FetchLatestVersionWithHTTP(context.Background(), server.Client())
		if err != nil {
			t.Fatalf("FetchLatestVersionWithHTTP() error = %v", err)
		}
		if tag != "v1.2.3" {
			t.Errorf("expected v1.2.3, got %s", tag)
		}
	})

	t.Run("returns error on non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		savedURL := githubAPIURL
		githubAPIURL = server.URL
		defer func() { githubAPIURL = savedURL }()

		_, err := FetchLatestVersionWithHTTP(context.Background(), server.Client())
		if err == nil {
			t.Fatal("expected error for 404, got nil")
		}
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		savedURL := githubAPIURL
		githubAPIURL = server.URL
		defer func() { githubAPIURL = savedURL }()

		_, err := FetchLatestVersionWithHTTP(context.Background(), server.Client())
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})

	t.Run("returns error on context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(Release{TagName: "v1.0.0"})
		}))
		defer server.Close()

		savedURL := githubAPIURL
		githubAPIURL = server.URL
		defer func() { githubAPIURL = savedURL }()

		_, err := FetchLatestVersionWithHTTP(ctx, server.Client())
		if err == nil {
			t.Fatal("expected context cancellation error, got nil")
		}
	})
}

func TestFetchLatestVersion(t *testing.T) {
	t.Run("delegates to FetchLatestVersionWithHTTP", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := Release{TagName: "v2.0.0"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		savedURL := githubAPIURL
		githubAPIURL = server.URL
		defer func() { githubAPIURL = savedURL }()

		tag, err := FetchLatestVersion(context.Background())
		if err != nil {
			t.Fatalf("FetchLatestVersion() error = %v", err)
		}
		if tag != "v2.0.0" {
			t.Errorf("expected v2.0.0, got %s", tag)
		}
	})
}

func TestCheckLatestVersion(t *testing.T) {
	t.Run("delegates to FetchLatestVersion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := Release{TagName: "v3.0.0"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		savedURL := githubAPIURL
		githubAPIURL = server.URL
		defer func() { githubAPIURL = savedURL }()

		tag, err := CheckLatestVersion(context.Background())
		if err != nil {
			t.Fatalf("CheckLatestVersion() error = %v", err)
		}
		if tag != "v3.0.0" {
			t.Errorf("expected v3.0.0, got %s", tag)
		}
	})
}
