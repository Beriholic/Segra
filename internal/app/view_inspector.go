package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/locales"
)

func (a *App) inspectorVisible(c *ui.Context) bool {
	w, _ := c.Size()
	return w >= 1060 && a.view != "settings" && len(a.rows) > 0
}

func (a *App) inspector(c *ui.Context, strip float32) {
	var t *download.Task
	if a.selected >= 0 && a.selected < len(a.rows) {
		t = &a.rows[a.selected]
	}

	// The panel folds down to a strip that holds its button with the same
	// margin as the open panel's header, and the button stays pinned to the
	// window's right edge in both states.
	target := float32(304)
	if a.detailsCollapsed {
		target = 66
	}
	panel := ui.Column(c).FillHeight().Background(colSurface).Clip()
	width := panel.Animate("details-width", target, 190*time.Millisecond)
	panel.Width(width).Children(func() {
		ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).Width(1).Background(colLineSoft)

		// The body keeps its width while the panel folds, so it slides out of
		// view instead of reflowing, and is drawn only while there is room.
		if width > 200 {
			ui.Column(c).Width(304).Grow(1).Children(func() {
				ui.Row(c).Padding(0, 60, 0, 20).Height(strip + 30).AlignItems(ui.Center).Children(func() {
					ui.Text(c, locales.TaskDetails()).FontSize(14).Bold().SingleLine().TextColor(colText)
				})
				if t == nil {
					ui.Spacer(c)
					return
				}
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Padding(0, 20, 18, 20).Gap(20).Children(func() {
						a.inspectorFile(c, *t)
						a.inspectorProgress(c, *t)
						a.inspectorStats(c, *t)
						if t.Status == download.StatusDownloading || t.Status == download.StatusPaused {
							a.inspectorSpark(c)
							a.inspectorConnections(c, *t)
						}
						a.inspectorMeta(c, *t)
						a.inspectorEvents(c, *t)
					})
				})
			})
		}

		icon, tip, collapse := icChevronR, locales.CollapseDetails(), true
		if a.detailsCollapsed {
			icon, tip, collapse = icChevronL, locales.ExpandDetails(), false
		}
		a.iconButton(c, icon, tip, func() { a.setDetailsCollapsed(collapse) }).
			Absolute().Right(20).Top(strip + 2)
	})
}

func (a *App) setDetailsCollapsed(collapsed bool) {
	a.detailsCollapsed = collapsed
	a.invalidate()
}

func (a *App) inspectorFile(c *ui.Context, t download.Task) {
	ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
		a.fileTile(c, t.FileName, 52)
		ui.Column(c).Gap(6).Grow(1).Children(func() {
			ui.Text(c, t.FileName).FontSize(15).Bold().MaxLines(2).TextColor(colText)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				a.chip(c, typeLabel(t.FileName))
				if t.Connections > 0 {
					a.chip(c, fmt.Sprintf("%d %s", t.Connections, locales.ConnectionCount()))
				}
			})
		})
	})
}

func (a *App) inspectorProgress(c *ui.Context, t download.Task) {
	pct := t.Percent()
	done := t.Status == download.StatusCompleted
	if done {
		pct = 1
	}
	ui.Column(c).Gap(10).Children(func() {
		ui.Row(c).AlignItems(ui.Baseline).Gap(8).Children(func() {
			ui.Text(c, fmt.Sprintf("%.0f%%", pct*100)).FontSize(36).Bold().FontFeatures("tnum").TextColor(colText)
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("%s / %s", formatBytes(t.Downloaded), sizeLabel(t.Size))).FontSize(12).TextColor(colMuted)
		})
		if !done {
			ui.Box(c).Height(6).Radius(3).Background(colLineSoft).Children(func() {
				ui.Box(c).Height(6).Radius(3).WidthPercent(float32(pct * 100)).Background(progressColor(t))
			})
		}
	})
}

func (a *App) inspectorStats(c *ui.Context, t download.Task) {
	value, unit := splitSpeed(t.Speed)
	cell := func(label, value, unit string) {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(colLabel).SingleLine()
			ui.Row(c).Gap(3).AlignItems(ui.Baseline).Children(func() {
				ui.Text(c, value).FontSize(16).Bold().FontFeatures("tnum").TextColor(colText)
				if unit != "" {
					ui.Text(c, unit).FontSize(11).TextColor(colMuted)
				}
			})
		})
	}
	ui.Column(c).Gap(16).Children(func() {
		ui.Row(c).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).Gap(16).Children(func() {
				cell(locales.Connections(), fmt.Sprint(t.Connections), "")
				cell(locales.Remaining(), etaValue(t), "")
			})
			ui.Box(c).Width(1).Background(colLineSoft)
			ui.Box(c).Width(16)
			ui.Column(c).Grow(1).Gap(16).Children(func() {
				if t.Status == download.StatusDownloading {
					cell(locales.Status(), value, unit)
				} else {
					cell(locales.Status(), statusLabel(t), "")
				}
				cell(locales.Type(), typeLabel(t.FileName), "")
			})
		})
		ui.Box(c).Height(1).FillWidth().Background(colLineSoft)
	})
}

