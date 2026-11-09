package app

import (
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/locales"
)

// newDialog is the New Download sheet. The modal's own panel is the card, so
// the dialog has a single surface, like the rest of the window.
func (a *App) newDialog(c *ui.Context) {
	dlg := ui.Modal(c, &a.newOpen, func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			ui.Text(c, locales.NewDownloadTitle()).FontSize(16).Bold().TextColor(a.pal.text)

			field := func(label string, body func()) {
				ui.Column(c).Gap(5).Children(func() {
					ui.Text(c, label).FontSize(11.5).TextColor(a.pal.label)
					body()
				})
			}

			field(locales.Link(), func() {
				ui.TextInput(c, &a.newLink).Placeholder(locales.LinkPlaceholder()).Height(32).AutoFocus().
					Background(a.pal.input).Border(1, a.pal.line).Radius(8).OnSubmit(func() { a.submitNew() })
			})

			field(locales.SaveTo(), func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &a.newDir).Grow(1).Height(32).Background(a.pal.input).Border(1, a.pal.line).Radius(8)
					a.secondaryButton(c, locales.Browse(), func() { go a.browseNewDir() })
				})
			})

			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.Connections()).FontSize(11.5).TextColor(a.pal.label)
				ui.NumberInput(c, &a.newConns, 1, 64, 1)
				ui.Text(c, locales.ConnectionsHint()).FontSize(11).TextColor(a.pal.faint).Grow(1)
			})

			if a.newErr != "" {
				ui.Text(c, a.newErr).FontSize(11.5).TextColor(a.pal.danger)
			}

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				a.secondaryButton(c, locales.Cancel(), func() { a.newOpen = false })
				a.primaryButton(c, locales.Download(), func() { a.submitNew() })
			})
		})
	}).Width(430).Padding(20).Radius(14).Background(a.pal.card).Border(1, a.pal.line).
		Shadow(0, 18, 50, 0, a.pal.shadowModal)

	// The sheet steps up and grows into place from a timeline rather than an
	// Enter: the modal's panel is new in the frame it shows, so there is no
	// parent for an Enter to arrive in.
	if a.newOpen {
		tl := dlg.Timeline("open", ui.TimelineOptions{Run: a.newRun})
		o := tl.Tween(ui.Tween{From: 0.6, To: 1, Duration: motionBase, Ease: ui.EaseOut})
		s := tl.Tween(ui.Tween{From: 0.97, To: 1, Duration: motionSlow, Ease: ui.EaseOut}, ui.WithPrevious(0))
		dlg.Opacity(o).Transform(ui.Scaling(s, s))
	}
}
