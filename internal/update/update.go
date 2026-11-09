// Package update asks GitHub for Segra's latest release and compares it
// with the version that is running.
package update

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Repository is the GitHub repository whose releases are the app's.
const Repository = "beriholic/segra"

// Endpoint is the GitHub API's latest release of Repository. GitHub leaves
// out drafts and prereleases from this one, which is what a stable build
// wants.
const Endpoint = "https://api.github.com/repos/" + Repository + "/releases/latest"

// maxReleaseBody bounds the release JSON the checker reads. The notes of a
// release are text, so a megabyte is far more than one can hold.
const maxReleaseBody = 1 << 20

// Asset is one file of a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release is the part of a GitHub release the app shows.
type Release struct {
	Tag         string    `json:"tag_name"`
	Name        string    `json:"name"`
	Notes       string    `json:"body"`
	URL         string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	Assets      []Asset   `json:"assets"`
}

// AssetFor returns the release file that installs on goos, judged by its
// name. It prefers an installer and falls back to an archive, and reports
// false when the release has neither.
func (r Release) AssetFor(goos string) (Asset, bool) {
	for _, a := range r.Assets {
		name := strings.ToLower(a.Name)
		switch goos {
		case "darwin":
			if strings.HasSuffix(name, ".dmg") {
				return a, true
			}
		case "windows":
			if strings.HasSuffix(name, ".exe") && strings.Contains(name, "setup") {
				return a, true
			}
		case "linux":
			if strings.HasSuffix(name, ".deb") {
				return a, true
			}
		}
	}
	// Older releases may name their packages differently, so take the
	// first file that installs, even when it is not the preferred one.
	for _, a := range r.Assets {
		name := strings.ToLower(a.Name)
		switch goos {
		case "windows":
			if strings.HasSuffix(name, ".exe") {
				return a, true
			}
		case "linux":
			if strings.HasSuffix(name, ".tar.gz") {
				return a, true
			}
		}
	}
	return Asset{}, false
}

// Result compares the latest release with the running version.
type Result struct {
	// Current is the running version, as the app reports it.
	Current string
	// Release is the latest release GitHub offers.
	Release Release
	// Comparable is false when Current is not a version number, as in a
	// development build: there is then nothing to compare the release with.
	Comparable bool
	// Newer reports that Release is newer than Current.
	Newer bool
}

// CheckAt fetches a release from endpoint, which serves the GitHub API's
// release JSON, and compares it with current. A current version that is
// not a release number, such as an empty one, makes a result that compares
// nothing rather than an error.
func CheckAt(ctx context.Context, client *http.Client, endpoint, current string) (Result, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", userAgent(current))

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			return Result{}, fmt.Errorf("check for updates: GitHub said %s", resp.Status)
		}
		return Result{}, fmt.Errorf("check for updates: GitHub said %s: %s", resp.Status, detail)
	}

	var release Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseBody)).Decode(&release); err != nil {
		return Result{}, fmt.Errorf("check for updates: read the release: %w", err)
	}
	if strings.TrimSpace(release.Tag) == "" {
		return Result{}, errors.New("check for updates: the release has no tag")
	}
	latest, err := ParseVersion(release.Tag)
	if err != nil {
		return Result{}, fmt.Errorf("check for updates: %w", err)
	}

	result := Result{Current: current, Release: release}
	running, err := ParseVersion(current)
	if err != nil {
		return result, nil
	}
	result.Comparable = true
	result.Newer = latest.Compare(running) > 0
	return result, nil
}

// Check fetches the latest release of Repository and compares it with
// current.
func Check(ctx context.Context, client *http.Client, current string) (Result, error) {
	return CheckAt(ctx, client, Endpoint, current)
}

// userAgent identifies the app to GitHub. GitHub asks every request to
// carry one.
func userAgent(current string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		return "Segra"
	}
	return "Segra/" + strings.TrimPrefix(strings.TrimPrefix(current, "v"), "V")
}

// Version is a release version: major.minor.patch and an optional
// prerelease, as in 1.2.3 or 1.2.3-beta.1. Build metadata is ignored.
type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

// ParseVersion reads a version, with or without the "v" of a release tag.
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Version{}, errors.New("an empty version is not a release version")
	}
	if raw[0] == 'v' || raw[0] == 'V' {
		raw = raw[1:]
	}
	if i := strings.IndexByte(raw, '+'); i >= 0 {
		raw = raw[:i]
	}
	var pre string
	if i := strings.IndexByte(raw, '-'); i >= 0 {
		pre = raw[i+1:]
		raw = raw[:i]
		if pre == "" {
			return Version{}, fmt.Errorf("%q has an empty prerelease", s)
		}
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("%q is not a major.minor.patch version", s)
	}
	var nums [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("%q is not a major.minor.patch version", s)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2], Pre: pre}, nil
}

// Compare orders two versions as semantic versioning does: a prerelease
// comes before the release it leads to, and its identifiers compare one by
// one, numbers numerically.
func (v Version) Compare(other Version) int {
	if c := cmp.Compare(v.Major, other.Major); c != 0 {
		return c
	}
	if c := cmp.Compare(v.Minor, other.Minor); c != 0 {
		return c
	}
	if c := cmp.Compare(v.Patch, other.Patch); c != 0 {
		return c
	}
	return comparePrerelease(v.Pre, other.Pre)
}

func comparePrerelease(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if c := compareIdentifier(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(as), len(bs))
}

func compareIdentifier(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return cmp.Compare(an, bn)
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
