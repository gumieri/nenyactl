package install

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Release is the subset of the GitHub release payload nenyactl consumes.
type Release struct {
	TagName string `json:"tag_name"`
}

// FetchLatestVersion returns the latest nenya release tag using the default
// HTTP client.
func FetchLatestVersion(ctx context.Context) (string, error) {
	return FetchLatestVersionWithHTTP(ctx, http.DefaultClient)
}

// FetchLatestVersionWithHTTP returns the latest nenya release tag using hc.
func FetchLatestVersionWithHTTP(ctx context.Context, hc HTTPDoer) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

// CheckLatestVersion is an alias for FetchLatestVersion.
func CheckLatestVersion(ctx context.Context) (string, error) {
	return FetchLatestVersion(ctx)
}

var (
	githubAPIURL = "https://api.github.com"
)
