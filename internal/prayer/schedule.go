// Package prayer holds Pray Time's domain: regions, the day schedule, the
// countdown to the next prayer, completed prayers and due reminders. It has
// no UI and no I/O, so it is tested without a window.
package prayer

import (
	"fmt"
	"time"

	"github.com/uzuki-P/pray-time-mygo/internal/adhan"
)

// Names of the day schedule, in order. Sunrise is shown but is not a prayer.
const (
	Fajr    = "Fajr"
	Sunrise = "Sunrise"
	Dhuhr   = "Dhuhr"
	Asr     = "Asr"
	Maghrib = "Maghrib"
	Isha    = "Isha"
)

// ArabicNames are shown beside the Latin names.
var ArabicNames = map[string]string{
	Fajr:    "الفجر",
	Sunrise: "الشروق",
	Dhuhr:   "الظهر",
	Asr:     "العصر",
	Maghrib: "المغرب",
	Isha:    "العشاء",
}

// Time is one entry of the day schedule.
type Time struct {
	Name    string
	Instant time.Time // UTC, truncated to the minute
	Zone    *time.Location
}

// IsPrayer is false for Sunrise, which is never a countdown target or
// reminder subject.
func (t Time) IsPrayer() bool { return t.Name != Sunrise }

// DisplayTime is HH:MM in the region's timezone.
func (t Time) DisplayTime() string { return t.Instant.In(t.Zone).Format("15:04") }

// Equal compares two entries by name and instant.
func (t Time) Equal(o Time) bool { return t.Name == o.Name && t.Instant.Equal(o.Instant) }

// Day is the six times calculated for one region on one local date.
type Day struct {
	Date   Date
	Region Region
	Times  []Time
}

type Date struct {
	Year  int
	Month time.Month
	Day   int
}

func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

func (d Date) AddDays(n int) Date {
	return DateOf(time.Date(d.Year, d.Month, d.Day+n, 12, 0, 0, 0, time.UTC))
}

// String is the ISO date, as the state file stores it.
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day) }

// Next is the first prayer after from, skipping Sunrise, or false when
// every prayer of the day has arrived.
func (d Day) Next(from time.Time) (Time, bool) {
	for _, t := range d.Times {
		if t.IsPrayer() && t.Instant.After(from) {
			return t, true
		}
	}
	return Time{}, false
}

// LoadZone resolves a region's timezone, falling back to UTC.
func LoadZone(id string) *time.Location {
	if loc, err := time.LoadLocation(id); err == nil {
		return loc
	}
	return time.UTC
}

// Calculate computes the day schedule for region on date, with the
// Singapore method and the Shafi madhab.
func Calculate(date Date, region Region) Day {
	zone := LoadZone(region.TimezoneID)
	day := Day{Date: date, Region: region}
	t, ok := adhan.Calculate(region.Latitude, region.Longitude, date.Year, date.Month, date.Day, adhan.Singapore())
	if !ok {
		return day
	}
	for _, e := range []struct {
		name string
		at   time.Time
	}{{Fajr, t.Fajr}, {Sunrise, t.Sunrise}, {Dhuhr, t.Dhuhr}, {Asr, t.Asr}, {Maghrib, t.Maghrib}, {Isha, t.Isha}} {
		day.Times = append(day.Times, Time{Name: e.name, Instant: e.at.UTC().Truncate(time.Minute), Zone: zone})
	}
	return day
}

// LatestDue is the latest prayer that has arrived and is not completed.
func LatestDue(day Day, completed map[int64]bool, now time.Time) (Time, bool) {
	var found Time
	ok := false
	for _, t := range day.Times {
		if t.IsPrayer() && !t.Instant.After(now) && !completed[t.Instant.Unix()] {
			if !ok || t.Instant.After(found.Instant) {
				found, ok = t, true
			}
		}
	}
	return found, ok
}

// Countdown is the next prayer and the time left until it.
type Countdown struct {
	Next      Time
	Remaining time.Duration
	OK        bool
}

// DisplayRemaining is "1h 02m 03s".
func (c Countdown) DisplayRemaining() string {
	s := max(int64(c.Remaining/time.Second), 0)
	return fmt.Sprintf("%dh %02dm %02ds", s/3600, s/60%60, s%60)
}

