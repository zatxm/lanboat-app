package desktop

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type releaseRoundTrip func(*http.Request) (*http.Response, error)

func (fn releaseRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestLatestAppRelease(t *testing.T) {
	a := testApp(t)
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		tag     string
		wantErr bool
	}{
		{"published", 200, `{"tag_name":"v26.1123.104302","created_at":"2026-11-23T02:43:02Z","published_at":"2026-11-23T02:47:09Z","html_url":"https://example.invalid"}`, "v26.1123.104302", false},
		{"missing", 404, `{"message":"Not Found"}`, "", false},
		{"rate limit", 403, `{"message":"rate limit"}`, "", true},
		{"invalid tag", 200, `{"tag_name":"../settings","created_at":"2026-11-23T02:43:02Z"}`, "", true},
		{"missing commit date", 200, `{"tag_name":"v26.1123.104302"}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.client.Transport = releaseRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet || req.URL.String() != appReleaseAPI || req.Header.Get("Authorization") != "" {
					t.Fatalf("unexpected release request: %s %s", req.Method, req.URL)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			release, err := a.LatestAppRelease(context.Background())
			if (err != nil) != tc.wantErr || release.Tag != tc.tag {
				t.Fatalf("result %+v, error %v", release, err)
			}
			if tc.tag != "" && (release.Page != "https://github.com/zatxm/lanboat-app/releases/tag/"+tc.tag || !release.CommitTime.Equal(time.Date(2026, 11, 23, 2, 43, 2, 0, time.UTC))) {
				t.Fatalf("unexpected release page or commit time: %+v", release)
			}
		})
	}
}
