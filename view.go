package main

import (
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/locales"
)

// viewFn draws the window. It runs every frame; all state lives on app.
func (a *app) viewFn(c *ui.Context) {
	a.refresh()
	th := c.Theme()

	ui.Column(c).Fill().Background(th.Background).Children(func() {
		a.header(c, th)
		ui.Divider(c)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c, th)
			a.content(c, th)
		})
		ui.Divider(c)
		a.statusBar(c, th)
		a.newDialog(c, th)
	})
}

func (a *app) header(c *ui.Context, th *ui.Theme) {
	ui.Row(c).Padding(14, 20).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, "Segra").FontSize(20).Bold()
			ui.Text(c, locales.Tagline()).FontSize(11).TextColor(th.TextMuted)
		})
		ui.Spacer(c)
		ui.SearchField(c, &a.search).Placeholder(locales.Search()).Width(220)
		ui.PrimaryButton(c, locales.NewDownload()).OnClick(a.openNew)
	})
}

func (a *app) sidebar(c *ui.Context, th *ui.Theme) {
	side := ui.Sidebar(c, &a.view, func() {
		ui.SidebarSection(c, locales.Downloads(), &a.downloadsOpen, func() {
			ui.SidebarItem(c, "all", nil, locales.All())
			ui.SidebarItem(c, "active", nil, locales.Active())
			ui.SidebarItem(c, "completed", nil, locales.Completed())
			ui.SidebarItem(c, "failed", nil, locales.Failed())
		})
		ui.SidebarSection(c, locales.Settings(), &a.settingsOpen, func() {
			ui.SidebarItem(c, "settings", nil, locales.General())
		})
	}).Width(210).Background(th.Surface)
	if side.Changed() {
		a.refresh()
	}
}

func (a *app) content(c *ui.Context, th *ui.Theme) {
	ui.Column(c).Grow(1).Children(func() {
		if a.view == "settings" {
			a.settingsView(c, th)
			return
		}
		a.downloadsToolbar(c, th)
		if len(a.rows) == 0 {
			a.emptyState(c, th)
			return
		}
		list := ui.List(c, &a.list, len(a.rows), func(i int) { a.row(c, th, i) }).Grow(1)
		if list.Submitted() && a.selected >= 0 && a.selected < len(a.rows) {
			t := a.rows[a.selected]
			if t.Status == download.StatusCompleted {
				mygo.Shell.OpenPath(t.Path())
			}
		}
	})
}

func (a *app) downloadsToolbar(c *ui.Context, th *ui.Theme) {
	ui.Row(c).Padding(12, 20, 6, 20).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("%d %s", len(a.rows), locales.Downloads())).FontSize(12).TextColor(th.TextMuted)
		ui.Spacer(c)
		ui.Button(c, locales.PauseAll()).OnClick(func() { a.mgr.PauseAll() })
		ui.Button(c, locales.ResumeAll()).OnClick(func() { a.mgr.ResumeAll() })
		ui.Button(c, locales.OpenFolder()).OnClick(a.openDownloadFolder)
		ui.Button(c, locales.ClearCompleted()).Disabled(a.mgr.ActiveCount() == 0 && len(a.tasks) == 0).OnClick(func() { a.mgr.ClearCompleted(false) })
	})
}

func (a *app) emptyState(c *ui.Context, th *ui.Theme) {
	ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
		ui.Text(c, locales.NoTasks()).FontSize(16).Bold()
		ui.Text(c, locales.NoTasksHint()).FontSize(12).TextColor(th.TextMuted).TextAlign(ui.Center).MaxWidth(360)
		ui.PrimaryButton(c, locales.NewDownload()).OnClick(a.openNew)
	})
}

func (a *app) row(c *ui.Context, th *ui.Theme, i int) {
	t := a.rows[i]
	row := ui.Column(c).Padding(10, 16, 12, 16).Gap(7)
	switch {
	case i == a.selected:
		row = row.Background(th.Selection)
	case row.Hovered():
		row = row.Background(th.SurfaceHover)
	}
	row.ContextMenu(func(m *ui.Menu) { a.rowMenu(m, t) })
	row.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, t.FileName).FontSize(14).Bold().SingleLine().Grow(1)
			ui.Text(c, statusLabel(t)).FontSize(12).TextColor(statusColor(th, t))
		})

		value := float64(0)
		switch {
		case t.Status == download.StatusCompleted:
			value = 1
		case t.Size > 0:
			value = t.Percent()
		case t.Status == download.StatusDownloading:
			value = -1
		}
		ui.Progress(c, value)

		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.metaLine(t)).FontSize(12).TextColor(th.TextMuted).SingleLine().Grow(1)
			if t.Status == download.StatusCompleted {
				ui.Button(c, locales.OpenFile()).OnClick(func() { mygo.Shell.OpenPath(t.Path()) })
			} else {
				ui.Button(c, toggleLabel(t)).OnClick(func() { a.toggle(t) })
			}
		})
	})
}

