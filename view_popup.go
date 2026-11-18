package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/uzuki-P/pray-time-mygo/internal/prayer"
	"github.com/uzuki-P/pray-time-mygo/internal/store"
)

const (
	rowHeight   = 34
	railInset   = 11
	listPadX    = 10
	listPadY    = 6
	checkSize   = 22
	tabularNums = "tnum"
)

// palette is the light or dark palette the theme setting picks.
func (a *app) palette() Palette {
	switch a.state.ThemeMode() {
	case store.ThemeLight:
		return lightPalette
	case store.ThemeDark:
		return darkPalette
	}
	if mygo.Theme.IsDark() {
		return darkPalette
	}
	return lightPalette
}

// everySecond asks for the next frame on the next second, while the window
// shows, so countdowns stay current without the ticker.
func everySecond(c *ui.Context) {
	if c.WindowHidden() {
		return
	}
	now := c.Now()
	c.After(now.Truncate(time.Second).Add(time.Second).Sub(now) + 5*time.Millisecond)
}

func display(c *ui.Context, s string, size float32, weight int) ui.Element {
	return ui.Text(c, s).Font(displayFont).FontSize(size).FontWeight(weight)
}

func body(c *ui.Context, s string, size float32, weight int) ui.Element {
	return ui.Text(c, s).FontSize(size).FontWeight(weight)
}

func figures(c *ui.Context, s string, size float32, weight int) ui.Element {
	return body(c, s, size, weight).FontFeatures(tabularNums)
}

func (a *app) popupView(c *ui.Context) {
	p := a.palette()
	c.SetTheme(theme(p))
	everySecond(c)
	if c.Shortcut(0, ui.KeyEscape) && a.popup != nil {
		a.popup.Hide()
		a.syncMenu()
	}
	a.popupBody(c, p)
}

func (a *app) popupBody(c *ui.Context, p Palette) {
	day := a.sched.Today()
	cd := a.sched.Countdown()
	now := a.sched.Now()
	due, hasDue := prayer.LatestDue(day, a.sched.CompletedSet(), now)

	ui.Column(c).Fill().Background(p.Surface).Children(func() {
		sky := skyColors["Isha"]
		if cd.OK {
			sky = skyColors[cd.Next.Name]
		}
		header := ui.Column(c).Padding(14, 18, 12, 18).Gap(0).DragWindow().
			Gradient(sky.Alpha(0.2).Over(p.Surface), p.Surface, 180)
		header.Children(func() {
			if !cd.OK {
				body(c, "No prayer times for this place today", 13, 500).TextColor(p.Muted())
				return
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				body(c, "Next prayer", 12, 500).TextColor(p.Muted()).Grow(1)
				display(c, prayer.ArabicNames[cd.Next.Name], 15, 500).TextColor(p.Muted())
			})
			display(c, cd.Next.Name, 32, 700).TextColor(p.OnSurface)
			ui.Row(c).AlignItems(ui.End).Gap(6).Children(func() {
				figures(c, "in "+prayer.CompactRemaining(cd.Remaining), 15, 600).TextColor(p.Accent)
				figures(c, "at "+cd.Next.DisplayTime(), 13, 400).TextColor(p.Muted())
			})
		})

		rail := prayer.RailPosition(day, now)
		ui.Column(c).Padding(listPadY, listPadX).Draw(func(pt *ui.Painter, r ui.Rect) {
			drawRail(pt, r, day, rail, p)
		}).Children(func() {
			for _, t := range day.Times {
				a.scheduleRow(c.Key(t.Name), p, t, cd, due, hasDue, now)
			}
		})

		ui.Box(c).Height(1).Background(p.Divider())
		footer := ui.Row(c).Padding(9, 18, 10, 18).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if footer.Hovered() {
			footer.Background(p.OnSurface.Alpha(0.04))
		}
		footer.Children(func() {
			body(c, day.Region.DisplayName(), 12, 400).TextColor(p.Muted()).SingleLine().Grow(1).Shrink(1)
			body(c, "Settings", 12, 600).TextColor(p.Accent)
		})
		if footer.Label("Settings").Clicked() {
			a.showSettings()
		}
	})
}

func (a *app) scheduleRow(c *ui.Context, p Palette, t prayer.Time, cd prayer.Countdown, due prayer.Time, hasDue bool, now time.Time) {
	done := a.sched.IsCompleted(t)
	isNext := cd.OK && cd.Next.Equal(t)
	isDue := hasDue && due.Equal(t)
	arrived := !t.Instant.After(now)

	row := ui.Row(c).Height(rowHeight).Radius(10).AlignItems(ui.Center).Padding(0, 2, 0, railInset*2+6)
	if isDue {
		row.Background(p.Due.Alpha(0.12))
	}
	row.Children(func() {
		if t.IsPrayer() {
			weight, color := 500, p.OnSurface
			if isNext || isDue {
				weight = 700
			}
			if done {
				color = p.Faint()
			} else if isNext {
				color = p.Accent
			}
			display(c, t.Name, 16, weight).TextColor(color).Grow(1)
		} else {
			body(c, t.Name, 12, 400).TextColor(p.Faint()).Grow(1)
		}
		if isDue {
			body(c, "Due", 11, 700).TextColor(p.Due).Margin(0, 8, 0, 0)
		}
		size, weight, color := float32(14), 500, p.Muted()
		if !t.IsPrayer() {
			size = 12
		}
		if isNext || isDue {
			weight = 600
		}
		if !t.IsPrayer() || done {
			color = p.Faint()
		} else if isNext {
			color = p.Accent
		}
		figures(c, t.DisplayTime(), size, weight).TextColor(color)
		ui.Row(c).Width(32).Justify(ui.End).Children(func() {
			if t.IsPrayer() && a.prayerCheck(c, p, t.Name, done, isDue, arrived) {
				a.markPrayed(t, !done)
			}
		})
	})
}

