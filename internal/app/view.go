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
	a.usePalette(c)
	c.SetTheme(segraTheme(a.pal))
	// A folder typed into the settings is done with when its view is left,
	// or the field would go away before anything saved it.
	if a.view != "settings" {
		a.commitField()
	}
	// A worker that saved something says so here, where a toast has the
	// frame's own clock to count its seconds from.
	if message := a.takeToast(); message != "" {
		c.Toast(message)
	}

	// The window draws no title bar of its own: the strip at the top holds
	// the traffic lights and drags, and everything else starts at the very
	// top of the window.
	tb := c.TitleBar()
	strip := tb.Height
	if strip < 28 {
		strip = 28
	}

	ui.Column(c).Fill().Background(a.pal.bg).Children(func() {
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c, strip)
			a.content(c, strip)
			if a.inspectorVisible(c) {
				a.inspector(c, strip)
			}
		})
		a.statusBar(c)
		a.newDialog(c)
		a.updateDialog(c)
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(strip).DragWindow()
	})
}

func (a *App) sidebar(c *ui.Context, strip float32) {
	ui.Column(c).Width(218).FillHeight().Background(a.pal.side).Children(func() {
		ui.Box(c).Absolute().Right(0).Top(0).Bottom(0).Width(1).Background(a.pal.edge)
		ui.Column(c).Grow(1).Padding(strip+12, 12, 4, 12).Children(func() {
			ui.SearchField(c, &a.search).Placeholder(locales.Search()).Height(32).
				Background(a.pal.input).Border(1, a.pal.edge).Radius(10)
			ui.Box(c).Height(18)

			a.sectionLabel(c, locales.Downloads())
			ui.Box(c).Height(2)
			a.navGroup(c, []navEntry{
				{id: "all", icon: icGrid, label: locales.All(), count: len(a.tasks)},
				{id: "active", icon: icActive, label: locales.Active(), count: a.countStatus(download.StatusDownloading, download.StatusQueued)},
				{id: "completed", icon: icCheck, label: locales.Completed(), count: a.countStatus(download.StatusCompleted)},
				{id: "failed", icon: icFailed, label: locales.Failed(), count: a.countStatus(download.StatusFailed)},
			})

			ui.Box(c).Height(16)
			a.sectionLabel(c, locales.Settings())
			ui.Box(c).Height(2)
			a.navGroup(c, []navEntry{
				{id: "settings", icon: icSettings, label: locales.General(), dot: a.updateAvailable()},
			})
			ui.Spacer(c)
		})
	})
}

// navPitch is the room one item takes, its height and the margin below it.
const navPitch = 35

// navEntry is one choice of the sidebar.
type navEntry struct {
	id    string
	icon  *ui.SVG
	label string
	count int
	// dot marks a place with news, as Settings has when a release is
	// waiting.
	dot bool
}

// navGroup lays its items out over one highlight that slides between them,
// so that choosing another view moves the mark instead of blinking it. The
// group draws the highlight itself, under its items: a box of its own would
// paint over them, as a positioned element paints over its siblings.
func (a *App) navGroup(c *ui.Context, items []navEntry) {
	group := ui.Column(c)
	index := -1
	for i, it := range items {
		if a.view == it.id {
			index = i
		}
	}
	targetY, targetOp := float32(0), float32(0)
	if index >= 0 {
		targetY, targetOp = float32(index)*navPitch, 1
	}
	y := group.AnimateTo("nav-y", targetY, ui.Animation{Duration: motionBase, Ease: ui.EaseOut})
	op := group.Animate("nav-op", targetOp, motionBase)
	group.Draw(func(p *ui.Painter, r ui.Rect) {
		if op <= 0.01 {
			return
		}
		p.Fill(ui.Rect{X: r.X, Y: r.Y + y, W: r.W, H: 32}, a.pal.sel.Alpha(op), 10)
	})
	group.Children(func() {
		for _, it := range items {
			a.navItem(c, it)
		}
	})
}