func (a *app) rowMenu(m *ui.Menu, t download.Task) {
	switch t.Status {
	case download.StatusDownloading, download.StatusQueued:
		if m.Item(locales.Pause()).Chosen() {
			a.toggle(t)
		}
	case download.StatusPaused, download.StatusFailed:
		if m.Item(locales.Resume()).Chosen() {
			a.toggle(t)
		}
	}
	if t.Status == download.StatusCompleted {
		if m.Item(locales.OpenFile()).Chosen() {
			mygo.Shell.OpenPath(t.Path())
		}
	}
	if m.Item(locales.ShowInFolder()).Chosen() {
		a.reveal(t)
	}
	if m.Item(locales.CopyLink()).Chosen() {
		mygo.Clipboard.WriteText(t.URL)
	}
	m.Separator()
	if m.Item(locales.Remove()).Chosen() {
		a.remove(t, false)
	}
	if m.Item(locales.RemoveAndDelete()).Chosen() {
		a.remove(t, true)
	}
}

func (a *app) statusBar(c *ui.Context, th *ui.Theme) {
	ui.Row(c).Padding(7, 16).Gap(12).AlignItems(ui.Center).Children(func() {
		if a.apiErr != "" {
			ui.Text(c, locales.ApiRunning()+": "+a.apiErr).FontSize(11).TextColor(th.Danger).SingleLine().Grow(1)
		} else {
			ui.Text(c, fmt.Sprintf("%s 127.0.0.1:%d", locales.ApiRunning(), a.apiPort)).FontSize(11).TextColor(th.TextMuted).Grow(1)
		}
		ui.Text(c, fmt.Sprintf("%d %s", a.mgr.ActiveCount(), locales.Active())).FontSize(11).TextColor(th.TextMuted)
	})
}

func (a *app) settingsView(c *ui.Context, th *ui.Theme) {
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(20, 22, 24).Gap(14).Children(func() {
			ui.Text(c, locales.SettingsTitle()).FontSize(18).Bold()

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.DownloadFolder()).FontSize(12).TextColor(th.TextMuted)
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.TextInput(c, &a.dirField).Grow(1)
					ui.Button(c, locales.Browse()).OnClick(func() { go a.browseSettingsDir() })
				})
			})

			ui.Row(c).Gap(20).Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, locales.Connections()).FontSize(12).TextColor(th.TextMuted)
					a.stepperField(c, th, &a.connsField, 1, 64, 1)
					ui.Text(c, locales.ConnectionsHint()).FontSize(11).TextColor(th.TextMuted).MaxWidth(240)
				})
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, locales.MaxConcurrent()).FontSize(12).TextColor(th.TextMuted)
					a.stepperField(c, th, &a.concurrentField, 1, 8, 1)
					ui.Text(c, locales.MaxConcurrentHint()).FontSize(11).TextColor(th.TextMuted).MaxWidth(240)
				})
			})

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.PortLabel()).FontSize(12).TextColor(th.TextMuted)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					a.stepperField(c, th, &a.portField, 1024, 65535, 1)
					ui.Text(c, locales.PortHint()).FontSize(11).TextColor(th.TextMuted).Grow(1)
				})
				ui.Text(c, fmt.Sprintf("%s 127.0.0.1:%d", locales.ApiRunning(), a.apiPort)).FontSize(11).TextColor(th.Success).SingleLine()
			})

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.Token()).FontSize(12).TextColor(th.TextMuted)
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.TextInput(c, &a.settings.APIToken).ReadOnly(true).Font("monospace").FontSize(11).Grow(1)
					ui.Button(c, locales.CopyToken()).OnClick(func() {
						mygo.Clipboard.WriteText(a.settings.APIToken)
						c.Toast(locales.Copied())
					})
				})
				ui.Text(c, locales.TokenHint()).FontSize(11).TextColor(th.TextMuted)
			})

			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, locales.Language()).FontSize(12).TextColor(th.TextMuted)
				ui.Row(c).Gap(6).Children(func() {
					a.languageButton(c, th, locales.SystemLanguage(), "")
					a.languageButton(c, th, "English", "en")
					a.languageButton(c, th, "简体中文", "zh-Hans")
				})
			})

			ui.Divider(c)

			a.switchRow(c, th, locales.Notifications(), locales.NotificationsHint(), &a.settings.Notifications)
			a.switchRow(c, th, locales.AutoResume(), locales.AutoResumeHint(), &a.settings.AutoResume)
			a.switchRow(c, th, locales.OpenAtLogin(), "", &a.settings.OpenAtLogin)
			a.switchRow(c, th, locales.CloseToTray(), locales.CloseToTrayHint(), &a.settings.CloseToTray)

			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.PrimaryButton(c, locales.Save()).OnClick(func() { a.applySettings(c) })
				ui.Text(c, fmt.Sprintf("%s %s", locales.Version(), mygo.App.Version())).FontSize(11).TextColor(th.TextMuted)
				ui.Spacer(c)
				ui.Text(c, locales.AboutText()).FontSize(11).TextColor(th.TextMuted).MaxWidth(320).TextAlign(ui.End)
			})
		})
	})
}

