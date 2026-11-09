package app

import (
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/store"
)

// The "quiet paper" palette: warm paper, near-white cards that lift off the
// page, and one green for everything actionable. The dark palette is the
// same page after dusk: warm charcoal instead of paper, cards a step above
// it, and the same green, lifted until it reads on the dark.
//
// The views draw with the palette of the frame, App.pal, so that every
// colour is picked in one place and a change of appearance is a change of
// palette rather than a second set of views.
type palette struct {
	// dark is the appearance the palette draws, and the one the widgets
	// take from segraTheme.
	dark bool

	bg      ui.Color // the page the cards sit on
	side    ui.Color // sidebar and the strip above the header
	card    ui.Color // download rows
	cardSel ui.Color // the chosen row, a breath of green
	surface ui.Color // inspector panel
	bar     ui.Color // the status bar

	text  ui.Color
	sub   ui.Color
	muted ui.Color
	faint ui.Color
	label ui.Color

	line     ui.Color
	lineSoft ui.Color
	edge     ui.Color
	pill     ui.Color
	sel      ui.Color // the sidebar's mark and selected text
	chip     ui.Color

	// The faces a control takes under the pointer and under the press, and
	// the fields a value is typed into.
	cardHover   ui.Color
	cardPressed ui.Color
	input       ui.Color
	hover       ui.Color // a wash over an icon button
	navHover    ui.Color // a wash over a sidebar item

	green     ui.Color
	greenDeep ui.Color // the green under the pointer, and pressed
	onGreen   ui.Color // text and icons on the green

	warn   ui.Color
	danger ui.Color

	// The tile a file's icon sits on, and the scroll bar's thumb.
	iconTile  ui.Color
	iconInk   ui.Color
	scrollbar ui.Color

	// The chunk map: an empty patch, the frontier of bytes being written,
	// and that frontier once the download pauses.
	blockTrack ui.Color
	blockHot   ui.Color
	blockCold  ui.Color

	// The shadows cards and sheets cast, and the tint they are cast in.
	shadowSoft  ui.Color
	shadowHover ui.Color
	shadowSel   ui.Color
	shadowModal ui.Color
}

// lightPal is the palette Segra has always drawn: the quiet paper.
var lightPal = palette{
	bg:      ui.RGB(0xF8, 0xF7, 0xF3),
	side:    ui.RGB(0xF0, 0xEF, 0xEA),
	card:    ui.RGB(0xFD, 0xFC, 0xFA),
	cardSel: ui.RGB(0xFB, 0xFD, 0xF9),
	surface: ui.RGB(0xFB, 0xFA, 0xF6),
	bar:     ui.RGB(0xF1, 0xF1, 0xEB),

	text:  ui.RGB(0x1C, 0x1C, 0x1E),
	sub:   ui.RGB(0x3B, 0x3B, 0x3D),
	muted: ui.RGB(0x5E, 0x5E, 0x60),
	faint: ui.RGB(0x77, 0x77, 0x6F),
	label: ui.RGB(0x4A, 0x4A, 0x48),

	line:     ui.RGB(0xEA, 0xE9, 0xE5),
	lineSoft: ui.RGB(0xF1, 0xF0, 0xEC),
	edge:     ui.RGB(0xE4, 0xE3, 0xDE),
	pill:     ui.RGB(0xE7, 0xE8, 0xE2),
	sel:      ui.RGB(0xE2, 0xE8, 0xDE),
	chip:     ui.RGB(0xEF, 0xED, 0xE6),

	cardHover:   ui.RGB(0xFF, 0xFF, 0xFF),
	cardPressed: ui.RGB(0xF4, 0xF3, 0xEF),
	input:       ui.RGB(0xFF, 0xFF, 0xFF),
	hover:       ui.RGBA(0x1C, 0x1C, 0x1E, 0.06),
	navHover:    ui.RGBA(0xFF, 0xFF, 0xFF, 0.55),

	green:     ui.RGB(0x2F, 0x7D, 0x57),
	greenDeep: ui.RGB(0x26, 0x6A, 0x49),
	onGreen:   ui.RGB(0xFF, 0xFF, 0xFF),

	warn:   ui.RGB(0xA9, 0x6A, 0x12),
	danger: ui.RGB(0xB4, 0x45, 0x3B),

	iconTile:  ui.RGB(0xF1, 0xEF, 0xE9),
	iconInk:   ui.RGB(0x55, 0x55, 0x5C),
	scrollbar: ui.RGBA(0x1C, 0x1C, 0x1E, 0.22),

	blockTrack: ui.RGBA(0x1C, 0x1C, 0x1E, 0.10),
	blockHot:   ui.RGB(0xC0, 0x84, 0x3A),
	blockCold:  ui.RGB(0xD2, 0xAC, 0x72),

	shadowSoft:  ui.RGBA(0x1C, 0x1C, 0x1E, 0.04),
	shadowHover: ui.RGBA(0x1C, 0x1C, 0x1E, 0.06),
	shadowSel:   ui.RGBA(0x1C, 0x1C, 0x1E, 0.08),
	shadowModal: ui.RGBA(0x14, 0x14, 0x1E, 0.18),
}

