package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The app's motion is quiet and physical: short fades for what the pointer
// touches, springs for what the layout moves, and one moment of delight when
// a download lands. Every duration and spring in the app comes from here, so
// that the whole window moves at one pace.
const (
	// motionFast fades a colour: a hover, a chip lighting up.
	motionFast = 120 * time.Millisecond
	// motionBase folds what is small: a selection, a dialog.
	motionBase = 180 * time.Millisecond
	// motionSlow moves what the layout gives room: a panel, a page.
	motionSlow = 260 * time.Millisecond

	// freshFor is how long a download counts as new: its row slides in
	// when it first shows, and never again, however the list scrolls.
	freshFor = 1500 * time.Millisecond
	// finishFor is how long a row shows that a download has just landed.
	finishFor = 900 * time.Millisecond
)

var (
	// springSoft moves what is laid out again, as a row leaving or the
	// details panel folding: a spring without a bounce.
	springSoft = ui.Animation{Spring: ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0}}
	// springSnap answers the pointer: hover, press, the sidebar's highlight.
	springSnap = ui.Snappy
	// fadeCurve fades what a value drives, where a spring would overshoot.
	fadeCurve = ui.Animation{Duration: motionBase, Ease: ui.EaseOut}
	// hoverColors fades the background and border of what the pointer is
	// over, as the card under it lifts.
	hoverColors = ui.ElementTransition{Colors: true, Duration: motionFast}
)

// fadeIn slides an element up from a little below where it belongs, without
// fading it: text stays readable in the first frame, and a list that scrolls
// back into view never shows a blank row.
func fadeIn(y float32) *ui.Motion {
	return &ui.Motion{Y: y, Opacity: 1}
}
