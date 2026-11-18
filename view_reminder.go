package main

import (
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/uzuki-P/pray-time-mygo/internal/prayer"
)

const (
	cardWidth  = 304
	cardInset  = 12 // room around the card for its shadow
	cardRadius = 18
)

var (
	pressSpring = spring{0.4, stiffnessMedium}
	bumpSpring  = spring{0.45, stiffnessMediumLow}
	droopSpring = spring{0.3, stiffnessLow}
	droopMillis = droopSpring.settle(1)
)

// cardMotion is where the reminder card's animations are at a moment.
type cardMotion struct {
	prayedPress, snoozePress float32
	// Mark as prayed: a teal flood from the button, a confetti burst, a
	// drawn check, a small bump, and the card lifting away.
	flood, confetti, check, bump, lift float32
	// Snooze: a shake of the head, then the card drops on one corner,
	// greys out, and sinks.
	shake, droop, grey, sink float32
	snoozed                  bool
	snoozedFade              float32
	end                      float64 // when the sequence is over
}

func restingCard() cardMotion {
	return cardMotion{prayedPress: 1, snoozePress: 1, bump: 1}
}

// cardMotionAt plays the Kotlin app's sequences: t is milliseconds since
// the button was pressed.
func cardMotionAt(o outcome, t float64) cardMotion {
	m := restingCard()
	switch o {
	case outcomePrayed:
		if t < 80 {
			m.prayedPress = tween(t, 0, 80, 1, 0.92, fastOutSlowIn)
		} else {
			m.prayedPress = pressSpring.at(t, 80, 0.92, 1)
		}
		m.flood = tween(t, 80, 360, 0, 1, fastOutSlowIn)
		m.confetti = tween(t, 200, 900, 0, 1, linearOutSlowIn)
		switch {
		case t < 530:
			m.bump = tween(t, 440, 90, 1, 1.04, fastOutSlowIn)
		default:
			m.bump = bumpSpring.at(t, 530, 1.04, 1)
		}
		m.check = tween(t, 440, 320, 0, 1, fastOutSlowIn)
		m.lift = tween(t, 1320, 260, 0, 1, fastOutLinearIn)
		m.end = 1580
	case outcomeSnoozed:
		if t < 80 {
			m.snoozePress = tween(t, 0, 80, 1, 0.95, fastOutSlowIn)
		} else {
			m.snoozePress = tween(t, 80, 160, 0.95, 1, fastOutSlowIn)
		}
		m.shake = keyframes(t, 80, []keyframe{{0, 0}, {60, -5}, {130, 5}, {200, -3}, {270, 3}, {360, 0}})
		m.snoozed = t >= 440
		m.snoozedFade = tween(t, 440, 220, 0, 1, fastOutSlowIn)
		m.grey = tween(t, 440, 420, 0, 1, fastOutSlowIn)
		m.droop = droopSpring.at(t, 440, 0, 1)
		sinkAt := 440 + droopMillis + 250
		m.sink = tween(t, sinkAt, 420, 0, 1, fastOutLinearIn)
		m.end = sinkAt + 420
	}
	return m
}

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
	// The window is transparent: the card floats in it with room to move.
	c.Root().Background(ui.Transparent)
	t := a.reminderPrayer()
	if t.Name == "" {
		return
	}
	m := restingCard()
	if a.anim != outcomeNone {
		if a.animStart.IsZero() {
			a.animStart = c.Now()
		}
		elapsed := float64(c.Now().Sub(a.animStart).Microseconds()) / 1000
		m = cardMotionAt(a.anim, elapsed)
		if elapsed >= m.end {
			a.finishAnimation()
			return
		}
		c.AnimationFrame()
	}

	snooze := "Dismiss"
	if a.state.SnoozeEnabled() {
		snooze = "Snooze " + strconv.Itoa(a.state.SnoozeMinutes()) + " min"
	}
	sky := skyColors[t.Name]

	ui.Column(c).Fill().Padding(cardInset).AlignItems(ui.Start).Children(func() {
		wrap := ui.Column(c).Width(cardWidth)
		scale := m.bump * (1 - 0.08*m.lift)
		wrap.Scale(scale, scale).Rotate(3.5*m.droop+2*m.sink).Translate(m.shake, -6*m.lift+22*m.sink)
		if m.droop > 0 {
			// A sign losing one nail: it swings from its top-left corner.
			wrap.TransformOrigin(0.04, 0)
		}
		wrap.Opacity(1 - max(m.lift, m.sink))
		wrap.Children(func() {
			card := ui.Row(c).Radius(cardRadius).Background(p.Surface).Clip().AlignItems(ui.Stretch).
				Shadow(0, 3, 14, 0, ui.RGBA(0, 0, 0, 0.32))
			card.Children(func() {
				ui.Box(c).Width(4).Background(greyed(sky, m.grey))
				ui.Column(c).Grow(1).Padding(12, 14, 14, 14).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Children(func() {
						display(c, t.Name, 24, 700).TextColor(p.OnSurface).Grow(1)
						display(c, prayer.ArabicNames[t.Name], 17, 500).TextColor(p.Muted())
					})
					if m.snoozed {
						figures(c, a.snoozedLabel(), 12, 400).TextColor(p.Muted()).Opacity(m.snoozedFade)
					} else {
						figures(c, prayer.DueDetail(t, a.sched.Now()), 12, 400).TextColor(p.Muted())
					}
					ui.Box(c).Height(12)
					ui.Row(c).Gap(8).Children(func() {
						prayed := pill(c, "Mark as prayed", greyed(p.Prayed, m.grey), p.OnPrayed, true, m.prayedPress)
						if prayed.Clicked() {
							b, cb := prayed.Bounds(), card.Bounds()
							a.burst = [2]float32{b.X + b.W/2 - cb.X, b.Y + b.H/2 - cb.Y}
							a.prayedFromReminder()
						}
						if pill(c, snooze, p.Raised, p.OnSurface, false, m.snoozePress).Clicked() {
							a.dismissReminder()
						}
					})
				})
				if m.grey > 0 {
					ui.Box(c).Absolute().Fill().Background(p.Surface.Alpha(0.25 * m.grey))
				}
				if m.flood > 0 {
					burst, flood := a.burst, m.flood
					ui.Box(c).Absolute().Fill().Draw(func(pt *ui.Painter, r ui.Rect) {
						rad := float32(math.Hypot(float64(r.W), float64(r.H))) * flood
						x, y := r.X+burst[0], r.Y+burst[1]
						pt.Fill(ui.Rect{X: x - rad, Y: y - rad, W: rad * 2, H: rad * 2}, p.Prayed, rad)
					})
					progress := m.check
					ui.Column(c).Absolute().Fill().Center().Gap(2).Opacity(max(progress, 0.001)).Children(func() {
						ui.Box(c).Size(40, 40).Draw(func(pt *ui.Painter, r ui.Rect) {
							strokeCheck(pt, r.X, r.Y, 40, 3.5, progress, p.OnPrayed)
						})
						display(c, t.Name+" marked as prayed", 16, 700).TextColor(p.OnPrayed)
					})
				}
			})
			if m.confetti > 0 && m.confetti < 1 {
				colors := []ui.Color{p.Accent, ui.RGB(255, 255, 255), sky, p.Due}
				burst, progress := a.burst, m.confetti
				ui.Box(c).Absolute().Fill().Draw(func(pt *ui.Painter, r ui.Rect) {
					drawConfetti(pt, progress, r.X+burst[0], r.Y+burst[1], colors)
				})
			}
		})
	})
}

