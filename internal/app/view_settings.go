package app

import (
	"fmt"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/extension"
	"github.com/beriholic/segra/internal/store"
	"github.com/beriholic/segra/locales"
)

// settingsTwoColumnWidth is where the settings get a second column. Below
// it the cards keep one column, so that a narrow window wraps the controls
// instead of squeezing them.
const settingsTwoColumnWidth = 1080

// settingsMaxWidth keeps the cards a readable width on a wide window: the
// settings are a page to read, not a list to scan across.
const settingsMaxWidth = 1040

// settingsView keeps the settings in paper cards, in two columns when the
// window has room for them. The left column holds what downloads and the
// extension need; the right one holds how the app looks and behaves.
func (a *App) settingsView(c *ui.Context) {
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		// The right padding keeps the cards and the version line clear of the
		// overlay scroll bar.
		ui.Column(c).FillWidth().MaxWidth(settingsMaxWidth).Margin(0, ui.Auto).
			Gap(16).Padding(0, 16, 18, 0).Children(func() {
			ui.Text(c, locales.SettingsTitle()).FontSize(26).Bold().TextColor(a.pal.text)

			if w, _ := c.Size(); w >= settingsTwoColumnWidth {
				a.settingsColumns(c)
			} else {
				a.settingsStack(c)
			}

			a.settingsFooter(c)
		})
	})
}

// settingsColumns lays the cards out in two columns that stack on their
// own, so that a short card does not stretch a tall one's neighbour to its
// height.
func (a *App) settingsColumns(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Gap(16).Children(func() {
			a.settingsFolderCard(c)
			a.settingsDownloadsCard(c)
			a.settingsPortCard(c)
			a.settingsExtensionCard(c)
		})
		ui.Column(c).Grow(1).Gap(16).Children(func() {
			a.settingsAppearanceCard(c)
			a.settingsLanguageCard(c)
			a.settingsBehaviorCard(c)
			a.settingsUpdatesCard(c)
		})
	})
}

// settingsStack is the one-column order, which reads the same way the two
// columns do: downloads and the extension first, then the app itself.
func (a *App) settingsStack(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		a.settingsFolderCard(c)
		a.settingsDownloadsCard(c)
		a.settingsPortCard(c)
		a.settingsExtensionCard(c)
		a.settingsAppearanceCard(c)
		a.settingsLanguageCard(c)
		a.settingsBehaviorCard(c)
		a.settingsUpdatesCard(c)
	})
}

// settingsCard is one titled group of controls on its own sheet of paper.
func (a *App) settingsCard(c *ui.Context, title string, fn func()) {
	ui.Column(c).FillWidth().Padding(18, 20).Gap(14).Radius(12).
		Background(a.pal.card).Border(1, a.pal.line).Children(func() {
		ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(a.pal.label).SingleLine()
		fn()
	})
}

func (a *App) settingsFolderCard(c *ui.Context) {
	a.settingsCard(c, locales.DownloadFolder(), func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			in := ui.TextInput(c, &a.dirField).Grow(1).Height(32).Background(a.pal.input).Border(1, a.pal.line).Radius(8)
			a.secondaryButton(c, locales.Browse(), func() { go a.browseSettingsDir() })
			// A folder saves once it is the one the user means: a
			// path half typed is not a setting, and no download
			// should land in it.
			if in.Changed() {
				a.fieldDirty = true
			}
			if a.fieldDirty && (!in.Focused() || in.Submitted()) {
				a.commitField()
			}
		})
	})
}

func (a *App) settingsDownloadsCard(c *ui.Context) {
	a.settingsCard(c, locales.Downloads(), func() {
		ui.Row(c).Gap(24).Children(func() {
			ui.Column(c).Gap(6).Grow(1).Children(func() {
				ui.Text(c, locales.Connections()).FontSize(12).TextColor(a.pal.muted)
				if ui.NumberInput(c, &a.connsField, 1, 64, 1).Changed() {
					a.applySettings()
				}
				ui.Text(c, locales.ConnectionsHint()).FontSize(11).TextColor(a.pal.faint)
			})
			ui.Column(c).Gap(6).Grow(1).Children(func() {
				ui.Text(c, locales.MaxConcurrent()).FontSize(12).TextColor(a.pal.muted)
				if ui.NumberInput(c, &a.concurrentField, 1, 8, 1).Changed() {
					a.applySettings()
				}
				ui.Text(c, locales.MaxConcurrentHint()).FontSize(11).TextColor(a.pal.faint)
			})
		})
	})
}

func (a *App) settingsPortCard(c *ui.Context) {
	a.settingsCard(c, locales.PortLabel(), func() {
		a.switchRow(c, locales.EnableAPI(), locales.EnableAPIHint(), &a.settings.EnableAPI)
		on := a.settings.EnableAPI
		body := ui.Column(c).Gap(12).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				if ui.NumberInput(c, &a.portField, 1024, 65535, 1).Changed() {
					a.applySettings()
				}
				label, color := locales.ApiOff(), a.pal.muted
				switch {
				case on && a.apiErr != "":
					label, color = a.apiErr, a.pal.danger
				case on:
					label, color = fmt.Sprintf("%s 127.0.0.1:%d", locales.ApiRunning(), a.apiPort), a.pal.green
				}
				ui.Text(c, label).FontSize(11.5).TextColor(color).SingleLine().Ellipsis("…").Grow(1)
			})
			ui.Text(c, locales.Token()).FontSize(11).TextColor(a.pal.faint)
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.TextInput(c, &a.settings.APIToken).ReadOnly(true).Font("monospace").FontSize(11).
					Grow(1).Height(30).Background(a.pal.input).Border(1, a.pal.line).Radius(8)
				a.secondaryButton(c, locales.CopyToken(), func() {
					mygo.Clipboard.WriteText(a.settings.APIToken)
					c.Toast(locales.Copied())
				})
			})
			ui.Text(c, locales.TokenHint()).FontSize(11).TextColor(a.pal.faint)
		})
		if !on {
			body.Opacity(0.45)
		}
	})
}

