package install

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Release struct {
	TagName string `json:"tag_name"`
}

func FetchLatestVersion(ctx context.Context) (string, error) {
	return FetchLatestVersionWithHTTP(ctx, http.DefaultClient)
}

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

func CheckLatestVersion(ctx context.Context) (string, error) {
	return FetchLatestVersion(ctx)
}

var (
	githubAPIURL = "https://api.github.com"
)