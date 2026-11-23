package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/goccy/go-json"
)

const appRepository = "zatxm/lanboat-app"
const appReleaseAPI = "https://api.github.com/repos/" + appRepository + "/releases/latest"

var appReleaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

type AppRelease struct {
	Tag  string
	Page string
	// GitHub provides the tagged commit time in created_at.
	CommitTime time.Time
}

func (a *App) LatestAppRelease(ctx context.Context) (AppRelease, error) {
	var result AppRelease
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appReleaseAPI, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Lanboat")
	resp, err := a.client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return result, nil
	case http.StatusOK:
	default:
		return result, fmt.Errorf("GitHub Releases: HTTP %d", resp.StatusCode)
	}
	var release struct {
		Tag       string    `json:"tag_name"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&release); err != nil {
		return result, fmt.Errorf("decode GitHub release: %w", err)
	}
	if len(release.Tag) > 64 || !appReleaseTag.MatchString(release.Tag) || release.CreatedAt.IsZero() {
		return result, errors.New("invalid GitHub release metadata")
	}
	return AppRelease{Tag: release.Tag, Page: "https://github.com/" + appRepository + "/releases/tag/" + release.Tag, CommitTime: release.CreatedAt}, nil
}
