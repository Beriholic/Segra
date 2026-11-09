package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/update"
	"github.com/beriholic/segra/locales"
)

// setUpdateResult is what a finished check leaves behind.
func setUpdateResult(a *App, result update.Result) {
	a.updateMu.Lock()
	a.updateResult = &result
	a.updateMu.Unlock()
}

func newerResult() update.Result {
	return update.Result{
		Current:    "0.1.0",
		Comparable: true,
		Newer:      true,
		Release: update.Release{
			Tag:   "v0.2.0",
			Notes: "Faster downloads.",
			URL:   "https://github.com/beriholic/segra/releases/tag/v0.2.0",
		},
	}
}

func TestSettingsOfferAnUpdateCheck(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	tt := ui.NewTester(a.viewFn, 920, 1200)

	for _, text := range []string{locales.Updates(), locales.CheckUpdates(), locales.CheckNow()} {
		if !tt.HasText(text) {
			t.Fatalf("texts %q do not offer %q", tt.Texts(), text)
		}
	}
}

func TestSettingsShowAnAvailableUpdate(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	setUpdateResult(a, newerResult())
	tt := ui.NewTester(a.viewFn, 920, 1200)

	want := fmt.Sprintf(locales.UpdateAvailableMsg(), "v0.2.0", "v0.1.0")
	if !tt.HasText(want) {
		t.Fatalf("texts %q do not say %q", tt.Texts(), want)
	}
	for _, text := range []string{locales.DownloadUpdate(), locales.WhatsNew()} {
		if !tt.HasText(text) {
			t.Fatalf("texts %q do not offer %q", tt.Texts(), text)
		}
	}
}

func TestUpdateDialogShowsReleaseNotes(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	setUpdateResult(a, newerResult())
	a.updateOpen = true
	tt := ui.NewTester(a.viewFn, 920, 760)

	if !tt.HasText(locales.UpdateAvailableTitle()) {
		t.Fatalf("texts %q do not show the update dialog", tt.Texts())
	}
	if !tt.HasText("Faster downloads.") {
		t.Fatalf("texts %q do not show the release notes", tt.Texts())
	}
	if !tt.HasText(fmt.Sprintf(locales.UpdateReadyMsg(), "v0.2.0", "v0.1.0")) {
		t.Fatalf("texts %q do not compare the versions", tt.Texts())
	}
}

// The status line covers every state a check can leave the app in.
func TestUpdateStatus(t *testing.T) {
	upToDate := update.Result{Current: "0.1.0", Comparable: true, Release: update.Release{Tag: "v0.1.0"}}
	dev := update.Result{Current: "", Comparable: false, Release: update.Release{Tag: "v0.2.0"}}
	newer := newerResult()
	cases := []struct {
		name     string
		snapshot updateSnapshot
		want     string
	}{
		{name: "checking", snapshot: updateSnapshot{Checking: true}, want: locales.CheckingUpdates()},
		{name: "failed", snapshot: updateSnapshot{Err: "offline"}, want: locales.UpdateCheckFailed()},
		{name: "never", snapshot: updateSnapshot{}, want: locales.UpdateNotChecked()},
		{name: "dev", snapshot: updateSnapshot{Result: &dev}, want: locales.DevBuild()},
		{name: "newer", snapshot: updateSnapshot{Result: &newer},
			want: fmt.Sprintf(locales.UpdateAvailableMsg(), "v0.2.0", "v0.1.0")},
		{name: "current", snapshot: updateSnapshot{Result: &upToDate}, want: locales.UpToDate()},
	}
	for _, c := range cases {
		if got, _ := updateStatus(lightPal, c.snapshot); got != c.want {
			t.Errorf("%s: status = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestVersionLabel(t *testing.T) {
	cases := map[string]string{
		"0.2.0":  "v0.2.0",
		"v0.2.0": "v0.2.0",
		"V0.2.0": "V0.2.0",
		"":       locales.DevBuild(),
	}
	for in, want := range cases {
		if got := versionLabel(in); got != want {
			t.Errorf("versionLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

// clipText counts characters, so a release note in Chinese is shortened
// by characters rather than split in the middle of one.
func TestClipText(t *testing.T) {
	if got := clipText("short", 10); got != "short" {
		t.Errorf("clipText kept %q", got)
	}
	if got := clipText("一二三四五", 3); got != "一二三…" {
		t.Errorf("clipText = %q, want 一二三…", got)
	}
}

func TestPlainNotes(t *testing.T) {
	in := "### Highlights\r\n\r\n- **Segmented** downloads\r\n- [mygo](https://github.com/egoist/mygo) draws it\r\n\r\n\r\n`Segra`\r\n"
	want := "Highlights\n\n- Segmented downloads\n- mygo draws it\n\nSegra"
	if got := plainNotes(in); got != want {
		t.Errorf("plainNotes = %q, want %q", got, want)
	}
}
