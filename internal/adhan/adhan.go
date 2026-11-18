// Package adhan is a Go port of the parts of Adhan Java 1.2.1
// (com.batoulapps.adhan) that Pray Time uses: the astronomical solar
// calculations and PrayerTimes for the angle-based methods. The Kotlin app
// used that library with the Singapore method and the Shafi madhab, so this
// port reproduces its results to the minute.
package adhan

import (
	"math"
	"time"
)

// Madhab picks the shadow length used for Asr.
type Madhab int

const (
	Shafi Madhab = iota
	Hanafi
)

func (m Madhab) shadowLength() float64 {
	if m == Hanafi {
		return 2
	}
	return 1
}

// HighLatitudeRule bounds Fajr and Isha where twilight never ends.
type HighLatitudeRule int

const (
	MiddleOfTheNight HighLatitudeRule = iota
	SeventhOfTheNight
	TwilightAngle
)

// Adjustments are minutes added to each time.
type Adjustments struct {
	Fajr, Sunrise, Dhuhr, Asr, Maghrib, Isha int
}

// Parameters configure a calculation, as Adhan's CalculationParameters.
type Parameters struct {
	FajrAngle         float64
	IshaAngle         float64
	IshaInterval      int // minutes after Maghrib; when > 0 it replaces IshaAngle
	Madhab            Madhab
	HighLatitudeRule  HighLatitudeRule
	Adjustments       Adjustments
	MethodAdjustments Adjustments
}

// Singapore is Adhan's CalculationMethod.SINGAPORE: Fajr 20°, Isha 18°,
// Dhuhr one minute after transit.
func Singapore() Parameters {
	return Parameters{
		FajrAngle:         20,
		IshaAngle:         18,
		Madhab:            Shafi,
		HighLatitudeRule:  MiddleOfTheNight,
		MethodAdjustments: Adjustments{Dhuhr: 1},
	}
}

func (p Parameters) nightPortions() (fajr, isha float64) {
	switch p.HighLatitudeRule {
	case SeventhOfTheNight:
		return 1.0 / 7.0, 1.0 / 7.0
	case TwilightAngle:
		return p.FajrAngle / 60.0, p.IshaAngle / 60.0
	default:
		return 0.5, 0.5
	}
}

// Times are the six times of one day, in UTC, rounded to the minute.
type Times struct {
	Fajr, Sunrise, Dhuhr, Asr, Maghrib, Isha time.Time
}

// Calculate returns the prayer times at latitude/longitude for the civil
// date year-month-day, or ok=false where the sun does not rise or set.
func Calculate(latitude, longitude float64, year int, month time.Month, day int, p Parameters) (Times, bool) {
	date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	tomorrow := date.AddDate(0, 0, 1)

	solar := newSolarTime(date, latitude, longitude)
	transit, okTransit := timeOn(solar.transit, date)
	sunrise, okSunrise := timeOn(solar.sunrise, date)
	sunset, okSunset := timeOn(solar.sunset, date)
	tomorrowSolar := newSolarTime(tomorrow, latitude, longitude)
	tomorrowSunrise, okTomorrow := timeOn(tomorrowSolar.sunrise, tomorrow)
	if !okTransit || !okSunrise || !okSunset || !okTomorrow {
		return Times{}, false
	}

	asr, okAsr := timeOn(solar.afternoon(p.Madhab.shadowLength()), date)
	if !okAsr {
		return Times{}, false
	}

	night := tomorrowSunrise.Sub(sunset)
	fajrPortion, ishaPortion := p.nightPortions()

	fajr, okFajr := timeOn(solar.hourAngle(-p.FajrAngle, false), date)
	safeFajr := sunrise.Add(-wholeSeconds(fajrPortion * night.Seconds()))
	if !okFajr || fajr.Before(safeFajr) {
		fajr = safeFajr
	}

	var isha time.Time
	if p.IshaInterval > 0 {
		isha = sunset.Add(time.Duration(p.IshaInterval) * time.Minute)
	} else {
		var okIsha bool
		isha, okIsha = timeOn(solar.hourAngle(-p.IshaAngle, true), date)
		safeIsha := sunset.Add(wholeSeconds(ishaPortion * night.Seconds()))
		if !okIsha || isha.After(safeIsha) {
			isha = safeIsha
		}
	}

	adjust := func(t time.Time, user, method int) time.Time {
		return roundedMinute(t.Add(time.Duration(user+method) * time.Minute))
	}
	a, m := p.Adjustments, p.MethodAdjustments
	return Times{
		Fajr:    adjust(fajr, a.Fajr, m.Fajr),
		Sunrise: adjust(sunrise, a.Sunrise, m.Sunrise),
		Dhuhr:   adjust(transit, a.Dhuhr, m.Dhuhr),
		Asr:     adjust(asr, a.Asr, m.Asr),
		Maghrib: adjust(sunset, a.Maghrib, m.Maghrib),
		Isha:    adjust(isha, a.Isha, m.Isha),
	}, true
}