// checkFlip is when a prayer check last turned completed.
type checkFlip struct {
	was bool
	at  time.Time
}

var checkPop = spring{0.42, stiffnessMedium}

// prayerCheck is the round check box of a prayer: teal and ticked when
// completed, ringed in coral while due, faint until the prayer arrives.
// Turning completed, it pops in from small and sends out a ring.
func (a *app) prayerCheck(c *ui.Context, p Palette, name string, done, due, enabled bool) bool {
	ring := p.OnSurface.Alpha(0.3)
	if due {
		ring = p.Due
	}
	fill := p.Prayed.Alpha(0)
	if done {
		fill, ring = p.Prayed, p.Prayed
	}
	label := "Mark " + name + " as prayed"
	if done {
		label = "Mark " + name + " as not prayed"
	}

	holder := ui.Box(c).Size(checkSize+8, checkSize+8).Center()
	flip := ui.Local(holder, "flip", func() checkFlip { return checkFlip{was: done} })
	if done && !flip.was {
		flip.at = c.Now()
	}
	flip.was = done
	pop, burst := float32(1), float32(1)
	if !flip.at.IsZero() {
		t := float64(c.Now().Sub(flip.at).Microseconds()) / 1000
		pop = checkPop.at(t, 0, 0.55, 1)
		burst = tween(t, 0, 380, 0, 1, fastOutSlowIn)
		if t < checkPop.settle(0.45) || burst < 1 {
			c.AnimationFrame()
		} else {
			flip.at = time.Time{}
		}
	}
	if burst < 1 {
		prayed := p.Prayed
		holder.Draw(func(pt *ui.Painter, r ui.Rect) {
			rad := checkSize / 2 * (1 + 0.9*burst)
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			w := 1.5 * (1 - burst)
			pt.Stroke(ui.Rect{X: cx - rad, Y: cy - rad, W: rad * 2, H: rad * 2}, prayed.Alpha(1-burst), rad, w)
		})
	}

	var box ui.Element
	holder.Children(func() {
		box = ui.Box(c).Size(checkSize, checkSize).Radius(checkSize/2).Border(2, ring).
			Label(label).Disabled(!enabled).Scale(pop, pop)
		bg := box.AnimateColor("fill", fill, ui.Animation{Duration: 150 * time.Millisecond})
		if enabled {
			box.Cursor(ui.CursorPointer)
			if box.Hovered() && !done {
				bg = ring.Alpha(0.15)
			}
		} else {
			box.Opacity(0.45)
		}
		box.Background(bg)
		if done {
			onPrayed := p.OnPrayed
			box.Draw(func(pt *ui.Painter, r ui.Rect) {
				strokeCheck(pt, r.X+r.W*0.15, r.Y+r.H*0.15, r.W*0.7, 2.2, 1, onPrayed)
			})
		}
	})
	return box.Clicked()
}

// drawRail draws the schedule rail: a line through every row, coloured with
// the sky of each period up to now and faint after it, a node per time, and
// a marker at now.
func drawRail(pt *ui.Painter, r ui.Rect, day prayer.Day, rail float32, p Palette) {
	n := len(day.Times)
	if n == 0 {
		return
	}
	x := r.X + listPadX + railInset
	y := func(pos float32) float32 { return r.Y + listPadY + (pos+0.5)*rowHeight }
	nowY := y(rail)
	last := float32(n - 1)

	pt.Line(x, nowY, x, y(last), 2, p.Divider())
	for i := 0; i < n-1; i++ {
		if rail <= float32(i) {
			break
		}
		top, bottom := y(float32(i)), y(float32(i+1))
		end := min(bottom, nowY)
		from := skyColors[day.Times[i].Name]
		to := from.Mix(skyColors[day.Times[i+1].Name], (end-top)/(bottom-top))
		pt.FillGradient(ui.Rect{X: x - 1.5, Y: top, W: 3, H: end - top},
			ui.LinearGradient{From: from, To: to, Angle: 180}, 1.5)
	}
	for i, t := range day.Times {
		cy := y(float32(i))
		radius := float32(5)
		if !t.IsPrayer() {
			radius = 3.5
		}
		color := skyColors[t.Name]
		dot := ui.Rect{X: x - radius, Y: cy - radius, W: radius * 2, H: radius * 2}
		if float32(i) <= rail {
			pt.Fill(dot, color, radius)
		} else {
			pt.Fill(dot, p.Surface, radius)
			pt.Stroke(ui.Rect{X: dot.X + 1, Y: dot.Y + 1, W: dot.W - 2, H: dot.H - 2}, color, radius-1, 2)
		}
	}
	if rail != float32(int(rail)) {
		const half = 5
		pt.Line(x-half-1, nowY, x+half+1, nowY, 4, p.Surface)
		pt.Line(x-half, nowY, x+half, nowY, 2, p.OnSurface)
	}
}