// darkPal is the same page after dusk. The green is lifted so that it keeps
// its contrast on charcoal, and the shadows deepen, since a dark surface
// casts a shadow the page already is.
var darkPal = palette{
	dark:    true,
	bg:      ui.RGB(0x17, 0x17, 0x15),
	side:    ui.RGB(0x1C, 0x1C, 0x19),
	card:    ui.RGB(0x22, 0x22, 0x1F),
	cardSel: ui.RGB(0x27, 0x30, 0x29),
	surface: ui.RGB(0x1E, 0x1E, 0x1B),
	bar:     ui.RGB(0x1B, 0x1B, 0x18),

	text:  ui.RGB(0xEC, 0xEB, 0xE5),
	sub:   ui.RGB(0xC8, 0xC7, 0xC0),
	muted: ui.RGB(0x9B, 0x9A, 0x92),
	faint: ui.RGB(0x8A, 0x89, 0x81),
	label: ui.RGB(0xB7, 0xB6, 0xAE),

	line:     ui.RGB(0x30, 0x2F, 0x2A),
	lineSoft: ui.RGB(0x2A, 0x29, 0x25),
	edge:     ui.RGB(0x3A, 0x39, 0x33),
	pill:     ui.RGB(0x2E, 0x2D, 0x28),
	sel:      ui.RGB(0x2C, 0x3B, 0x31),
	chip:     ui.RGB(0x2B, 0x2A, 0x25),

	cardHover:   ui.RGB(0x2A, 0x2A, 0x26),
	cardPressed: ui.RGB(0x33, 0x33, 0x2E),
	input:       ui.RGB(0x1B, 0x1B, 0x18),
	hover:       ui.RGBA(0xFF, 0xFF, 0xFF, 0.07),
	navHover:    ui.RGBA(0xFF, 0xFF, 0xFF, 0.05),

	green:     ui.RGB(0x63, 0xBA, 0x90),
	greenDeep: ui.RGB(0x7F, 0xCB, 0xA5),
	onGreen:   ui.RGB(0x0E, 0x15, 0x12),

	warn:   ui.RGB(0xD9, 0xA2, 0x4A),
	danger: ui.RGB(0xE2, 0x79, 0x6C),

	iconTile:  ui.RGB(0x2A, 0x2A, 0x25),
	iconInk:   ui.RGB(0xB9, 0xB8, 0xB1),
	scrollbar: ui.RGBA(0xFF, 0xFF, 0xFF, 0.22),

	blockTrack: ui.RGBA(0xFF, 0xFF, 0xFF, 0.10),
	blockHot:   ui.RGB(0xD2, 0xA0, 0x52),
	blockCold:  ui.RGB(0xA8, 0x84, 0x5C),

	shadowSoft:  ui.RGBA(0x00, 0x00, 0x00, 0.45),
	shadowHover: ui.RGBA(0x00, 0x00, 0x00, 0.55),
	shadowSel:   ui.RGBA(0x00, 0x00, 0x00, 0.60),
	shadowModal: ui.RGBA(0x00, 0x00, 0x00, 0.55),
}

// dark reports whether the app draws in the dark appearance: the one the
// settings chose, or the desktop's while they follow it.
func (a *App) dark(c *ui.Context) bool {
	switch a.settings.Theme {
	case store.ThemeLight:
		return false
	case store.ThemeDark:
		return true
	}
	// The frame's own theme is the desktop's appearance, which the runtime
	// has already read for this pass; asking it costs nothing and cannot be
	// a frame behind a change of the desktop's.
	return c.Theme().Dark
}

// usePalette picks the palette the frame draws with. Every view calls it as
// it builds, before it sets the theme, so that a change of appearance
// reaches the frame whole.
func (a *App) usePalette(c *ui.Context) {
	a.pal = paletteFor(a.dark(c))
}

// paletteFor returns the palette of an appearance.
func paletteFor(dark bool) palette {
	if dark {
		return darkPal
	}
	return lightPal
}

// block paints one patch of the chunk map: green where the bytes are on
// disk, the frontier's amber while they are on their way, and the empty
// track before that.
func (p palette) block(state blockState, paused bool) ui.Color {
	switch state {
	case blockDone:
		return p.green
	case blockActive:
		if paused {
			// A paused download writes nothing: the frontier cools off.
			return p.blockCold
		}
		return p.blockHot
	}
	return p.blockTrack
}

// segraTheme keeps the widgets built into mygo — search fields, text inputs,
// steppers, switches, scroll bars and focus rings — on the same palette.
func segraTheme(p palette) *ui.Theme {
	t := ui.LightTheme()
	if p.dark {
		t = ui.DarkTheme()
	}
	t.Background = p.bg
	t.Surface = p.card
	t.SurfaceHover = p.cardHover
	t.SurfacePressed = p.cardPressed
	t.Border = p.line
	t.Text = p.text
	t.TextMuted = p.muted
	t.Accent = p.green
	// The app draws most of its green by hand, in a colour no window state
	// changes, so the widgets that take the accent from the theme keep it too:
	// a window of quiet paper should not grow a patch of grey when it loses
	// the keyboard.
	t.InactiveAccent = p.green
	t.AccentHover = p.greenDeep
	t.AccentPressed = p.greenDeep
	t.AccentText = p.onGreen
	t.Danger = p.danger
	t.Warning = p.warn
	t.Success = p.green
	t.Selection = p.sel
	t.Focus = p.green
	t.Scrollbar = p.scrollbar
	t.Radius = 8
	return t
}
