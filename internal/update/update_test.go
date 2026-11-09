package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want Version
		bad  bool
	}{
		{in: "1.2.3", want: Version{Major: 1, Minor: 2, Patch: 3}},
		{in: "v1.2.3", want: Version{Major: 1, Minor: 2, Patch: 3}},
		{in: "V1.2.3", want: Version{Major: 1, Minor: 2, Patch: 3}},
		{in: " 1.2.3 ", want: Version{Major: 1, Minor: 2, Patch: 3}},
		{in: "1.2.3-beta.1", want: Version{Major: 1, Minor: 2, Patch: 3, Pre: "beta.1"}},
		{in: "v1.2.3-beta.1+build.5", want: Version{Major: 1, Minor: 2, Patch: 3, Pre: "beta.1"}},
		{in: "1.2.3+build.5", want: Version{Major: 1, Minor: 2, Patch: 3}},
		{in: "", bad: true},
		{in: "1.2", bad: true},
		{in: "1.2.3.4", bad: true},
		{in: "one.two.three", bad: true},
		{in: "1.2.-3", bad: true},
		{in: "1.2.3-", bad: true},
	}
	for _, c := range cases {
		got, err := ParseVersion(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("ParseVersion(%q) = %v, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVersion(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseVersion(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	// Each pair is in the order semantic versioning puts them: older first.
	order := []string{
		"0.1.0",
		"0.1.1",
		"0.2.0",
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha.2",
		"1.0.0-alpha.10",
		"1.0.0-beta",
		"1.0.0-beta.2",
		"1.0.0",
		"1.0.1",
		"1.1.0",
	}
	for i := 0; i < len(order); i++ {
		for j := 0; j < len(order); j++ {
			a, err := ParseVersion(order[i])
			if err != nil {
				t.Fatal(err)
			}
			b, err := ParseVersion(order[j])
			if err != nil {
				t.Fatal(err)
			}
			got := a.Compare(b)
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got != want {
				t.Errorf("%s.Compare(%s) = %d, want %d", order[i], order[j], got, want)
			}
		}
	}
}

func TestCheckFindsNewerRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "Segra/0.1.0" {
			t.Errorf("User-Agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.2.0",
			"name": "Segra 0.2.0",
			"body": "Faster downloads.",
			"html_url": "https://github.com/beriholic/segra/releases/tag/v0.2.0",
			"published_at": "2026-01-02T03:04:05Z",
			"assets": [
				{"name": "segra-0.2.0.dmg", "browser_download_url": "https://example.com/segra.dmg", "size": 10},
				{"name": "segra-setup-0.2.0-amd64.exe", "browser_download_url": "https://example.com/segra.exe", "size": 20},
				{"name": "segra_0.2.0_amd64.deb", "browser_download_url": "https://example.com/segra.deb", "size": 30}
			]
		}`))
	}))
	defer srv.Close()

	res, err := CheckAt(context.Background(), srv.Client(), srv.URL, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Comparable {
		t.Fatal("result is not comparable with 0.1.0")
	}
	if !res.Newer {
		t.Fatal("v0.2.0 is not reported as newer than 0.1.0")
	}
	if res.Release.Tag != "v0.2.0" || res.Release.Notes != "Faster downloads." {
		t.Fatalf("release = %+v", res.Release)
	}
}

func TestCheckUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "v0.1.0"}`))
	}))
	defer srv.Close()

	res, err := CheckAt(context.Background(), srv.Client(), srv.URL, "v0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if res.Newer {
		t.Fatal("the running version is reported as older than itself")
	}
	if !res.Comparable {
		t.Fatal("v0.1.0 is not comparable")
	}
}

// A development build has no version to compare, so the release is
// reported without claiming that it is newer.
func TestCheckWithoutAVersionComparesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "v0.2.0"}`))
	}))
	defer srv.Close()

	res, err := CheckAt(context.Background(), srv.Client(), srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Comparable || res.Newer {
		t.Fatalf("result = %+v, want nothing compared", res)
	}
	if res.Release.Tag != "v0.2.0" {
		t.Fatalf("release tag = %q", res.Release.Tag)
	}
}

func TestCheckReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "API rate limit exceeded", http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := CheckAt(context.Background(), srv.Client(), srv.URL, "0.1.0"); err == nil {
		t.Fatal("a 403 is not reported as an error")
	}
}

func TestCheckReportsAMalformedRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "nightly"}`))
	}))
	defer srv.Close()

	if _, err := CheckAt(context.Background(), srv.Client(), srv.URL, "0.1.0"); err == nil {
		t.Fatal("a release without a version is not reported as an error")
	}
}

func TestAssetFor(t *testing.T) {
	r := Release{Assets: []Asset{
		{Name: "segra-0.2.0-linux-amd64.tar.gz"},
		{Name: "segra-0.2.0.dmg"},
		{Name: "segra_0.2.0_amd64.deb"},
		{Name: "segra-setup-0.2.0-amd64.exe"},
	}}
	cases := map[string]string{
		"darwin":  "segra-0.2.0.dmg",
		"windows": "segra-setup-0.2.0-amd64.exe",
		"linux":   "segra_0.2.0_amd64.deb",
	}
	for goos, want := range cases {
		got, ok := r.AssetFor(goos)
		if !ok || got.Name != want {
			t.Errorf("AssetFor(%q) = %q, %v; want %q", goos, got.Name, ok, want)
		}
	}
	if _, ok := r.AssetFor("plan9"); ok {
		t.Error("AssetFor(plan9) found an installer")
	}
}

// The archive from an older publish is better than nothing when a release
// has no package for the platform.
func TestAssetForFallsBackToAnArchive(t *testing.T) {
	r := Release{Assets: []Asset{{Name: "segra-0.2.0-linux-amd64.tar.gz"}}}
	got, ok := r.AssetFor("linux")
	if !ok || got.Name != "segra-0.2.0-linux-amd64.tar.gz" {
		t.Fatalf("AssetFor(linux) = %q, %v", got.Name, ok)
	}
}
