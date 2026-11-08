package app

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/locales"
)

// The menu bar panel: a flyout hanging from the tray icon, as wide as the
// chunk map it draws. Its height comes from the number of downloads it
// lists, since native UI cannot size a flyout to its content.
const (
	panelWidth   = 314
	panelRadius  = 12
	panelPad     = 14
	panelCell    = 10
	panelCellGap = 2
	panelGap     = 10
	panelTaskGap = 12
	panelMaxRows = 3
)

// panelHeight is the height the panel needs to list n downloads.
func panelHeight(n int) int {
	// Padding, the gaps of the column (its lines, the spacer that takes the
	// slack, the divider, the buttons), the divider and the buttons, plus a
	// little slack of its own: a line that lays out taller than the sum of
	// its parts then pushes nothing out of the flyout.
	height := panelPad*2 + 2*panelGap + 1 + 30 + 10
	if n = min(n, panelMaxRows); n == 0 {
		// One line that says nothing is running.
		return height + 15
	}
	// The header above the downloads, and the downloads themselves.
	body := n*panelRowHeight() + (n-1)*panelTaskGap
	return height + 16 + 2*panelGap + body
}

// panelRowHeight is the height of one download: its name, its chunk map and
// the line under them, with the gaps between.
func panelRowHeight() int {
	heat := chunkRows*panelCell + (chunkRows-1)*panelCellGap
	return 16 + 6 + heat + 6 + 14
}

// unfinished reports whether the download is still on its way, paused ones
// included.
func unfinished(t download.Task) bool {
	switch t.Status {
	case download.StatusDownloading, download.StatusQueued, download.StatusPaused:
		return true
	}
	return false
}

// panelTasks returns the downloads the panel lists, at most panelMaxRows of
// them: the header counts the rest.
func panelTasks(tasks []download.Task) []download.Task {
	out := make([]download.Task, 0, panelMaxRows)
	for _, t := range tasks {
		if !unfinished(t) {
			continue
		}
		out = append(out, t)
		if len(out) == panelMaxRows {
			break
		}
	}
	return out
}

// panelTotal counts every download the panel would list, had it the room.
func panelTotal(tasks []download.Task) int {
	n := 0
	for _, t := range tasks {
		if unfinished(t) {
			n++
		}
	}
	return n
}

// togglePanel opens the panel next to the tray icon, or closes it.
func (a *App) togglePanel() {
	a.mu.Lock()
	panel := a.panel
	a.mu.Unlock()
	if panel != nil {
		panel.Close()
		return
	}
	a.openPanel()
}

func (a *App) openPanel() {
	a.refresh()
	rows := len(panelTasks(a.tasks))

	a.mu.Lock()
	tray := a.tray
	a.panelRows = rows
	a.mu.Unlock()
	if tray == nil {
		return
	}

	panel := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:         tray,
		Placement:    mygo.PlacementBottom,
		Gap:          5,
		Width:        panelWidth,
		Height:       panelHeight(rows),
		Focusable:    true,
		Shadow:       true,
		CornerRadius: panelRadius,
		Content:      ui.View(a.panelView),
	})
	a.mu.Lock()
	a.panel = panel
	a.mu.Unlock()
	panel.OnClosed(func() {
		a.mu.Lock()
		if a.panel == panel {
			a.panel = nil
		}
		a.mu.Unlock()
	})
}

func (a *App) closePanel() {
	a.mu.Lock()
	panel := a.panel
	a.mu.Unlock()
	if panel != nil {
		panel.Close()
	}
}

// panelView draws the panel: the downloads still on their way, each with the
// map of the bytes it has, and the buttons that act on them.
func (a *App) panelView(c *ui.Context) {
	a.refresh()
	c.SetTheme(segraTheme())

	// The flyout has no frame of its own: the panel draws all of it, its
	// shape rounded as the system rounds a popover.
	c.Root().Background(ui.Transparent)
	box := ui.Column(c).Fill().Background(colCard)
	switch runtime.GOOS {
	case "darwin":
		// The system's shadow follows the shape and outlines it.
		box = box.Radius(panelRadius)
	case "windows":
		// Windows rounds and outlines the flyout as it rounds its menus.
	default:
		box = box.Radius(panelRadius).Border(1, colEdge)
	}

	tasks := panelTasks(a.tasks)
	box.Padding(panelPad).Gap(panelGap).Children(func() {
		if len(tasks) == 0 {
			ui.Text(c, locales.PanelIdle()).FontSize(12).TextColor(colMuted).SingleLine().Ellipsis("…")
		} else {
			a.panelHeader(c, tasks, panelTotal(a.tasks))
			ui.Column(c).Gap(panelTaskGap).Children(func() {
				for _, t := range tasks {
					a.panelRow(c, t)
				}
			})
		}
		ui.Spacer(c)
		ui.Box(c).Height(1).Background(colLine)
		a.panelActions(c, panelRunning(tasks))
	})
}

// panelRunning reports whether any of the downloads the panel lists is on its
// way, rather than paused.
func panelRunning(tasks []download.Task) bool {
	for _, t := range tasks {
		if t.Status != download.StatusPaused {
			return true
		}
	}
	return false
}

// panelHeader says what the downloads are doing, and how fast.
func (a *App) panelHeader(c *ui.Context, tasks []download.Task, total int) {
	state := locales.Downloading()
	if !panelRunning(tasks) {
		state = locales.Paused()
	}
	label := fmt.Sprintf("%s · %d", state, total)
	var speed int64
	for _, t := range tasks {
		if t.Status == download.StatusDownloading {
			speed += t.Speed
		}
	}
	ui.Row(c).Height(16).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, label).FontSize(11.5).FontWeight(600).TextColor(colLabel).Grow(1).SingleLine().Ellipsis("…")
		if speed > 0 {
			value, unit := splitSpeed(speed)
			ui.Text(c, value+" "+unit).FontSize(11.5).FontFeatures("tnum").TextColor(colGreen).SingleLine()
		}
	})
}

