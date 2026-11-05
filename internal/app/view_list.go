package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/locales"
)

func (a *App) row(c *ui.Context, i int) {
	t := a.rows[i]
	selected := i == a.selected
	ui.Box(c).Height(79).Padding(0, 0, 13, 0).Children(func() {
		card := ui.Box(c).Height(65).Radius(10).FillWidth().
			Background(colCard).Border(1, colLine).
			Shadow(0, 1, 2, 0, ui.RGBA(28, 28, 30, 0.04)).
			FocusRing(false)
		if card.Hovered() {
			card = card.Background(ui.RGB(0xFF, 0xFF, 0xFF)).Shadow(0, 2, 6, 0, ui.RGBA(28, 28, 30, 0.06))
		}
		if selected {
			card = card.Shadow(0, 2, 8, 0, ui.RGBA(28, 28, 30, 0.08))
		}
		card.ContextMenu(func(m *ui.Menu) { a.rowMenu(m, t) })
		card.OnClick(func() { a.selected = i })
		if card.DoubleClicked() {
			a.openTask(t)
		}
		card.Children(func() {
			if selected {
				ui.Box(c).Absolute().Left(-1).Top(9).Bottom(9).Width(4).Radius(2).Background(colGreen)
			}
			ui.Row(c).FillHeight().Padding(9, 12, 9, 13).Gap(12).AlignItems(ui.Center).Children(func() {
				a.fileTile(c, t.FileName, 34)
				ui.Column(c).Gap(3).Grow(1).Children(func() {
					ui.Text(c, t.FileName).FontSize(14).Bold().SingleLine().Ellipsis("…").TextColor(colText)
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(5, 5).Radius(3).Background(statusColor(t))
						ui.Text(c, statusLabel(t)).FontSize(11.5).TextColor(statusColor(t)).SingleLine()
						ui.Text(c, rowMeta(t)).FontSize(11.5).TextColor(colMuted).SingleLine().Ellipsis("…").Grow(1)
					})
				})
				a.rowStat(c, t)
				a.rowAction(c, t)
			})
		})
	})
}

func (a *App) fileTile(c *ui.Context, name string, size float32) {
	ui.Box(c).Size(size, size).Radius(9).Center().Background(ui.RGB(0xF1, 0xEF, 0xE9)).Border(1, colLine).Children(func() {
		ui.Icon(c, iconForName(name)).FontSize(size * 0.48).TextColor(ui.RGB(0x55, 0x55, 0x5C))
	})
}

func (a *App) rowStat(c *ui.Context, t download.Task) {
	ui.Column(c).Gap(1).AlignItems(ui.End).Width(86).Children(func() {
		switch t.Status {
		case download.StatusDownloading:
			value, unit := splitSpeed(t.Speed)
			ui.Row(c).Gap(3).AlignItems(ui.Baseline).Children(func() {
				ui.Text(c, value).FontSize(14).Bold().FontFeatures("tnum").TextColor(colText)
				ui.Text(c, unit).FontSize(9.5).TextColor(colMuted)
			})
			ui.Text(c, etaLabel(t)).FontSize(10.5).TextColor(colMuted).SingleLine()
		case download.StatusCompleted:
			ui.Text(c, locales.CompletedStatus()).FontSize(12).Bold().TextColor(colGreen)
			ui.Text(c, formatBytes(t.Size)).FontSize(11).TextColor(colMuted).SingleLine()
		case download.StatusPaused:
			ui.Text(c, percentLabel(t)).FontSize(13).Bold().FontFeatures("tnum").TextColor(colWarn)
			ui.Text(c, locales.Paused()).FontSize(10.5).TextColor(colMuted)
		case download.StatusFailed:
			ui.Text(c, "✕").FontSize(13).Bold().TextColor(colDanger)
			ui.Text(c, locales.FailedStatus()).FontSize(10.5).TextColor(colMuted)
		default:
			ui.Text(c, "—").FontSize(12).TextColor(colMuted)
			ui.Text(c, locales.Queued()).FontSize(10.5).TextColor(colMuted)
		}
	})
}

func (a *App) rowAction(c *ui.Context, t download.Task) {
	var icon *ui.SVG
	var fn func()
	var tip string
	switch t.Status {
	case download.StatusCompleted:
		icon, fn, tip = icFolder, func() { a.reveal(t) }, locales.ShowInFolder()
	case download.StatusDownloading, download.StatusQueued:
		icon, fn, tip = icPause, func() { a.toggle(t) }, locales.Pause()
	default:
		icon, fn, tip = icPlay, func() { a.toggle(t) }, locales.Resume()
	}
	btn := ui.Box(c).Size(26, 26).Radius(8).AlignItems(ui.Center).Justify(ui.Center)
	if btn.Hovered() {
		btn = btn.Background(ui.RGBA(28, 28, 30, 0.06))
	}
	btn.Tooltip(tip).OnClick(fn).Children(func() {
		ui.Icon(c, icon).FontSize(15).TextColor(colMuted)
	})
}

func (a *App) rowMenu(m *ui.Menu, t download.Task) {
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