// CompactRemaining is "1h 2m", "2m 3s" or "3s", as the popup shows it.
func CompactRemaining(d time.Duration) string {
	total := max(int64(d/time.Second), 0)
	h, m, s := total/3600, total%3600/60, total%60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// RailPosition is where now sits along the schedule, in row units: 0 is the
// first row's centre, 1.5 halfway between the second and third rows,
// clamped to the first and last rows.
func RailPosition(day Day, now time.Time) float32 {
	times := day.Times
	if len(times) == 0 || !now.After(times[0].Instant) {
		return 0
	}
	last := len(times) - 1
	if !now.Before(times[last].Instant) {
		return float32(last)
	}
	index := 0
	for i, t := range times {
		if !t.Instant.After(now) {
			index = i
		}
	}
	start := times[index].Instant.Unix()
	end := times[index+1].Instant.Unix()
	return float32(index) + float32(now.Unix()-start)/float32(end-start)
}

// DueDetail describes how long a due prayer has been due.
func DueDetail(t Time, now time.Time) string {
	if now.Before(t.Instant) {
		return "Starts at " + t.DisplayTime()
	}
	minutes := int64(now.Sub(t.Instant) / time.Minute)
	var ago string
	switch {
	case minutes < 1:
		ago = "just now"
	case minutes < 60:
		ago = fmt.Sprintf("%d min ago", minutes)
	default:
		ago = fmt.Sprintf("%dh %dm ago", minutes/60, minutes%60)
	}
	return "Due since " + t.DisplayTime() + " · " + ago
}

// Schedule is the live model behind every window: today's times for the
// selected region, the countdown and the completed prayers. Tick advances
// it; it rolls over at the region's local midnight.
type Schedule struct {
	now       func() time.Time
	region    Region
	zone      *time.Location
	today     Day
	tomorrow  Day
	countdown Countdown
	completed map[int64]bool
}

// NewSchedule starts a schedule at the clock's time; now may be nil for the
// system clock.
func NewSchedule(now func() time.Time, region Region) *Schedule {
	if now == nil {
		now = time.Now
	}
	s := &Schedule{now: now, completed: map[int64]bool{}}
	s.setRegion(region)
	return s
}

func (s *Schedule) Now() time.Time          { return s.now() }
func (s *Schedule) Region() Region          { return s.region }
func (s *Schedule) Today() Day              { return s.today }
func (s *Schedule) Countdown() Countdown    { return s.countdown }
func (s *Schedule) IsCompleted(t Time) bool { return s.completed[t.Instant.Unix()] }

// Completed returns the completed prayers' instants.
func (s *Schedule) Completed() []time.Time {
	var out []time.Time
	for _, t := range s.today.Times {
		if s.completed[t.Instant.Unix()] {
			out = append(out, t.Instant)
		}
	}
	return out
}

// CompletedSet is the completed set, keyed by Unix seconds, for LatestDue.
func (s *Schedule) CompletedSet() map[int64]bool { return s.completed }

func (s *Schedule) localDate() Date { return DateOf(s.now().In(s.zone)) }

func (s *Schedule) setRegion(r Region) {
	s.region = r
	s.zone = LoadZone(r.TimezoneID)
	date := s.localDate()
	s.today = Calculate(date, r)
	s.tomorrow = Calculate(date.AddDays(1), r)
	s.completed = map[int64]bool{}
	s.Tick()
}

// SetRegion recalculates for another region and clears completions.
func (s *Schedule) SetRegion(r Region) {
	if r == s.region {
		return
	}
	s.setRegion(r)
}

// RestoreCompleted keeps the instants that belong to today's schedule.
func (s *Schedule) RestoreCompleted(instants []time.Time) {
	s.completed = map[int64]bool{}
	for _, in := range instants {
		for _, t := range s.today.Times {
			if t.Instant.Equal(in) {
				s.completed[in.Unix()] = true
			}
		}
	}
}

// Tick rolls the day over when the local date changed and refreshes the
// countdown. It reports whether the day rolled over.
func (s *Schedule) Tick() bool {
	now := s.now()
	date := DateOf(now.In(s.zone))
	rolled := false
	if s.today.Date != date {
		s.today = Calculate(date, s.region)
		s.tomorrow = Calculate(date.AddDays(1), s.region)
		s.completed = map[int64]bool{}
		rolled = true
	}
	next, ok := s.today.Next(now)
	if !ok {
		next, ok = s.tomorrow.Next(now)
	}
	s.countdown = Countdown{Next: next, OK: ok}
	if ok {
		s.countdown.Remaining = next.Instant.Sub(now)
	}
	return rolled
}

// MarkPrayed completes (or un-completes) a prayer of today that has
// arrived. It reports whether anything changed.
func (s *Schedule) MarkPrayed(t Time, prayed bool) bool {
	if !t.IsPrayer() || t.Instant.After(s.now()) {
		return false
	}
	inToday := false
	for _, x := range s.today.Times {
		if x.Equal(t) {
			inToday = true
		}
	}
	if !inToday {
		return false
	}
	key := t.Instant.Unix()
	if s.completed[key] == prayed {
		return false
	}
	if prayed {
		s.completed[key] = true
	} else {
		delete(s.completed, key)
	}
	return true
}
