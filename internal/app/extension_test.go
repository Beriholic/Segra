package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

// readEntry returns the contents of name in the archive.
func readEntry(t *testing.T, zr *zip.Reader, name string) []byte {
	t.Helper()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return data
	}
	t.Fatalf("%s is missing from the zip", name)
	return nil
}

func TestExtensionZip(t *testing.T) {
	var buf bytes.Buffer
	if err := extensionZip(&buf, 47823, "test-token"); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}

	// Chrome loads the unpacked folder as it stands, so the archive has to
	// hold every file the manifest names.
	for _, name := range []string{
		"manifest.json",
		"background.js",
		"options.js",
		"popup.js",
		"popup.html",
		"options.html",
		"style.css",
		"icons/16.png",
		"icons/128.png",
		"_locales/en/messages.json",
		"_locales/zh_CN/messages.json",
	} {
		if len(readEntry(t, zr, name)) == 0 {
			t.Errorf("%s is empty", name)
		}
	}

	// The Go source of the embed lives beside the extension; it is not part
	// of what the browser loads.
	for _, name := range []string{"embed.go", ".DS_Store"} {
		for _, f := range zr.File {
			if f.Name == name {
				t.Errorf("zip carries %s", name)
			}
		}
	}

	var manifest struct {
		Version int `json:"manifest_version"`
	}
	if err := json.Unmarshal(readEntry(t, zr, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 3 {
		t.Errorf("manifest_version = %d, want 3", manifest.Version)
	}

	// A copy from the zip is meant to work without the user copying a token
	// across, so it carries the one the app exported with.
	var config extensionConfig
	if err := json.Unmarshal(readEntry(t, zr, "config.json"), &config); err != nil {
		t.Fatal(err)
	}
	if config.Port != 47823 || config.Token != "test-token" {
		t.Errorf("config = %+v, want port 47823 and the exported token", config)
	}
}
