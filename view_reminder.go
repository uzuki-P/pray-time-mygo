package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/uzuki-P/pray-time-mygo/internal/prayer"
)

func (a *app) reminderView(c *ui.Context) {
	p := a.palette()
	c.SetTheme(theme(p))
	everySecond(c)
	if c.Shortcut(0, ui.KeyEscape) {
		a.dismissReminder()
	}
	a.reminderBody(c, p)
}

func (a *app) reminderBody(c *ui.Context, p Palette) {
	t := a.reminderPrayer()
	if t.Name == "" {
		ui.Column(c).Fill().Background(p.Surface)
		return
	}
	switch a.outcome {
	case outcomePrayed:
		ui.Column(c).Fill().Background(p.Prayed).Center().Gap(6).Children(func() {
			check := ui.Box(c).Size(34, 34).Radius(17).Border(2.5, p.OnPrayed)
			check.Draw(func(pt *ui.Painter, r ui.Rect) {
				pt.StrokePolyline([][2]float32{{r.X + 9.5, r.Y + 17.5}, {r.X + 15, r.Y + 23}, {r.X + 25, r.Y + 12}}, 3, p.OnPrayed)
			})
			display(c, a.outcomeText, 16, 700).TextColor(p.OnPrayed)
		})
		return
	case outcomeSnoozed:
		ui.Column(c).Fill().Background(p.Raised).Center().Children(func() {
			display(c, t.Name, 22, 700).TextColor(p.Faint())
			body(c, a.outcomeText, 13, 500).TextColor(p.Muted())
		})
		return
	}

	snooze := "Dismiss"
	if a.state.SnoozeEnabled() {
		snooze = "Snooze " + strconv.Itoa(a.state.SnoozeMinutes()) + " min"
	}
	sky := skyColors[t.Name]
	ui.Column(c).Fill().Background(p.Surface).Children(func() {
		// A band of the prayer's sky colour down the left edge.
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Width(5).Background(sky)
			ui.Column(c).Grow(1).Padding(14, 16, 14, 14).Gap(4).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Box(c).Size(8, 8).Radius(4).Background(p.Due)
					body(c, "Prayer due", 11, 700).TextColor(p.Due)
				})
				ui.Row(c).AlignItems(ui.End).Gap(10).Children(func() {
					display(c, t.Name, 24, 700).TextColor(p.OnSurface)
					display(c, prayer.ArabicNames[t.Name], 17, 500).TextColor(p.Muted())
				})
				figures(c, prayer.DueDetail(t, a.sched.Now()), 12, 400).TextColor(p.Muted())
				ui.Spacer(c)
				ui.Row(c).Gap(8).Children(func() {
					prayed := ui.Button(c, "Mark as prayed").Background(p.Prayed).TextColor(p.OnPrayed).FontWeight(600).Grow(1)
					if prayed.Clicked() {
						a.prayedFromReminder()
					}
					if ui.Button(c, snooze).Grow(1).Clicked() {
						a.dismissReminder()
					}
				})
			})
		})
	})
}
