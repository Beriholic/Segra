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
						}
						a.inspectorShards(c, *t)
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
	// The peak is what the curve is scaled to, so it says how high the
	// chart goes; a minute without transfer has no height to name.
	peak := "—"
	if p := sparkPeak(a.sparkSeries()); p >= 1 {
		peak = "↑ " + formatBytes(int64(p)) + "/s"
	}
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SpeedLastMinute()).FontSize(11.5).TextColor(colLabel).Grow(1)
			ui.Text(c, peak).FontSize(11).TextColor(colMuted)
		})
		ui.Box(c).Height(46).FillWidth().Draw(func(p *ui.Painter, r ui.Rect) {
			a.drawSpark(p, r)
		})
	})
}

// sparkSeries returns the last minute of samples smoothed into a trend: a
// binomial average over five seconds takes the jitter of single samples out
// while keeping the shape of a real drop, so the curve drawn from it shows
// where the throughput is going instead of every hiccup on the way.
func (a *App) sparkSeries() []float64 {
	a.speedMu.Lock()
	if cap(a.sparkBuf) < len(a.speedSamples) {
		a.sparkBuf = make([]float64, 0, sparkSlots)
	}
	src := append(a.sparkBuf[:0], a.speedSamples...)
	a.sparkBuf = src
	a.speedMu.Unlock()
	if len(src) < 3 {
		return src
	}
	if cap(a.sparkSmooth) < len(src) {
		a.sparkSmooth = make([]float64, len(src))
	}
	out := a.sparkSmooth[:len(src)]
	kernel := [5]float64{1, 4, 6, 4, 1}
	for i := range src {
		var sum, weight float64
		for k, w := range kernel {
			j := i + k - 2
			if j < 0 {
				j = 0
			}
			if j >= len(src) {
				j = len(src) - 1
			}
			sum += src[j] * w
			weight += w
		}
		out[i] = sum / weight
	}
	return out
}

func sparkPeak(vals []float64) float64 {
	peak := 0.0
	for _, v := range vals {
		if v > peak {
			peak = v
		}
	}
	return peak
}

// sparkSlots is how many seconds the chart shows: one slot per sample of the
// minute, so that the newest sample stays on the right edge and the curve
// keeps its scale from the first second of a download instead of stretching
// over the whole width until the minute is full.
const sparkSlots = 60

// drawSpark draws the throughput as a smooth area chart: a curve through
// the samples and the room under it tinted down to the floor. Time without
// a transfer keeps only the floor, so an idle minute reads as a chart at
// rest rather than a green line stuck to the bottom.
func (a *App) drawSpark(p *ui.Painter, r ui.Rect) {
	vals := a.sparkSeries()
	if len(vals) < 2 {
		return
	}
	peak := max(sparkPeak(vals), 1)
	// Leave room for the stroke's round ends and for a spike to sit at the
	// very top without touching it.
	plot := ui.Rect{X: r.X + 1, Y: r.Y, W: r.W - 2, H: r.H}
	floor := plot.Y + plot.H - 1
	rise := plot.H - 5
	step := plot.W / float32(sparkSlots-1)
	lead := float32(sparkSlots-len(vals)) * step

	if cap(a.sparkPts) < len(vals) {
		a.sparkPts = make([][2]float32, len(vals))
	}
	pts := a.sparkPts[:len(vals)]
	for i, v := range vals {
		h := float32(v/peak) * rise
		if h < 0.6 {
			h = 0 // a stalled second sits on the floor, not above it
		}
		pts[i] = [2]float32{plot.X + lead + float32(i)*step, floor - h}
	}
	a.sparkPts = pts

	// The room under the curve, deepest where the transfer runs fastest.
	area := &ui.Path{}
	area.MoveTo(pts[0][0], floor)
	area.LineTo(pts[0][0], pts[0][1])
	sparkCurve(area, pts, 0, len(pts)-1, plot.Y, floor)
	area.LineTo(pts[len(pts)-1][0], floor)
	area.Close()
	p.FillPathGradient(area, ui.LinearGradient{
		From:  ui.RGBA(0x2F, 0x7D, 0x57, 0.20),
		To:    ui.RGBA(0x2F, 0x7D, 0x57, 0.00),
		Angle: 180,
	})

	// The floor the curve stands on, and the curve itself over the stretches
	// that carried bytes: a pause or a stall leaves the floor alone instead
	// of dragging the green line along it.
	p.Fill(ui.Rect{X: plot.X, Y: floor, W: plot.W, H: 1}, colLine, 0)
	active := func(i int) bool { return pts[i][1] < floor-0.5 }
	for i := 0; i < len(pts); {
		if !active(i) {
			i++
			continue
		}
		from := i
		for i < len(pts) && active(i) {
			i++
		}
		// Reach back to the floor on both sides, so the curve rises out of
		// it and settles back onto it.
		start, end := max(from-1, 0), min(i, len(pts)-1)
		curve := &ui.Path{}
		curve.MoveTo(pts[start][0], pts[start][1])
		sparkCurve(curve, pts, start, end, plot.Y, floor)
		p.StrokePath(curve, 1.6, colGreen)
	}
}

