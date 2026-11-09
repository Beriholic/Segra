package app

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/locales"
)

// updateNotesRoom is how tall the release notes get before they scroll.
// The release page has them laid out in full, with every link working.
const updateNotesRoom = 168

// updateStatus is the line under the update switch: what the last check
// found, in the colour that says whether it needs anything.
func updateStatus(snapshot updateSnapshot) (string, ui.Color) {
	switch {
	case snapshot.Checking:
		return locales.CheckingUpdates(), colMuted
	case snapshot.Err != "":
		return locales.UpdateCheckFailed(), colDanger
	case snapshot.Result == nil:
		return locales.UpdateNotChecked(), colFaint
	case !snapshot.Result.Comparable:
		return locales.DevBuild(), colFaint
	case snapshot.Result.Newer:
		return fmt.Sprintf(
			locales.UpdateAvailableMsg(),
			versionLabel(snapshot.Result.Release.Tag),
			versionLabel(snapshot.Result.Current),
		), colGreen
	default:
		return locales.UpToDate(), colMuted
	}
}

// updateDialog shows the release notes of the version a check found, with
// the file that installs it. The modal's own panel is the card, as in the
// New Download sheet.
func (a *App) updateDialog(c *ui.Context) {
	snapshot := a.snapshotUpdate()
	if snapshot.Result == nil || !snapshot.Result.Newer {
		return
	}
	release := snapshot.Result.Release
	notes := clipText(plainNotes(release.Notes), 1200)

	dlg := ui.Modal(c, &a.updateOpen, func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			ui.Row(c).Gap(11).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(34, 34).Radius(10).Center().Background(colSel).Children(func() {
					ui.Icon(c, icDownload).FontSize(17).TextColor(colGreen)
				})
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, locales.UpdateAvailableTitle()).FontSize(16).Bold().TextColor(colText)
					ui.Text(c, fmt.Sprintf(
						locales.UpdateReadyMsg(),
						versionLabel(release.Tag),
						versionLabel(snapshot.Result.Current),
					)).FontSize(11.5).TextColor(colMuted)
				})
			})

			ui.Box(c).Height(1).FillWidth().Background(colLine)

			if notes != "" {
				ui.Scroll(c).MaxHeight(updateNotesRoom).FillWidth().Children(func() {
					ui.Text(c, notes).FontSize(11.5).TextColor(colSub)
				})
			} else {
				ui.Text(c, locales.UpdateNotesHint()).FontSize(11.5).TextColor(colMuted)
			}

			ui.Row(c).Gap(8).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				a.secondaryButton(c, locales.Later(), func() { a.updateOpen = false })
				a.secondaryButton(c, locales.WhatsNew(), a.openUpdatePage)
				a.primaryButton(c, locales.DownloadUpdate(), a.downloadUpdate)
			})
		})
	}).Width(468).Padding(20).Radius(14).Background(colCard).Border(1, colLine).
		Shadow(0, 18, 50, 0, ui.RGBA(20, 20, 30, 0.18))

	// The sheet steps up and grows into place from a timeline, as the New
	// Download sheet does: the modal's panel is new in the frame it shows.
	if a.updateOpen {
		tl := dlg.Timeline("open", ui.TimelineOptions{Run: a.updateRun})
		o := tl.Tween(ui.Tween{From: 0.6, To: 1, Duration: motionBase, Ease: ui.EaseOut})
		s := tl.Tween(ui.Tween{From: 0.97, To: 1, Duration: motionSlow, Ease: ui.EaseOut}, ui.WithPrevious(0))
		dlg.Opacity(o).Transform(ui.Scaling(s, s))
	}
}

// clipText shortens a release note to whole characters, so that a long one
// does not make the dialog grow taller than the window.
func clipText(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

var (
	// markdownLink is the [label](url) a release note links with.
	markdownLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	// markdownHeading is the # or ## that starts a heading line.
	markdownHeading = regexp.MustCompile(`(?m)^#{1,6}[ \t]*`)
)

// plainNotes turns the Markdown of a GitHub release into the text the
// dialog shows: the app has no Markdown renderer, and the syntax reads as
// noise next to its own typography. The release page still has the notes
// laid out, with every link working.
func plainNotes(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = markdownLink.ReplaceAllString(s, "$1")
	s = markdownHeading.ReplaceAllString(s, "")
	s = strings.NewReplacer("**", "", "__", "", "`", "").Replace(s)
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}