func (a *App) settingsExtensionCard(c *ui.Context) {
	a.settingsCard(c, locales.BrowserExtension(), func() {
		ui.Text(c, locales.BrowserExtensionHint()).FontSize(11.5).TextColor(a.pal.faint)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			a.secondaryButton(c, locales.ExportExtension(), func() {
				go a.exportExtension(a.settings.APIPort, a.settings.APIToken)
			})
			if v := extension.Version; v != "" {
				ui.Text(c, "v"+v).FontSize(11).TextColor(a.pal.faint)
			}
		})
		ui.Text(c, locales.ExtensionInstallHint()).FontSize(11).TextColor(a.pal.faint)
	})
}

func (a *App) settingsAppearanceCard(c *ui.Context) {
	a.settingsCard(c, locales.Appearance(), func() {
		labels, tags := themeChoices()
		// The control takes the width of its labels: a track
		// stretched across the card leaves the segments bunched at
		// its start.
		sel := ui.Segmented(c, &a.themePick, labels...).AlignSelf(ui.Start).Label(locales.Appearance())
		if sel.Changed() && a.themePick >= 0 && a.themePick < len(tags) {
			a.setTheme(tags[a.themePick])
		}
		ui.Text(c, locales.AppearanceHint()).FontSize(11).TextColor(a.pal.faint)
	})
}

func (a *App) settingsLanguageCard(c *ui.Context) {
	a.settingsCard(c, locales.Language(), func() {
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
}

func (a *App) settingsBehaviorCard(c *ui.Context) {
	a.settingsCard(c, locales.Behavior(), func() {
		ui.Column(c).Gap(14).Children(func() {
			a.switchRow(c, locales.Notifications(), locales.NotificationsHint(), &a.settings.Notifications)
			a.switchRow(c, locales.AutoResume(), locales.AutoResumeHint(), &a.settings.AutoResume)
			a.switchRow(c, locales.ClipboardFill(), locales.ClipboardFillHint(), &a.settings.ClipboardFill)
			a.switchRow(c, locales.OpenAtLogin(), "", &a.settings.OpenAtLogin)
			// macOS keeps a menu bar app running when its window closes, so
			// there the only choice is whether the Dock icon goes with the
			// window. Elsewhere closing the window would end the app, and
			// that is worth asking about.
			if runtime.GOOS != "darwin" {
				a.switchRow(c, locales.CloseToTray(), locales.CloseToTrayHint(), &a.settings.CloseToTray)
			}
			if runtime.GOOS == "darwin" {
				a.switchRow(c, locales.HideDock(), locales.HideDockHint(), &a.settings.HideDock)
			}
		})
	})
}

func (a *App) settingsUpdatesCard(c *ui.Context) {
	a.settingsCard(c, locales.Updates(), func() {
		a.switchRow(c, locales.CheckUpdates(), locales.CheckUpdatesHint(), &a.settings.CheckUpdates)
		snapshot := a.snapshotUpdate()
		label, color := updateStatus(a.pal, snapshot)
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			status := ui.Text(c, label).FontSize(11.5).TextColor(color).Grow(1).SingleLine().Ellipsis("…")
			if snapshot.Err != "" {
				status.Tooltip(snapshot.Err)
			}
			check := a.secondaryButton(c, locales.CheckNow(), func() { a.requestUpdateCheck(true) })
			if snapshot.Checking {
				check.Disabled(true)
				check.Opacity(0.6)
			}
		})
		if snapshot.Result != nil && snapshot.Result.Newer {
			ui.Row(c).Gap(8).Children(func() {
				a.primaryButton(c, locales.DownloadUpdate(), a.downloadUpdate)
				a.secondaryButton(c, locales.WhatsNew(), a.openUpdateDialog)
			})
		}
	})
}

func (a *App) settingsFooter(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
		// There is no Save button: every control writes its setting
		// down as it changes, and the line says so where the button
		// used to be.
		ui.Text(c, locales.AutoSave()).FontSize(11.5).TextColor(a.pal.faint).SingleLine()
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf("%s %s", locales.Version(), mygo.App.Version())).FontSize(11).TextColor(a.pal.faint)
	})
}

// themeChoices are the appearances the settings offer: their labels in the
// language the app shows, and the tags the app stores.
func themeChoices() (labels, tags []string) {
	return []string{locales.ThemeSystem(), locales.ThemeLight(), locales.ThemeDark()},
		[]string{store.ThemeSystem, store.ThemeLight, store.ThemeDark}
}

// themeIndex is the segment the appearance picker shows for a tag.
func themeIndex(tag string) int {
	_, tags := themeChoices()
	for i, t := range tags {
		if t == tag {
			return i
		}
	}
	return 0
}

// languageChoices are the languages the settings offer: their labels in the
// language the app shows, and the tags the app stores.
func languageChoices() (labels, tags []string) {
	return []string{locales.SystemLanguage(), "English", "简体中文"}, []string{"", "en", "zh-Hans"}
}
