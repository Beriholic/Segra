package app

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/locales"
)

// viewFn draws the window. It runs every frame; all state lives on app.
func (a *App) viewFn(c *ui.Context) {
	a.refresh()
	c.SetTheme(segraTheme())

	// The window draws no title bar of its own: the strip at the top holds
	// the traffic lights and drags, and everything else starts at the very
	// top of the window.
	tb := c.TitleBar()
	strip := tb.Height
	if strip < 28 {
		strip = 28
	}

	ui.Column(c).Fill().Background(colBG).Children(func() {
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c, strip)
			a.content(c, strip)
			if a.inspectorVisible(c) {
				a.inspector(c, strip)
			}
		})
		a.statusBar(c)
		a.newDialog(c)
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(strip).DragWindow()
	})
}

func (a *App) sidebar(c *ui.Context, strip float32) {
	ui.Column(c).Width(218).FillHeight().Background(colSide).Children(func() {
		ui.Box(c).Absolute().Right(0).Top(0).Bottom(0).Width(1).Background(colEdge)
		ui.Column(c).Grow(1).Padding(strip+12, 12, 4, 12).Children(func() {
			ui.SearchField(c, &a.search).Placeholder(locales.Search()).Height(32).
				Background(ui.RGB(0xFF, 0xFF, 0xFF)).Border(1, colEdge).Radius(10)
			ui.Box(c).Height(18)

			a.sectionLabel(c, locales.Downloads())
			ui.Box(c).Height(2)
			a.navItem(c, "all", icGrid, locales.All(), len(a.tasks))
			a.navItem(c, "active", icActive, locales.Active(), a.countStatus(download.StatusDownloading, download.StatusQueued))
			a.navItem(c, "completed", icCheck, locales.Completed(), a.countStatus(download.StatusCompleted))
			a.navItem(c, "failed", icFailed, locales.Failed(), a.countStatus(download.StatusFailed))

			ui.Box(c).Height(16)
			a.sectionLabel(c, locales.Settings())
			ui.Box(c).Height(2)
			a.navItem(c, "settings", icSettings, locales.General(), 0)
			ui.Spacer(c)
		})
	})
}

func (a *App) navItem(c *ui.Context, id string, icon *ui.SVG, label string, count int) {
	selected := a.view == id
	item := ui.Row(c).Height(32).Margin(0, 0, 3, 0).Padding(0, 10).Gap(9).AlignItems(ui.Center).Radius(10)
	if selected {
		item = item.Background(colSel)
	} else if item.Hovered() {
		item = item.Background(ui.RGBA(255, 255, 255, 0.55))
	}
	item.OnClick(func() {
		a.view = id
		a.selected = -1
		a.refresh()
	})
	item.Children(func() {
		iconColor, textColor := colMuted, colSub
		if selected {
			iconColor, textColor = colGreen, colGreen
		}
		if selected {
			ui.Box(c).Absolute().Left(0).Top(8).Bottom(8).Width(3).Radius(2).Background(colGreen)
		}
		ui.Icon(c, icon).FontSize(13.5).TextColor(iconColor)
		text := ui.Text(c, label).FontSize(13).SingleLine().Grow(1).TextColor(textColor)
		if selected {
			text.Bold()
		}
		if count > 0 {
			ui.Box(c).Padding(1, 6).Radius(6).Background(colPill).Children(func() {
				countColor := colMuted
				if selected {
					countColor = colGreen
				}
				ui.Text(c, fmt.Sprint(count)).FontSize(10.5).FontFeatures("tnum").TextColor(countColor)
			})
		}
	})
}

func (a *App) countStatus(statuses ...download.Status) int {
	n := 0
	for _, t := range a.tasks {
		for _, s := range statuses {
			if t.Status == s {
				n++
				break
			}
		}
	}
	return n
}

func (a *App) content(c *ui.Context, strip float32) {
	ui.Column(c).Grow(1).FillHeight().Padding(strip+8, 22, 0, 24).Children(func() {
		if a.view == "settings" {
			a.settingsView(c)
			return
		}
		a.mainHeader(c)
		if len(a.rows) == 0 {
			a.emptyState(c)
			return
		}
		list := ui.List(c, &a.list, len(a.rows), func(i int) { a.row(c, i) }).Grow(1)
		if list.Submitted() && a.selected >= 0 && a.selected < len(a.rows) {
			t := a.rows[a.selected]
			if t.Status == download.StatusCompleted {
				mygo.Shell.OpenPath(t.Path())
			}
		}
	})
}

func (a *App) mainHeader(c *ui.Context) {
	active := a.mgr.ActiveCount()
	ui.Row(c).AlignItems(ui.Center).Gap(10).Padding(0, 0, 16, 0).Children(func() {
		ui.Text(c, locales.Downloads()).FontSize(26).Bold().TextColor(colText).Grow(1)
		pause := a.secondaryButton(c, locales.PauseAll(), func() { a.mgr.PauseAll() }).Height(34).Radius(17).Disabled(active == 0)
		if active == 0 {
			pause.Opacity(0.5)
		}
		a.primaryButton(c, locales.NewDownload(), a.openNew).Height(34).Radius(17)
	})
}

func (a *App) emptyState(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Gap(9).Children(func() {
		ui.Box(c).Size(56, 56).Radius(18).Center().Background(colSide).Children(func() {
			ui.Icon(c, icDownload).FontSize(24).TextColor(colGreen)
		})
		ui.Box(c).Height(4)
		ui.Text(c, locales.NoTasks()).FontSize(16).Bold().TextColor(colText)
		ui.Text(c, locales.NoTasksHint()).FontSize(12.5).TextColor(colMuted).TextAlign(ui.Center).MaxWidth(340)
		ui.Box(c).Height(4)
		a.primaryButton(c, locales.NewDownload(), a.openNew)
	})
}

func (a *App) statusBar(c *ui.Context) {
	ui.Row(c).Height(30).Padding(0, 16).Gap(12).AlignItems(ui.Center).Background(colBar).Children(func() {
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(1).Background(colEdge)
		ui.Text(c, fmt.Sprintf("%d %s", len(a.tasks), locales.Files())).FontSize(11).TextColor(colSub).SingleLine().Grow(1)
		if a.apiErr != "" {
			ui.Text(c, locales.ApiRunning()+": "+a.apiErr).FontSize(11).TextColor(colDanger).SingleLine()
		}
		ui.Text(c, fmt.Sprintf("%d %s", a.mgr.ActiveCount(), locales.Active())).FontSize(11).TextColor(colSub)
	})
}
