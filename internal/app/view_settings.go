package app

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/locales"
)

// settingsView keeps the settings compact, grouped in paper cards.
func (a *App) settingsView(c *ui.Context) {
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		// The right padding keeps the cards and the version line clear of the
		// overlay scroll bar.
		ui.Column(c).FillWidth().Gap(12).Padding(0, 16, 18, 0).Children(func() {
			ui.Text(c, locales.SettingsTitle()).FontSize(26).Bold().TextColor(colText)

			card := func(title string, fn func()) {
				ui.Column(c).FillWidth().Padding(16, 18).Gap(12).Radius(12).Background(colCard).Border(1, colLine).Children(func() {
					ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(colLabel).SingleLine()
					fn()
				})
			}

			card(locales.DownloadFolder(), func() {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.TextInput(c, &a.dirField).Grow(1).Height(32).Background(ui.RGB(0xFF, 0xFF, 0xFF)).Border(1, colLine).Radius(8)
					a.secondaryButton(c, locales.Browse(), func() { go a.browseSettingsDir() })
				})
			})

			card(locales.Downloads(), func() {
				ui.Row(c).Gap(32).Children(func() {
					ui.Column(c).Gap(6).Children(func() {
						ui.Text(c, locales.Connections()).FontSize(12).TextColor(colMuted)
						ui.NumberInput(c, &a.connsField, 1, 64, 1)
						ui.Text(c, locales.ConnectionsHint()).FontSize(11).TextColor(colFaint).MaxWidth(230)
					})
					ui.Column(c).Gap(6).Children(func() {
						ui.Text(c, locales.MaxConcurrent()).FontSize(12).TextColor(colMuted)
						ui.NumberInput(c, &a.concurrentField, 1, 8, 1)
						ui.Text(c, locales.MaxConcurrentHint()).FontSize(11).TextColor(colFaint).MaxWidth(210)
					})
				})
			})

			card(locales.PortLabel(), func() {
				a.switchRow(c, locales.EnableAPI(), locales.EnableAPIHint(), &a.settings.EnableAPI)
				on := a.settings.EnableAPI
				body := ui.Column(c).Gap(12).Children(func() {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.NumberInput(c, &a.portField, 1024, 65535, 1)
						label, color := locales.ApiOff(), colMuted
						switch {
						case on && a.apiErr != "":
							label, color = a.apiErr, colDanger
						case on:
							label, color = fmt.Sprintf("%s 127.0.0.1:%d", locales.ApiRunning(), a.apiPort), colGreen
						}
						ui.Text(c, label).FontSize(11.5).TextColor(color).SingleLine().Ellipsis("…").Grow(1)
					})
					ui.Text(c, locales.Token()).FontSize(11).TextColor(colFaint)
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						ui.TextInput(c, &a.settings.APIToken).ReadOnly(true).Font("monospace").FontSize(11).
							Grow(1).Height(30).Background(ui.RGB(0xFF, 0xFF, 0xFF)).Border(1, colLine).Radius(8)
						a.secondaryButton(c, locales.CopyToken(), func() {
							mygo.Clipboard.WriteText(a.settings.APIToken)
							c.Toast(locales.Copied())
						})
					})
					ui.Text(c, locales.TokenHint()).FontSize(11).TextColor(colFaint)
				})
				if !on {
					body.Opacity(0.45)
				}
			})

			card(locales.Language(), func() {
				labels, tags := languageChoices()
				a.langPick = labels[0]
				for i, tag := range tags {
					if tag == a.langTag {
						a.langPick = labels[i]
					}
				}
				sel := ui.Select(c, &a.langPick, labels).Width(180).Height(30)
				if sel.Changed() {
					for i, label := range labels {
						if label == a.langPick {
							a.setLanguage(tags[i])
						}
					}
				}
			})

			card(locales.Behavior(), func() {
				a.switchRow(c, locales.Notifications(), locales.NotificationsHint(), &a.settings.Notifications)
				a.switchRow(c, locales.AutoResume(), locales.AutoResumeHint(), &a.settings.AutoResume)
				a.switchRow(c, locales.ClipboardFill(), locales.ClipboardFillHint(), &a.settings.ClipboardFill)
				a.switchRow(c, locales.OpenAtLogin(), "", &a.settings.OpenAtLogin)
				a.switchRow(c, locales.CloseToTray(), locales.CloseToTrayHint(), &a.settings.CloseToTray)
			})

			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				a.primaryButton(c, locales.Save(), func() { a.applySettings(c) })
				ui.Spacer(c)
				ui.Text(c, fmt.Sprintf("%s %s", locales.Version(), mygo.App.Version())).FontSize(11).TextColor(colFaint)
			})
		})
	})
}

// languageChoices are the languages the settings offer: their labels in the
// language the app shows, and the tags the app stores.
func languageChoices() (labels, tags []string) {
	return []string{locales.SystemLanguage(), "English", "简体中文"}, []string{"", "en", "zh-Hans"}
}
