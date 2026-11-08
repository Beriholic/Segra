package app

import (
	"github.com/egoist/mygo/ui"
)

// The "quiet paper" palette: warm paper, near-white cards that lift off the
// page, and one green for everything actionable.
var (
	colBG        = ui.RGB(0xF8, 0xF7, 0xF3) // the page the cards sit on
	colSide      = ui.RGB(0xF0, 0xEF, 0xEA) // sidebar and the strip above the header
	colCard      = ui.RGB(0xFD, 0xFC, 0xFA) // download rows
	colCardSel   = ui.RGB(0xFB, 0xFD, 0xF9) // the chosen row, a breath of green
	colSurface   = ui.RGB(0xFB, 0xFA, 0xF6) // inspector panel
	colText      = ui.RGB(0x1C, 0x1C, 0x1E)
	colSub       = ui.RGB(0x3B, 0x3B, 0x3D)
	colMuted     = ui.RGB(0x5E, 0x5E, 0x60)
	colFaint     = ui.RGB(0x77, 0x77, 0x6F)
	colLabel     = ui.RGB(0x4A, 0x4A, 0x48)
	colLine      = ui.RGB(0xEA, 0xE9, 0xE5)
	colLineSoft  = ui.RGB(0xF1, 0xF0, 0xEC)
	colEdge      = ui.RGB(0xE4, 0xE3, 0xDE)
	colBar       = ui.RGB(0xF1, 0xF1, 0xEB)
	colPill      = ui.RGB(0xE7, 0xE8, 0xE2)
	colSel       = ui.RGB(0xE2, 0xE8, 0xDE)
	colChip      = ui.RGB(0xEF, 0xED, 0xE6)
	colGreen     = ui.RGB(0x2F, 0x7D, 0x57)
	colGreenDeep = ui.RGB(0x26, 0x6A, 0x49)
	colAccent    = colGreen
	colOK        = colGreen
	colWarn      = ui.RGB(0xA9, 0x6A, 0x12)
	colDanger    = ui.RGB(0xB4, 0x45, 0x3B)

	// The chunk map: an empty patch, the frontier of bytes being written,
	// and that frontier once the download pauses.
	colBlockTrack = ui.RGBA(0x1C, 0x1C, 0x1E, 0.10)
	colBlockHot   = ui.RGB(0xC0, 0x84, 0x3A)
	colBlockCold  = ui.RGB(0xD2, 0xAC, 0x72)
)

// segraTheme keeps the widgets built into mygo — search fields, text inputs,
// steppers, switches, scroll bars and focus rings — on the same palette.
func segraTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Background = colBG
	t.Surface = colCard
	t.SurfaceHover = ui.RGB(0xFF, 0xFF, 0xFF)
	t.SurfacePressed = ui.RGB(0xF4, 0xF3, 0xEF)
	t.Border = colLine
	t.Text = colText
	t.TextMuted = colMuted
	t.Accent = colGreen
	t.AccentHover = colGreenDeep
	t.AccentPressed = colGreenDeep
	t.AccentText = ui.RGB(0xFF, 0xFF, 0xFF)
	t.Danger = colDanger
	t.Warning = colWarn
	t.Success = colGreen
	t.Selection = colSel
	t.Focus = colGreen
	t.Scrollbar = ui.RGBA(0x1C, 0x1C, 0x1E, 0.22)
	t.Radius = 8
	return t
}
