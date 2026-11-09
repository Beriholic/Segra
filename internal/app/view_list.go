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

	// A row slides in when its download first shows, and the rows around it
	// follow with the same spring when the list reflows. A row that leaves
	// goes at once: the list is virtualized, so an exit copy would also fly
	// away at the edge every time a row scrolled out of view.
	tr := ui.ElementTransition{Animation: &springSoft}
	if a.fresh(t.ID, freshFor) {
		tr.Enter = fadeIn(14)
	}
	ui.Box(c).Key(t.ID).Height(79).Padding(0, 0, 13, 0).Transition(tr).Children(func() {
		card := ui.Box(c).Height(65).Radius(10).FillWidth().
			Background(a.pal.card).Border(1, a.pal.line).
			Shadow(0, 1, 2, 0, a.pal.shadowSoft).
			FocusRing(false)
		switch {
		case card.Hovered():
			card = card.Background(a.pal.cardHover).Shadow(0, 2, 6, 0, a.pal.shadowHover)
		case selected:
			card = card.Background(a.pal.cardSel)
		}
		if selected {
			card = card.Shadow(0, 2, 8, 0, a.pal.shadowSel)
		}
		// The card's colours fade as the pointer comes and goes, and it
		// lifts by a point: a pose, so the hover costs no rebuild.
		card.Transition(hoverColors)
		lift := ui.Pose{}
		if card.Hovered() {
			lift = ui.Pose{Y: -1, Scale: 1.004}
		}
		card.AnimatePose(lift, springSnap)
		card.Draw(func(p *ui.Painter, r ui.Rect) { a.drawLanded(p, r, t.ID) })
		card.ContextMenu(func(m *ui.Menu) { a.rowMenu(m, t) })
		card.OnClick(func() { a.selected = i })
		if card.DoubleClicked() {
			a.openTask(t)
		}
		card.Children(func() {
			// The mark of the chosen row stays in the tree, so that it fades
			// in and out instead of blinking.
			mark := float32(0)
			if selected {
				mark = 1
			}
			ui.Box(c).Absolute().Left(-1).Top(9).Bottom(9).Width(4).Radius(2).
				Background(a.pal.green).
				AnimatePose(ui.Pose{Fade: 1 - mark}, fadeCurve)
			ui.Row(c).FillHeight().Padding(9, 12, 9, 13).Gap(12).AlignItems(ui.Center).Children(func() {
				a.fileTile(c, t.FileName, 34)
				ui.Column(c).Gap(3).Grow(1).Children(func() {
					ui.Text(c, t.FileName).FontSize(14).Bold().SingleLine().Ellipsis("…").TextColor(a.pal.text)
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(5, 5).Radius(3).Background(statusColor(a.pal, t)).
							Transition(ui.ElementTransition{Colors: true, Duration: motionBase})
						ui.Text(c, statusLabel(t)).FontSize(11.5).TextColor(statusColor(a.pal, t)).SingleLine().
							ContentTransition(ui.ContentTransition{})
						ui.Text(c, rowMeta(t)).FontSize(11.5).TextColor(a.pal.muted).SingleLine().Ellipsis("…").Grow(1)
					})
				})
				a.rowStat(c, t)
				a.rowAction(c, t)
			})
		})
	})
}

// drawLanded rings a row whose download has just landed, the ring fading
// over the moment: the one flourish the list allows itself, and a frame that
// only paints.
func (a *App) drawLanded(p *ui.Painter, r ui.Rect, id string) {
	f := a.flash(id)
	if f <= 0 {
		return
	}
	p.Stroke(ui.Rect{X: r.X + 0.5, Y: r.Y + 0.5, W: r.W - 1, H: r.H - 1}, a.pal.green.Alpha(0.45*f), 10, 1.5)
	p.AnimationFrame()
}

func (a *App) fileTile(c *ui.Context, name string, size float32) {
	ui.Box(c).Size(size, size).Radius(9).Center().Background(a.pal.iconTile).Border(1, a.pal.line).Children(func() {
		ui.Icon(c, iconForName(name)).FontSize(size * 0.48).TextColor(a.pal.iconInk)
	})
}

func (a *App) rowStat(c *ui.Context, t download.Task) {
	ui.Column(c).Gap(1).AlignItems(ui.End).Width(86).Children(func() {
		switch t.Status {
		case download.StatusDownloading:
			value, unit := splitSpeed(t.Speed)
			ui.Row(c).Gap(3).AlignItems(ui.Baseline).Children(func() {
				ui.Text(c, value).FontSize(14).Bold().FontFeatures("tnum").TextColor(a.pal.text)
				ui.Text(c, unit).FontSize(9.5).TextColor(a.pal.muted)
			})
			ui.Text(c, etaLabel(t)).FontSize(10.5).TextColor(a.pal.muted).SingleLine()
		case download.StatusCompleted:
			ui.Text(c, locales.CompletedStatus()).FontSize(12).Bold().TextColor(a.pal.green)
			ui.Text(c, formatBytes(t.Size)).FontSize(11).TextColor(a.pal.muted).SingleLine()
		case download.StatusPaused:
			ui.Text(c, percentLabel(t)).FontSize(13).Bold().FontFeatures("tnum").TextColor(a.pal.warn)
			ui.Text(c, locales.Paused()).FontSize(10.5).TextColor(a.pal.muted)
		case download.StatusFailed:
			ui.Text(c, "✕").FontSize(13).Bold().TextColor(a.pal.danger)
			ui.Text(c, locales.FailedStatus()).FontSize(10.5).TextColor(a.pal.muted)
		default:
			ui.Text(c, "—").FontSize(12).TextColor(a.pal.muted)
			ui.Text(c, locales.Queued()).FontSize(10.5).TextColor(a.pal.muted)
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
		btn = btn.Background(a.pal.hover)
	}
	btn.Transition(hoverColors)
	btn.AnimatePose(pressPose(btn), springSnap)
	btn.Tooltip(tip).OnClick(fn).Children(func() {
		ui.Icon(c, icon).FontSize(15).TextColor(a.pal.muted)
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
