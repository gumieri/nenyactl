package install

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Release struct {
	TagName string `json:"tag_name"`
}

func FetchLatestVersion(ctx context.Context) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
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

var buf bytes.Buffer