// wholeSeconds truncates toward zero, as Java's (long) cast does.
func wholeSeconds(s float64) time.Duration {
	return time.Duration(int64(s)) * time.Second
}

// timeOn turns hours after midnight UTC into a time on date, dropping the
// fraction of a second as Adhan's TimeComponents does.
func timeOn(hours float64, date time.Time) (time.Time, bool) {
	if math.IsNaN(hours) || math.IsInf(hours, 0) {
		return time.Time{}, false
	}
	h := math.Floor(hours)
	m := math.Floor((hours - h) * 60)
	s := math.Floor((hours - (h + m/60)) * 3600)
	return date.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second), true
}

// roundedMinute rounds to the nearest minute: 30 seconds or more round up.
func roundedMinute(t time.Time) time.Time {
	t = t.Truncate(time.Second)
	sec := t.Second()
	t = t.Add(-time.Duration(sec) * time.Second)
	if sec >= 30 {
		t = t.Add(time.Minute)
	}
	return t
}

type solarCoordinates struct {
	declination, rightAscension, apparentSiderealTime float64
}

func newSolarCoordinates(julianDay float64) solarCoordinates {
	T := julianCentury(julianDay)
	L0 := meanSolarLongitude(T)
	Lp := meanLunarLongitude(T)
	omega := ascendingLunarNodeLongitude(T)
	lambda := radians(apparentSolarLongitude(T, L0))
	theta0 := meanSiderealTime(T)
	dPsi := nutationInLongitude(L0, Lp, omega)
	dEpsilon := nutationInObliquity(L0, Lp, omega)
	epsilon0 := meanObliquityOfTheEcliptic(T)
	epsilonApp := radians(apparentObliquityOfTheEcliptic(T, epsilon0))
	return solarCoordinates{
		declination:          degrees(math.Asin(math.Sin(epsilonApp) * math.Sin(lambda))),
		rightAscension:       unwindAngle(degrees(math.Atan2(math.Cos(epsilonApp)*math.Sin(lambda), math.Cos(lambda)))),
		apparentSiderealTime: theta0 + (((dPsi * 3600) * math.Cos(radians(epsilon0+dEpsilon))) / 3600),
	}
}

type solarTime struct {
	latitude, longitude         float64
	solar, prevSolar, nextSolar solarCoordinates
	approximateTransit          float64
	transit, sunrise, sunset    float64
}