func (a *App) navItem(c *ui.Context, it navEntry) {
	selected := a.view == it.id
	item := ui.Row(c).Height(32).Margin(0, 0, 3, 0).Padding(0, 10).Gap(9).AlignItems(ui.Center).Radius(10)
	if item.Hovered() && !selected {
		item = item.Background(a.pal.navHover)
	}
	item.Transition(hoverColors)
	item.OnClick(func() {
		if a.view == it.id {
			return
		}
		a.view = it.id
		a.selected = -1
		a.refresh()
	})
	iconColor, textColor := a.pal.muted, a.pal.sub
	if selected {
		iconColor, textColor = a.pal.green, a.pal.green
	}
	iconColor = item.AnimateColor("icon", iconColor, fadeCurve)
	textColor = item.AnimateColor("text", textColor, fadeCurve)
	item.Children(func() {
		ui.Icon(c, it.icon).FontSize(13.5).TextColor(iconColor)
		text := ui.Text(c, it.label).FontSize(13).SingleLine().Grow(1).TextColor(textColor)
		if selected {
			text.Bold()
		}
		if it.dot {
			ui.Box(c).Size(6, 6).Radius(3).Background(a.pal.green)
		}
		if it.count > 0 {
			ui.Box(c).Padding(1, 6).Radius(6).Background(a.pal.pill).Children(func() {
				countColor := a.pal.muted
				if selected {
					countColor = a.pal.green
				}
				ui.Text(c, fmt.Sprint(it.count)).FontSize(10.5).FontFeatures("tnum").TextColor(countColor)
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
	// A page replaces the one before it at once: the content has no surface
	// of its own, so two of them faded across each other would show through
	// one another. The sidebar's highlight and the numbers carry the change
	// instead.
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
		ui.Text(c, locales.Downloads()).FontSize(26).Bold().TextColor(a.pal.text).Grow(1)
		pause := a.secondaryButton(c, locales.PauseAll(), func() { a.mgr.PauseAll() }).Height(34).Radius(17).Disabled(active == 0)
		if active == 0 {
			pause.Opacity(0.5)
		}
		a.historyMenu(c)
		a.primaryButton(c, locales.NewDownload(), a.openNew).Height(34).Radius(17)
	})
}

// historyMenu is the header's way to tidy the list by hand: the finished
// records go and the files stay, as the automatic clean-up leaves them. It
// greys out when nothing is finished, and each item says how many records it
// would take, so a clear that does nothing is not offered at all.
func (a *App) historyMenu(c *ui.Context) {
	completed := a.countStatus(download.StatusCompleted)
	failed := a.countStatus(download.StatusFailed)
	empty := completed+failed == 0

	btn := ui.Box(c).Size(34, 34).Radius(17).AlignItems(ui.Center).Justify(ui.Center).
		Background(a.pal.card).Border(1, a.pal.line)
	if btn.Hovered() && !empty {
		btn = btn.Background(a.pal.cardHover).Border(1, a.pal.edge)
	}
	btn.Transition(hoverColors)
	btn.AnimatePose(pressPose(btn), springSnap)
	btn.Disabled(empty)
	if empty {
		btn.Opacity(0.5)
	}
	btn.Tooltip(locales.ClearHistory())
	// An icon-only button needs its name for assistive technology, and it
	// is how a test finds it.
	btn.Label(locales.ClearHistory())
	btn.Menu(func(m *ui.Menu) {
		a.historyItem(m, download.StatusCompleted, locales.ClearCompleted(), completed)
		a.historyItem(m, download.StatusFailed, locales.ClearFailed(), failed)
	})
	btn.Children(func() {
		ui.Icon(c, icTrash).FontSize(15).TextColor(a.pal.muted)
	})
}

// historyItem is one clear of the history menu. The count says what it would
// take, so a record group that is already empty is not offered.
func (a *App) historyItem(m *ui.Menu, status download.Status, label string, count int) {
	if m.Item(fmt.Sprintf("%s (%d)", label, count)).Disabled(count == 0).Chosen() {
		a.clearHistory(status)
	}
}

func (a *App) emptyState(c *ui.Context) {
	ui.Column(c).Key("empty").Grow(1).Center().Gap(9).
		Transition(ui.ElementTransition{Animation: &springSoft, Enter: fadeIn(10)}).
		Children(func() {
			ui.Box(c).Size(56, 56).Radius(18).Center().Background(a.pal.side).Children(func() {
				ui.Icon(c, icDownload).FontSize(24).TextColor(a.pal.green)
			})
			ui.Box(c).Height(4)
			ui.Text(c, locales.NoTasks()).FontSize(16).Bold().TextColor(a.pal.text)
			ui.Text(c, locales.NoTasksHint()).FontSize(12.5).TextColor(a.pal.muted).TextAlign(ui.Center).MaxWidth(340)
			ui.Box(c).Height(4)
			a.primaryButton(c, locales.NewDownload(), a.openNew)
		})
}

func (a *App) statusBar(c *ui.Context) {
	ui.Row(c).Height(30).Padding(0, 16).Gap(12).AlignItems(ui.Center).Background(a.pal.bar).Children(func() {
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(1).Background(a.pal.edge)
		ui.Text(c, fmt.Sprintf("%d %s", len(a.tasks), locales.Files())).FontSize(11).TextColor(a.pal.sub).SingleLine().Grow(1)
		if a.apiErr != "" {
			ui.Text(c, locales.ApiRunning()+": "+a.apiErr).FontSize(11).TextColor(a.pal.danger).SingleLine()
		}
		ui.Text(c, fmt.Sprintf("%d %s", a.mgr.ActiveCount(), locales.Active())).FontSize(11).TextColor(a.pal.sub)
	})
}
