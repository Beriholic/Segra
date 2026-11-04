package app

import (
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/locales"
)

// newDialog is the New Download sheet. The modal's own panel is the card, so
// the dialog has a single surface, like the rest of the window.
func (a *App) newDialog(c *ui.Context) {
	ui.Modal(c, &a.newOpen, func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			ui.Text(c, locales.NewDownloadTitle()).FontSize(16).Bold().TextColor(colText)

			field := func(label string, body func()) {
				ui.Column(c).Gap(5).Children(func() {
					ui.Text(c, label).FontSize(11.5).TextColor(colLabel)
					body()
				})
			}

			field(locales.Link(), func() {
				ui.TextInput(c, &a.newLink).Placeholder(locales.LinkPlaceholder()).Height(32).AutoFocus().
					Background(ui.RGB(0xFF, 0xFF, 0xFF)).Border(1, colLine).Radius(8).OnSubmit(func() { a.submitNew() })
			})

			field(locales.SaveTo(), func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &a.newDir).Grow(1).Height(32).Background(ui.RGB(0xFF, 0xFF, 0xFF)).Border(1, colLine).Radius(8)
					a.secondaryButton(c, locales.Browse(), func() { go a.browseNewDir() })
				})
			})

			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.Connections()).FontSize(11.5).TextColor(colLabel)
				ui.NumberInput(c, &a.newConns, 1, 64, 1)
				ui.Text(c, locales.ConnectionsHint()).FontSize(11).TextColor(colFaint).Grow(1)
			})

			if a.newErr != "" {
				ui.Text(c, a.newErr).FontSize(11.5).TextColor(colDanger)
			}

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				a.secondaryButton(c, locales.Cancel(), func() { a.newOpen = false })
				a.primaryButton(c, locales.Download(), func() { a.submitNew() })
			})
		})
	}).Width(430).Padding(20).Radius(14).Background(colCard).Border(1, colLine).
		Shadow(0, 18, 50, 0, ui.RGBA(20, 20, 30, 0.18))
}