func newSolarTime(date time.Time, latitude, longitude float64) *solarTime {
	jd := julianDay(date.Year(), int(date.Month()), date.Day())
	s := &solarTime{
		latitude:  latitude,
		longitude: longitude,
		prevSolar: newSolarCoordinates(jd - 1),
		solar:     newSolarCoordinates(jd),
		nextSolar: newSolarCoordinates(jd + 1),
	}
	s.approximateTransit = approximateTransit(longitude, s.solar.apparentSiderealTime, s.solar.rightAscension)
	const solarAltitude = -50.0 / 60.0
	s.transit = correctedTransit(s.approximateTransit, longitude, s.solar.apparentSiderealTime,
		s.solar.rightAscension, s.prevSolar.rightAscension, s.nextSolar.rightAscension)
	s.sunrise = s.hourAngle(solarAltitude, false)
	s.sunset = s.hourAngle(solarAltitude, true)
	return s
}

func (s *solarTime) hourAngle(angle float64, afterTransit bool) float64 {
	return correctedHourAngle(s.approximateTransit, angle, s.latitude, s.longitude, afterTransit,
		s.solar.apparentSiderealTime, s.solar.rightAscension, s.prevSolar.rightAscension,
		s.nextSolar.rightAscension, s.solar.declination, s.prevSolar.declination, s.nextSolar.declination)
}

func (s *solarTime) afternoon(shadowLength float64) float64 {
	tangent := math.Abs(s.latitude - s.solar.declination)
	inverse := shadowLength + math.Tan(radians(tangent))
	return s.hourAngle(degrees(math.Atan(1.0/inverse)), true)
}

func radians(d float64) float64 { return d * math.Pi / 180 }
func degrees(r float64) float64 { return r * 180 / math.Pi }

func normalizeWithBound(value, max float64) float64 {
	return value - (max * math.Floor(value/max))
}

func unwindAngle(value float64) float64 { return normalizeWithBound(value, 360) }

func closestAngle(angle float64) float64 {
	if angle >= -180 && angle <= 180 {
		return angle
	}
	return angle - (360 * javaRound(angle/360))
}

// javaRound is Java's Math.round: halves round toward positive infinity.
func javaRound(x float64) float64 { return math.Floor(x + 0.5) }

func julianDay(year, month, day int) float64 {
	Y, M := year, month
	if month <= 2 {
		Y, M = year-1, month+12
	}
	D := float64(day)
	A := Y / 100
	B := 2 - A + (A / 4)
	i0 := int(365.25 * float64(Y+4716))
	i1 := int(30.6001 * float64(M+1))
	return float64(i0) + float64(i1) + D + float64(B) - 1524.5
}

func julianCentury(jd float64) float64 { return (jd - 2451545.0) / 36525 }

func meanSolarLongitude(T float64) float64 {
	return unwindAngle(280.4664567 + 36000.76983*T + 0.0003032*math.Pow(T, 2))
}

func meanLunarLongitude(T float64) float64 {
	return unwindAngle(218.3165 + 481267.8813*T)
}

func apparentSolarLongitude(T, L0 float64) float64 {
	longitude := L0 + solarEquationOfTheCenter(T, meanSolarAnomaly(T))
	omega := 125.04 - (1934.136 * T)
	return unwindAngle(longitude - 0.00569 - (0.00478 * math.Sin(radians(omega))))
}

func ascendingLunarNodeLongitude(T float64) float64 {
	return unwindAngle(125.04452 - 1934.136261*T + 0.0020708*math.Pow(T, 2) + math.Pow(T, 3)/450000)
}

func meanSolarAnomaly(T float64) float64 {
	return unwindAngle(357.52911 + 35999.05029*T - 0.0001537*math.Pow(T, 2))
}

func solarEquationOfTheCenter(T, M float64) float64 {
	Mrad := radians(M)
	term1 := (1.914602 - (0.004817 * T) - (0.000014 * math.Pow(T, 2))) * math.Sin(Mrad)
	term2 := (0.019993 - (0.000101 * T)) * math.Sin(2*Mrad)
	term3 := 0.000289 * math.Sin(3*Mrad)
	return term1 + term2 + term3
}

func meanObliquityOfTheEcliptic(T float64) float64 {
	return 23.439291 - 0.013004167*T - 0.0000001639*math.Pow(T, 2) + 0.0000005036*math.Pow(T, 3)
}