// sparkCurve appends a curve through pts[from..to], whose current point is
// pts[from], to path: the Catmull-Rom spline through the samples, as cubic
// Bézier segments. Control points are held between top and floor, so that a
// spike pulls the curve to the edge it belongs to and never past it.
func sparkCurve(path *ui.Path, pts [][2]float32, from, to int, top, floor float32) {
	n := len(pts)
	at := func(i int) [2]float32 {
		if i < 0 {
			return pts[0]
		}
		if i >= n {
			return pts[n-1]
		}
		return pts[i]
	}
	clamp := func(v float32) float32 {
		if v < top {
			return top
		}
		if v > floor {
			return floor
		}
		return v
	}
	for i := from; i < to; i++ {
		prev, here, next, after := at(i-1), pts[i], pts[i+1], at(i+2)
		path.CubeTo(
			here[0]+(next[0]-prev[0])/6, clamp(here[1]+(next[1]-prev[1])/6),
			next[0]-(after[0]-here[0])/6, clamp(next[1]-(after[1]-here[1])/6),
			next[0], next[1],
		)
	}
}

// The chunk map: 24 cells of 8 points with a 3 point gap fill the 261 the
// panel gives, so the grid runs edge to edge. Four rows keep the cells
// square, the way Surge's TUI chunk map does.
const (
	chunkRows = 4
	chunkCols = 24
	chunkSize = 8
	chunkGap  = 3
)

// blockState is one patch of the file.
type blockState uint8

const (
	blockPending blockState = iota // no byte of this patch is on disk
	blockActive                    // some are, and the rest are on their way
	blockDone                      // every byte of this patch is on disk
)

// chunkBlocks paints the file as rows*cols cells in byte order, each cell
// saying how much of the bytes it covers is on disk. The cells a segment
// overlaps add up to that share, so a cell is green once every byte it
// covers has landed, amber while it straddles a segment being written, and
// empty before that. This is the reading Surge's TUI chunk map gives: one
// frontier over the whole file instead of one bar per connection.
//
// It returns nil when nothing is known yet, so the panel stays out of the way.
func chunkBlocks(t download.Task) []blockState {
	if len(t.Segments) == 0 && t.Status != download.StatusCompleted {
		return nil
	}
	out := make([]blockState, chunkRows*chunkCols)
	if t.Status == download.StatusCompleted {
		// Every byte of a finished download is on disk, whether the ranges
		// are still around or not.
		for i := range out {
			out[i] = blockDone
		}
		return out
	}
	if len(t.Segments) == 0 || t.Size <= 0 {
		// No ranges to read, or no length to divide: fall back to the
		// task's own share and fill the cells in order.
		return fillBlocks(out, t.Percent())
	}
	per := float64(t.Size) / float64(len(out))
	for i := range out {
		start := int64(float64(i) * per)
		end := int64(float64(i+1) * per)
		if end <= start {
			continue
		}
		var done int64
		for _, s := range t.Segments {
			if s.Done <= 0 {
				continue
			}
			lo := max(start, s.Start)
			hi := min(end, s.Start+s.Done)
			if hi > lo {
				done += hi - lo
			}
		}
		switch {
		case done >= end-start:
			out[i] = blockDone
		case done > 0:
			out[i] = blockActive
		}
	}
	return out
}

// fillBlocks lights the cells a share p of the way through, for a task whose
// ranges say nothing.
func fillBlocks(cells []blockState, p float64) []blockState {
	filled := p * float64(len(cells))
	for i := range cells {
		switch {
		case float64(i+1) <= filled+1e-9:
			cells[i] = blockDone
		case float64(i) < filled:
			cells[i] = blockActive
		}
	}
	return cells
}

func blockColor(state blockState, paused bool) ui.Color {
	switch state {
	case blockDone:
		return colGreen
	case blockActive:
		if paused {
			// A paused download writes nothing: the frontier cools off.
			return colBlockCold
		}
		return colBlockHot
	}
	return colBlockTrack
}

func (a *App) inspectorShards(c *ui.Context, t download.Task) {
	cells := chunkBlocks(t)
	if len(cells) == 0 {
		return
	}
	paused := t.Status == download.StatusPaused
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, locales.ShardProgress()).FontSize(11.5).TextColor(colLabel)
		ui.Column(c).Gap(chunkGap).Children(func() {
			for row := 0; row < chunkRows; row++ {
				line := cells[row*chunkCols : (row+1)*chunkCols]
				ui.Row(c).Gap(chunkGap).Children(func() {
					for _, state := range line {
						ui.Box(c).Width(chunkSize).Height(chunkSize).
							Radius(1.5).Background(blockColor(state, paused))
					}
				})
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