func (a *app) stepperField(c *ui.Context, th *ui.Theme, value *float64, lo, hi, step float64) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("%.0f", *value)).FontSize(13).Bold().Width(52)
		ui.Stepper(c, value, lo, hi, step)
	})
}

func (a *app) languageButton(c *ui.Context, th *ui.Theme, label, tag string) {
	btn := ui.Button(c, label)
	if a.langTag == tag {
		btn = btn.Background(th.Accent).TextColor(th.AccentText)
	}
	btn.OnClick(func() { a.setLanguage(tag) })
}

func (a *app) switchRow(c *ui.Context, th *ui.Theme, label, hint string, value *bool) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Column(c).Gap(1).Grow(1).Children(func() {
			ui.Text(c, label).FontSize(13)
			if hint != "" {
				ui.Text(c, hint).FontSize(11).TextColor(th.TextMuted)
			}
		})
		ui.Switch(c, value)
	})
}

func (a *app) newDialog(c *ui.Context, th *ui.Theme) {
	ui.Modal(c, &a.newOpen, func() {
		ui.Column(c).Width(520).Padding(22).Gap(12).Children(func() {
			ui.Text(c, locales.NewDownloadTitle()).FontSize(17).Bold()

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.Link()).FontSize(12).TextColor(th.TextMuted)
				ui.TextInput(c, &a.newLink).Placeholder(locales.LinkPlaceholder()).OnSubmit(func() { a.submitNew() })
			})

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.SaveTo()).FontSize(12).TextColor(th.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &a.newDir).Grow(1)
					ui.Button(c, locales.Browse()).OnClick(func() { go a.browseNewDir() })
				})
			})

			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.Connections()).FontSize(12).TextColor(th.TextMuted)
				a.stepperField(c, th, &a.newConns, 1, 64, 1)
				ui.Text(c, locales.ConnectionsHint()).FontSize(11).TextColor(th.TextMuted).Grow(1)
			})

			if a.newErr != "" {
				ui.Text(c, a.newErr).FontSize(12).TextColor(th.Danger)
			}

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.Cancel()).OnClick(func() { a.newOpen = false })
				ui.PrimaryButton(c, locales.Download()).OnClick(func() { a.submitNew() })
			})
		})
	})
}

func statusLabel(t download.Task) string {
	switch t.Status {
	case download.StatusQueued:
		return locales.Queued()
	case download.StatusDownloading:
		return locales.Downloading()
	case download.StatusPaused:
		return locales.Paused()
	case download.StatusCompleted:
		return locales.CompletedStatus()
	case download.StatusFailed:
		return locales.FailedStatus()
	default:
		return string(t.Status)
	}
}

func statusColor(th *ui.Theme, t download.Task) ui.Color {
	switch t.Status {
	case download.StatusDownloading:
		return th.Accent
	case download.StatusCompleted:
		return th.Success
	case download.StatusFailed:
		return th.Danger
	case download.StatusPaused:
		return th.Warning
	default:
		return th.TextMuted
	}
}

func toggleLabel(t download.Task) string {
	switch t.Status {
	case download.StatusDownloading, download.StatusQueued:
		return locales.Pause()
	case download.StatusPaused, download.StatusFailed:
		return locales.Resume()
	default:
		return locales.Resume()
	}
}

func (a *app) metaLine(t download.Task) string {
	switch t.Status {
	case download.StatusCompleted:
		return formatBytes(t.Size) + " · " + t.Dir
	case download.StatusFailed:
		if t.Error != "" {
			return t.Error
		}
		return locales.FailedStatus()
	case download.StatusQueued:
		if t.Size > 0 {
			return locales.Queued() + " · " + formatBytes(t.Size)
		}
		return locales.Queued()
	case download.StatusPaused:
		return formatBytes(t.Downloaded) + " / " + sizeLabel(t.Size) + " · " + percentLabel(t) + " · " + locales.Paused()
	default:
		line := formatBytes(t.Downloaded) + " / " + sizeLabel(t.Size)
		if t.Size > 0 {
			line += " · " + percentLabel(t)
		}
		if t.Speed > 0 {
			line += " · " + formatBytes(t.Speed) + "/s"
			if t.Size > 0 {
				remaining := t.Remaining()
				if remaining > 0 {
					eta := remaining / t.Speed
					line += " · " + locales.Remaining() + " " + formatDuration(eta)
				}
			}
		}
		if t.Connections > 0 {
			line += fmt.Sprintf(" · %d %s", t.Connections, locales.ConnectionCount())
		}
		return line
	}
}

func percentLabel(t download.Task) string {
	if t.Size <= 0 {
		return ""
	}
	return fmt.Sprintf("%.0f%%", t.Percent()*100)
}

func sizeLabel(size int64) string {
	if size < 0 {
		return locales.UnknownSize()
	}
	return formatBytes(size)
}

func formatBytes(n int64) string {
	if n < 0 {
		return locales.UnknownSize()
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	for _, unit := range units {
		value /= 1024
		if value < 1024 {
			if value >= 100 {
				return fmt.Sprintf("%.0f %s", value, unit)
			}
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f PB", value/1024)
}

func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %02ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %02dm", seconds/3600, (seconds%3600)/60)
}