func (a *App) inspectorSpark(c *ui.Context) {
	peak := 0.0
	for _, v := range a.speedSamples {
		if v > peak {
			peak = v
		}
	}
	if peak < 1 {
		peak = 1
	}
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SpeedLastMinute()).FontSize(11.5).TextColor(colLabel).Grow(1)
			ui.Text(c, "↑ "+formatBytes(int64(peak))+"/s").FontSize(11).TextColor(colMuted)
		})
		ui.Box(c).Height(40).FillWidth().Draw(func(p *ui.Painter, r ui.Rect) {
			samples := a.speedSamples
			if len(samples) < 2 {
				return
			}
			n := len(samples)
			step := r.W / float32(n-1)
			barW := step
			if barW > 4 {
				barW = 4
			}
			if barW < 1 {
				barW = 1
			}
			for i, v := range samples {
				h := float32(v/peak) * (r.H - 4)
				if h < 1 {
					h = 1
				}
				x := r.X + float32(i)*step
				p.Fill(ui.Rect{X: x, Y: r.Y + r.H - h, W: barW, H: h}, ui.RGBA(0x1C, 0x1C, 0x1E, 0.08), 1)
			}
			for i := 1; i < n; i++ {
				y0 := r.Y + r.H - float32(samples[i-1]/peak)*(r.H-4)
				y1 := r.Y + r.H - float32(samples[i]/peak)*(r.H-4)
				p.Line(r.X+float32(i-1)*step, y0, r.X+float32(i)*step, y1, 1.4, colGreen)
			}
		})
	})
}

func (a *App) inspectorConnections(c *ui.Context, t download.Task) {
	active := t.Status == download.StatusDownloading
	n := t.Connections
	if n <= 0 {
		n = 1
	}
	if n > 32 {
		n = 32
	}
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ConnectionHealth()).FontSize(11.5).TextColor(colLabel).Grow(1)
			ui.Text(c, fmt.Sprintf("%d / %d", activeCount(n, active), n)).FontSize(11).TextColor(colMuted)
		})
		ui.Row(c).Gap(2).AlignItems(ui.End).Height(26).Children(func() {
			for i := 0; i < n; i++ {
				h := float32(9 + (i*7)%17)
				color := ui.RGBA(0x1C, 0x1C, 0x1E, 0.16)
				if active && i%5 != 3 {
					color = ui.RGBA(0x2F, 0x7D, 0x57, 0.8)
					h = float32(14 + (i*11)%12)
				}
				ui.Box(c).Width(4).Height(h).Radius(1).Background(color)
			}
		})
	})
}

func (a *App) inspectorMeta(c *ui.Context, t download.Task) {
	row := func(label, value string) {
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(colLabel).Width(48).SingleLine()
			ui.Text(c, value).FontSize(12.5).TextColor(colMuted).Grow(1).SingleLine().Ellipsis("…").Selectable()
		})
	}
	ui.Column(c).Gap(8).Children(func() {
		row(locales.Source(), hostOf(t.URL))
		if t.ETag != "" {
			row(locales.Checksum(), strings.Trim(t.ETag, `"`))
		}
		row(locales.Shards(), fmt.Sprintf("%d", t.Connections))
	})
}

func (a *App) inspectorEvents(c *ui.Context, t download.Task) {
	ui.Column(c).Gap(7).Children(func() {
		ui.Text(c, locales.Events()).FontSize(11.5).TextColor(colLabel)
		event := func(when, text string, color ui.Color) {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, when).FontSize(11).TextColor(colFaint).Width(46)
				ui.Text(c, text).FontSize(11.5).TextColor(color).SingleLine().Ellipsis("…").Grow(1)
			})
		}
		event(t.CreatedAt.Format("15:04"), locales.Queued(), colMuted)
		if !t.StartedAt.IsZero() {
			event(t.StartedAt.Format("15:04"), statusLabel(t), statusColor(t))
		}
		if t.Status == download.StatusDownloading || t.Status == download.StatusPaused || t.Status == download.StatusQueued {
			now := fmt.Sprintf("%d %s · %s/s", t.Connections, locales.ConnectionCount(), formatBytes(t.Speed))
			if t.Status == download.StatusQueued {
				now = locales.Queued()
			}
			event("now", now, colMuted)
		}
	})
}