// pill is the reminder's rounded button, pressed in by scale.
func pill(c *ui.Context, label string, bg, fg ui.Color, check bool, scale float32) ui.Element {
	b := ui.Row(c).Padding(7, 12).Radius(10).Background(bg).Gap(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Role(ui.RoleButton).Focusable().Label(label)
	if scale != 1 {
		b.Scale(scale, scale)
	}
	b.Children(func() {
		if check {
			ui.Box(c).Size(14, 14).Draw(func(pt *ui.Painter, r ui.Rect) {
				strokeCheck(pt, r.X, r.Y, 14, 2, 1, fg)
			})
		}
		body(c, label, 12, 600).TextColor(fg)
	})
	return b
}

const confettiPieces = 22

// drawConfetti bursts the pieces out from x, y as t goes 0 to 1: each flies
// its own way, slows down, falls a little, shrinks, and fades at the end.
func drawConfetti(pt *ui.Painter, t, x, y float32, colors []ui.Color) {
	travel := 1 - (1-t)*(1-t)
	alpha := float32(1)
	if t >= 0.6 {
		alpha = (1 - t) / 0.4
	}
	for i := 0; i < confettiPieces; i++ {
		// A fixed, uneven spread, so every burst looks the same but loose.
		angle := (float64(i)*360/confettiPieces + float64(i*37%23) - 11) * math.Pi / 180
		distance := float64(52+i*53%48) * float64(travel)
		fall := 22 * t * t
		cx := x + float32(math.Cos(angle)*distance)
		cy := y + float32(math.Sin(angle)*distance) + fall
		radius := float32(3)
		if i%3 == 0 {
			radius = 4.5
		}
		radius *= 1 - 0.4*t
		pt.Fill(ui.Rect{X: cx - radius, Y: cy - radius, W: radius * 2, H: radius * 2},
			colors[i%len(colors)].Alpha(alpha), radius)
	}
}
