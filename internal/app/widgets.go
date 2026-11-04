package app

import (
	"github.com/egoist/mygo/ui"
)

// pillButton builds a full-round button: icon (when given) then label.
func (a *App) pillButton(c *ui.Context, label string, icon *ui.SVG, primary bool, fn func()) ui.Element {
	b := ui.ButtonBase(c).Height(32).Radius(16).Padding(0, 16).Gap(7).
		AlignItems(ui.Center).Justify(ui.Center).FocusRing(false).Label(label)
	if primary {
		b = b.Background(colGreen)
		if b.Hovered() {
			b = b.Background(colGreenDeep)
		}
	} else {
		b = b.Background(colCard).Border(1, colLine)
		if b.Hovered() {
			b = b.Background(ui.RGB(0xFF, 0xFF, 0xFF)).Border(1, colEdge)
		}
	}
	b.OnClick(fn)
	b.Children(func() {
		if icon != nil {
			iconColor := colMuted
			if primary {
				iconColor = ui.RGB(0xFF, 0xFF, 0xFF)
			}
			ui.Icon(c, icon).FontSize(13).TextColor(iconColor)
		}
		text := ui.Text(c, label).FontSize(12.5).SingleLine()
		if primary {
			text = text.Bold().TextColor(ui.RGB(0xFF, 0xFF, 0xFF))
		} else {
			text = text.TextColor(colText)
		}
	})
	return b
}

func (a *App) secondaryButton(c *ui.Context, label string, fn func()) ui.Element {
	return a.pillButton(c, label, nil, false, fn)
}

func (a *App) primaryButton(c *ui.Context, label string, fn func()) ui.Element {
	return a.pillButton(c, label, nil, true, fn)
}

// iconButton is a square, borderless button holding one icon.
func (a *App) iconButton(c *ui.Context, icon *ui.SVG, tip string, fn func()) ui.Element {
	btn := ui.Box(c).Size(26, 26).Radius(7).AlignItems(ui.Center).Justify(ui.Center)
	if btn.Hovered() {
		btn = btn.Background(ui.RGBA(28, 28, 30, 0.06))
	}
	return btn.Tooltip(tip).OnClick(fn).Children(func() {
		ui.Icon(c, icon).FontSize(15).TextColor(colMuted)
	})
}

func (a *App) chip(c *ui.Context, label string) {
	ui.Box(c).Padding(2, 7).Radius(6).Background(colChip).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(colMuted).SingleLine()
	})
}

func (a *App) switchRow(c *ui.Context, label, hint string, value *bool) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, label).FontSize(12.5).TextColor(colText)
			if hint != "" {
				ui.Text(c, hint).FontSize(11).TextColor(colFaint)
			}
		})
		ui.Switch(c, value)
	})
}

func (a *App) sectionLabel(c *ui.Context, label string) {
	ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(colLabel).Padding(8, 8, 4, 8).SingleLine()
}