// panelRow is one download: its name, the map of its bytes and the line under
// them. Choosing it brings the window up on that download.
func (a *App) panelRow(c *ui.Context, t download.Task) {
	paused := t.Status == download.StatusPaused
	ui.Column(c).Gap(6).OnClick(func() { a.focusTask(t.ID) }).Children(func() {
		ui.Row(c).Height(16).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, t.FileName).FontSize(12.5).TextColor(colText).SingleLine().Ellipsis("…").Grow(1)
			ui.Text(c, percentLabel(t)).FontSize(11.5).FontFeatures("tnum").TextColor(progressColor(t)).SingleLine()
		})
		a.panelHeat(c, t, paused)
		ui.Row(c).Height(14).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, panelMeta(t)).FontSize(11).TextColor(colMuted).SingleLine().Ellipsis("…").Grow(1)
			if t.Speed > 0 {
				value, unit := splitSpeed(t.Speed)
				ui.Text(c, value+" "+unit).FontSize(11).FontFeatures("tnum").TextColor(colFaint).SingleLine()
			}
		})
	})
}

// panelHeat draws the chunk map of one download, the grid the inspector
// draws, sized for the panel.
func (a *App) panelHeat(c *ui.Context, t download.Task, paused bool) {
	cells := chunkBlocks(t)
	if len(cells) == 0 {
		// A queued download has nothing on disk yet: draw the empty grid all
		// the same, so the row keeps its height.
		cells = make([]blockState, chunkRows*chunkCols)
	}
	ui.Column(c).Gap(panelCellGap).Children(func() {
		for row := 0; row < chunkRows; row++ {
			line := cells[row*chunkCols : (row+1)*chunkCols]
			ui.Row(c).Gap(panelCellGap).Children(func() {
				for _, state := range line {
					ui.Box(c).Width(panelCell).Height(panelCell).Radius(2).
						Background(blockColor(state, paused))
				}
			})
		}
	})
}

// panelMeta is the line under a chunk map: the bytes of the file on disk.
func panelMeta(t download.Task) string {
	if t.Size > 0 {
		return fmt.Sprintf("%s / %s", formatBytes(t.Downloaded), formatBytes(t.Size))
	}
	return formatBytes(t.Downloaded)
}

func (a *App) panelActions(c *ui.Context, running bool) {
	ui.Row(c).Height(28).AlignItems(ui.Center).Gap(8).Children(func() {
		pause := a.panelButton(c, locales.PauseAll(), false, func() { a.mgr.PauseAll() })
		if !running {
			pause.Disabled(true).Opacity(0.5)
		}
		a.panelButton(c, locales.ResumeAll(), false, func() { a.mgr.ResumeAll() })
		ui.Spacer(c)
		a.panelButton(c, locales.TrayOpen(), true, func() {
			a.showWindow()
			a.closePanel()
		})
	})
}

// panelButton is a button of the panel's footer: a pill with tighter padding
// than a window's, so that three of them fit the width of the panel.
func (a *App) panelButton(c *ui.Context, label string, primary bool, fn func()) ui.Element {
	return a.pillButton(c, label, nil, primary, fn).Height(28).Radius(14).Padding(0, 10)
}

// focusTask brings the window up on the download the user picked.
func (a *App) focusTask(id string) {
	a.view = "all"
	a.refresh()
	for i, t := range a.rows {
		if t.ID == id {
			a.selected = i
			break
		}
	}
	a.showWindow()
	a.invalidate()
	a.closePanel()
}

// trayPercent is how far the downloads are, as the menu bar shows it: the
// share of the bytes of all of them that is on disk. The digits are padded
// with figure spaces, which are as wide as a digit, so the item keeps its
// width as the number changes.
func trayPercent(tasks []download.Task) string {
	var size, done int64
	for _, t := range tasks {
		switch t.Status {
		case download.StatusDownloading, download.StatusQueued, download.StatusPaused:
		default:
			continue
		}
		if t.Size > 0 {
			size += t.Size
			done += t.Downloaded
		}
	}
	if size <= 0 {
		return ""
	}
	text := fmt.Sprintf("%3.0f%%", float64(done)/float64(size)*100)
	return strings.ReplaceAll(text, " ", "\u2007")
}

// trayTip lists the downloads the panel would list, for the hover text of the
// icon.
func trayTip(tasks []download.Task) string {
	tip := "Segra"
	rows := panelTasks(tasks)
	if len(rows) == 0 {
		return tip
	}
	for _, t := range rows {
		tip += "\n" + t.FileName + " " + percentLabel(t)
	}
	return tip
}

// updateTrayStatus puts how far the downloads are next to the icon, and the
// downloads themselves in its hover text. While the panel shows, it follows
// the same snapshot, and grows or shrinks by a row as downloads start and
// finish. It runs off the main thread, once a second.
func (a *App) updateTrayStatus(tasks []download.Task) {
	title, tip := trayPercent(tasks), trayTip(tasks)

	a.mu.Lock()
	tray, panel := a.tray, a.panel
	changed := title != a.trayTitle || tip != a.trayTip
	a.trayTitle, a.trayTip = title, tip
	rows := len(panelTasks(tasks))
	resize := panel != nil && rows != a.panelRows
	if resize {
		a.panelRows = rows
	}
	a.mu.Unlock()

	if tray != nil && changed {
		tray.SetTitle(title)
		tray.SetToolTip(tip)
	}
	if panel == nil {
		return
	}
	if resize {
		panel.SetContentSize(panelWidth, panelHeight(rows))
	}
	panel.Invalidate()
}