func apparentObliquityOfTheEcliptic(T, epsilon0 float64) float64 {
	O := 125.04 - (1934.136 * T)
	return epsilon0 + (0.00256 * math.Cos(radians(O)))
}

func meanSiderealTime(T float64) float64 {
	JD := (T * 36525) + 2451545.0
	theta := 280.46061837 + 360.98564736629*(JD-2451545) + 0.000387933*math.Pow(T, 2) - math.Pow(T, 3)/38710000
	return unwindAngle(theta)
}

func nutationInLongitude(L0, Lp, omega float64) float64 {
	term1 := (-17.2 / 3600) * math.Sin(radians(omega))
	term2 := (1.32 / 3600) * math.Sin(2*radians(L0))
	term3 := (0.23 / 3600) * math.Sin(2*radians(Lp))
	term4 := (0.21 / 3600) * math.Sin(2*radians(omega))
	return term1 - term2 - term3 + term4
}

func nutationInObliquity(L0, Lp, omega float64) float64 {
	term1 := (9.2 / 3600) * math.Cos(radians(omega))
	term2 := (0.57 / 3600) * math.Cos(2*radians(L0))
	term3 := (0.10 / 3600) * math.Cos(2*radians(Lp))
	term4 := (0.09 / 3600) * math.Cos(2*radians(omega))
	return term1 + term2 + term3 - term4
}

func altitudeOfCelestialBody(phi, delta, H float64) float64 {
	term1 := math.Sin(radians(phi)) * math.Sin(radians(delta))
	term2 := math.Cos(radians(phi)) * math.Cos(radians(delta)) * math.Cos(radians(H))
	return degrees(math.Asin(term1 + term2))
}

func approximateTransit(L, theta0, alpha2 float64) float64 {
	Lw := L * -1
	return normalizeWithBound((alpha2+Lw-theta0)/360, 1)
}

func correctedTransit(m0, L, theta0, alpha2, alpha1, alpha3 float64) float64 {
	Lw := L * -1
	theta := unwindAngle(theta0 + (360.985647 * m0))
	alpha := unwindAngle(interpolateAngles(alpha2, alpha1, alpha3, m0))
	H := closestAngle(theta - Lw - alpha)
	dm := H / -360
	return (m0 + dm) * 24
}

func correctedHourAngle(m0, h0, latitude, longitude float64, afterTransit bool,
	theta0, alpha2, alpha1, alpha3, delta2, delta1, delta3 float64) float64 {
	Lw := longitude * -1
	term1 := math.Sin(radians(h0)) - (math.Sin(radians(latitude)) * math.Sin(radians(delta2)))
	term2 := math.Cos(radians(latitude)) * math.Cos(radians(delta2))
	H0 := degrees(math.Acos(term1 / term2))
	m := m0 - (H0 / 360)
	if afterTransit {
		m = m0 + (H0 / 360)
	}
	theta := unwindAngle(theta0 + (360.985647 * m))
	alpha := unwindAngle(interpolateAngles(alpha2, alpha1, alpha3, m))
	delta := interpolate(delta2, delta1, delta3, m)
	H := theta - Lw - alpha
	h := altitudeOfCelestialBody(latitude, delta, H)
	term3 := h - h0
	term4 := 360 * math.Cos(radians(delta)) * math.Cos(radians(latitude)) * math.Sin(radians(H))
	dm := term3 / term4
	return (m + dm) * 24
}

func interpolate(y2, y1, y3, n float64) float64 {
	a := y2 - y1
	b := y3 - y2
	c := b - a
	return y2 + ((n / 2) * (a + b + (n * c)))
}

func interpolateAngles(y2, y1, y3, n float64) float64 {
	a := unwindAngle(y2 - y1)
	b := unwindAngle(y3 - y2)
	c := b - a
	return y2 + ((n / 2) * (a + b + (n * c)))
}
