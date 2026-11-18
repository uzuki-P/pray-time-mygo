package main

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// The Kotlin app's motion is Compose's: these are its easing curves and its
// springs, so the reminder's sequences play with the same timing. Times are
// milliseconds since a sequence started.
var (
	fastOutSlowIn   = ui.CubicBezier(0.4, 0, 0.2, 1)
	linearOutSlowIn = ui.CubicBezier(0, 0, 0.2, 1)
	fastOutLinearIn = ui.CubicBezier(0.4, 0, 1, 1)
)

// tween goes from `from` to `to` between start and start+dur along ease.
func tween(t, start, dur float64, from, to float32, ease ui.Easing) float32 {
	switch {
	case t <= start:
		return from
	case t >= start+dur:
		return to
	}
	return from + (to-from)*ease(float32((t-start)/dur))
}

// spring is a Compose spring (dampingRatio, stiffness) at rest at `from`
// when it starts towards `to`.
type spring struct{ ratio, stiffness float64 }

// Compose's stiffness presets.
const (
	stiffnessMedium    = 1500
	stiffnessMediumLow = 400
	stiffnessLow       = 200
)

func (s spring) at(t, start float64, from, to float32) float32 {
	if t <= start {
		return from
	}
	dt := (t - start) / 1000
	x0 := float64(from - to)
	w := math.Sqrt(s.stiffness)
	z := s.ratio
	var x float64
	if z < 1 {
		wd := w * math.Sqrt(1-z*z)
		x = math.Exp(-z*w*dt) * (x0*math.Cos(wd*dt) + z*w*x0/wd*math.Sin(wd*dt))
	} else {
		x = (x0 + w*x0*dt) * math.Exp(-w*dt)
	}
	return to + float32(x)
}

// settle is how long, in milliseconds, the spring takes over a distance
// until it stays within Compose's visibility threshold (0.01) of its target.
func (s spring) settle(distance float64) float64 {
	w := math.Sqrt(s.stiffness)
	z := min(s.ratio, 0.999)
	wd := w * math.Sqrt(1-z*z)
	amp := math.Abs(distance) * math.Sqrt(1+(z*w/wd)*(z*w/wd))
	if amp <= 0.01 {
		return 0
	}
	return math.Log(amp/0.01) / (z * w) * 1000
}

type keyframe struct {
	at    float64
	value float32
}

// keyframes plays linear segments between frames, from start, as Compose's
// keyframes spec does with its default easing.
func keyframes(t, start float64, frames []keyframe) float32 {
	t -= start
	if t <= frames[0].at {
		return frames[0].value
	}
	for i := 1; i < len(frames); i++ {
		a, b := frames[i-1], frames[i]
		if t < b.at {
			return a.value + (b.value-a.value)*float32((t-a.at)/(b.at-a.at))
		}
	}
	return frames[len(frames)-1].value
}

// greyed mixes a colour towards its own grey, as the Kotlin app's
// saturation blend took the colour out of the snoozed card.
func greyed(c ui.Color, amount float32) ui.Color {
	if amount <= 0 {
		return c
	}
	l := uint8(0.299*float32(c.R) + 0.587*float32(c.G) + 0.114*float32(c.B))
	g := ui.RGB(l, l, l)
	g.A = c.A
	return c.Mix(g, amount)
}

// checkPoints is the check mark in a size×size box at x, y: from its short
// arm to the tip of its long one.
func checkPoints(x, y, size float32) [3][2]float32 {
	return [3][2]float32{
		{x + size*0.18, y + size*0.53},
		{x + size*0.42, y + size*0.76},
		{x + size*0.84, y + size*0.28},
	}
}

// strokeCheck draws the check mark up to progress (0 to 1) of its length.
func strokeCheck(pt *ui.Painter, x, y, size, width, progress float32, c ui.Color) {
	if progress <= 0 {
		return
	}
	p := checkPoints(x, y, size)
	l1 := float32(math.Hypot(float64(p[1][0]-p[0][0]), float64(p[1][1]-p[0][1])))
	l2 := float32(math.Hypot(float64(p[2][0]-p[1][0]), float64(p[2][1]-p[1][1])))
	d := (l1 + l2) * min(progress, 1)
	lerp := func(a, b [2]float32, f float32) [2]float32 {
		return [2]float32{a[0] + (b[0]-a[0])*f, a[1] + (b[1]-a[1])*f}
	}
	if d <= l1 {
		pt.StrokePolyline([][2]float32{p[0], lerp(p[0], p[1], d/l1)}, width, c)
		return
	}
	pt.StrokePolyline([][2]float32{p[0], p[1], lerp(p[1], p[2], (d-l1)/l2)}, width, c)
}
