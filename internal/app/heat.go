package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The chunk map is drawn rather than built from a box a cell: ninety-six
// cells in the inspector, and three times as many in the panel, would cost
// the view a hundred elements a frame for a grid that never takes the
// pointer. Drawn, the grid also tweens in frames that only paint.
const (
	// heatFade is how long a cell takes to light up or cool down.
	heatFade = 160 * time.Millisecond
	// heatRadius rounds a cell, as the boxes of the grid used to be.
	heatRadius = 1.5
)

// heatTween is the chunk map of one download as it shows: the states of the
// frame before, the states of this one, and when they changed.
type heatTween struct {
	id   string
	from []blockState
	to   []blockState
	at   time.Time
}

// update tells the tween the states the download has now, and returns how
// far it goes from the ones it showed to them, from 0 to 1.
func (h *heatTween) update(id string, cells []blockState, now time.Time) float32 {
	if h.id != id || len(h.to) != len(cells) {
		h.id, h.from, h.to, h.at = id, cells, cells, now
		return 1
	}
	if !sameBlocks(h.to, cells) {
		h.from, h.to, h.at = h.to, cells, now
	}
	if h.at.IsZero() {
		return 1
	}
	k := float32(now.Sub(h.at)) / float32(heatFade)
	if k >= 1 {
		return 1
	}
	return ui.EaseOut(max(k, 0))
}

// sameBlocks reports whether two chunk maps say the same thing.
func sameBlocks(a, b []blockState) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// drawHeat paints the chunk map of one download in r: cells as large as the
// room allows, each lit by how much of its bytes are on disk, moving from the
// states the last frame showed to the ones the download has now.
func drawHeat(p *ui.Painter, r ui.Rect, pal palette, h *heatTween, id string, cells []blockState, paused bool, gap float32) {
	if len(cells) != chunkRows*chunkCols {
		return
	}
	k := h.update(id, cells, p.Now())
	size := min(
		(r.W-gap*float32(chunkCols-1))/float32(chunkCols),
		(r.H-gap*float32(chunkRows-1))/float32(chunkRows),
	)
	if size <= 0 {
		return
	}
	w := float32(chunkCols)*size + gap*float32(chunkCols-1)
	hh := float32(chunkRows)*size + gap*float32(chunkRows-1)
	x0 := r.X + (r.W-w)/2
	y0 := r.Y + (r.H-hh)/2
	for i := range cells {
		color := pal.block(cells[i], paused)
		if k < 1 {
			color = pal.block(h.from[i], paused).Mix(color, k)
		}
		x := x0 + float32(i%chunkCols)*(size+gap)
		y := y0 + float32(i/chunkCols)*(size+gap)
		p.Fill(ui.Rect{X: x, Y: y, W: size, H: size}, color, heatRadius)
	}
	if k < 1 {
		// The cells are still on their way: draw frames that only paint
		// until they arrive, without building the view again.
		p.AnimationFrame()
	}
}
