/*
Copyright 2026 DataKnifeAI.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tshock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	// ReleasesURL is the GitHub releases API for Pryaxis/TShock.
	ReleasesURL = "https://api.github.com/repos/Pryaxis/TShock/releases"
)

var (
	terrariaVersionRe = regexp.MustCompile(`(?i)terraria\s*(?:version)?[:\s]+v?(\d+(?:\.\d+){1,3})`)
	terrariaForRe     = regexp.MustCompile(`(?i)(?:for|supports?|built\s+against)\s+terraria\s+v?(\d+(?:\.\d+){1,3})`)
	plainVersionRe    = regexp.MustCompile(`(?i)\bv?(\d+\.\d+\.\d+(?:\.\d+)?)\b`)
)

// ReleaseInfo is a parsed TShock GitHub release with Terraria compat.
type ReleaseInfo struct {
	TagName         string
	TShockVersion   string
	TerrariaVersion string
	HTMLURL         string
	PublishedAt     time.Time
}

// VersionClient polls TShock GitHub releases.
type VersionClient struct {
	HTTP    *http.Client
	BaseURL string
}

// DefaultVersionClient returns a client with sensible timeouts.
func DefaultVersionClient() *VersionClient {
	return &VersionClient{
		HTTP:    &http.Client{Timeout: 15 * time.Second},
		BaseURL: ReleasesURL,
	}
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

// LatestStable returns the newest non-draft, non-prerelease TShock release.
func (c *VersionClient) LatestStable(ctx context.Context) (*ReleaseInfo, error) {
	base := c.BaseURL
	if base == "" {
		base = ReleasesURL
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?per_page=10", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "DataKnifeAI/terraria-operator")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tshock github releases: HTTP %d", resp.StatusCode)
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode tshock releases: %w", err)
	}
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease {
			continue
		}
		info := ReleaseInfo{
			TagName:       rel.TagName,
			TShockVersion: strings.TrimPrefix(rel.TagName, "v"),
			HTMLURL:       rel.HTMLURL,
			PublishedAt:   rel.PublishedAt,
		}
		info.TerrariaVersion = ParseTerrariaVersion(rel.Name + "\n" + rel.Body)
		return &info, nil
	}
	return nil, fmt.Errorf("no stable TShock releases found")
}

// ParseTerrariaVersion extracts a Terraria version from release notes.
// Tolerant: returns "" (does not error) when notes cannot be parsed.
func ParseTerrariaVersion(notes string) string {
	for _, re := range []*regexp.Regexp{terrariaVersionRe, terrariaForRe} {
		if m := re.FindStringSubmatch(notes); len(m) >= 2 {
			return m[1]
		}
	}
	// Last resort: look for a 1.4.x style version near the word Terraria.
	lower := strings.ToLower(notes)
	idx := strings.Index(lower, "terraria")
	if idx >= 0 {
		window := notes[idx:]
		if len(window) > 120 {
			window = window[:120]
		}
		if m := plainVersionRe.FindStringSubmatch(window); len(m) >= 2 {
			return m[1]
		}
	}
	return ""
}

// CompareVersions returns -1 if a<b, 0 if equal, 1 if a>b (dotted numeric).
func CompareVersions(a, b string) int {
	as := splitVer(a)
	bs := splitVer(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func splitVer(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n := 0
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				break
			}
			n = n*10 + int(ch-'0')
		}
		out = append(out, n)
	}
	return out
}
