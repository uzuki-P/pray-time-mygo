package prayer

import (
	"regexp"
	"testing"
	"time"
)

// Ports of the Kotlin app's ScheduleViewModelTest, ReminderControllerTest,
// ScheduleRailTest and PrayerTimeCalculatorTest.

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

var (
	// 2026-09-17 09:00 WIB: morning, after Fajr and Sunrise.
	morning = mustTime("2026-09-17T02:00:00Z")
	// 2026-09-18 01:00 WIB: past Isha of Sep 17.
	lateNight = mustTime("2026-09-17T18:00:00Z")
)

func schedule(at time.Time) (*clock, *Schedule) {
	c := &clock{at}
	return c, NewSchedule(c.now, MedanJohor)
}

func find(day Day, name string) Time {
	for _, t := range day.Times {
		if t.Name == name {
			return t
		}
	}
	panic("no " + name)
}

func TestCountdownTargetsNextPrayerWithPositiveRemaining(t *testing.T) {
	_, s := schedule(morning)
	cd := s.Countdown()
	if !cd.OK || cd.Next.Name != Dhuhr || cd.Remaining <= 0 {
		t.Fatalf("countdown %+v", cd)
	}
}

func TestAfterIshaNextIsTomorrowFajr(t *testing.T) {
	_, s := schedule(lateNight)
	cd := s.Countdown()
	if cd.Next.Name != Fajr || cd.Remaining <= 0 {
		t.Fatalf("countdown %+v", cd)
	}
}

func TestDisplayRemainingFormatsHoursMinutesSeconds(t *testing.T) {
	_, s := schedule(morning)
	if d := s.Countdown().DisplayRemaining(); !regexp.MustCompile(`^\d+h \d{2}m \d{2}s$`).MatchString(d) {
		t.Fatal(d)
	}
}

func TestTickRollsOverToNextDay(t *testing.T) {
	c, s := schedule(morning)
	first := s.Today().Date
	c.t = lateNight
	if !s.Tick() {
		t.Fatal("no rollover")
	}
	if s.Today().Date != first.AddDays(1) || s.Countdown().Next.Name != Fajr {
		t.Fatalf("date %v next %v", s.Today().Date, s.Countdown().Next.Name)
	}
}

func TestTickRollsBackwardsAcrossDays(t *testing.T) {
	c, s := schedule(lateNight)
	c.t = morning
	s.Tick()
	want := DateOf(lateNight.In(LoadZone("Asia/Jakarta"))).AddDays(-1)
	if s.Today().Date != want {
		t.Fatalf("date %v, want %v", s.Today().Date, want)
	}
}

func TestMarkPrayed(t *testing.T) {
	_, s := schedule(morning)
	fajr := find(s.Today(), Fajr)
	s.MarkPrayed(fajr, true)
	if !s.IsCompleted(fajr) {
		t.Fatal("fajr not completed")
	}
	s.MarkPrayed(fajr, false)
	if s.IsCompleted(fajr) {
		t.Fatal("fajr still completed")
	}
	isha := find(s.Today(), Isha)
	if s.MarkPrayed(isha, true) || s.IsCompleted(isha) {
		t.Fatal("future prayer completed")
	}
	sunrise := find(s.Today(), Sunrise)
	if s.MarkPrayed(sunrise, true) || s.IsCompleted(sunrise) {
		t.Fatal("sunrise completed")
	}
}

func TestCompletedClearsOnDateRollover(t *testing.T) {
	c, s := schedule(morning)
	s.MarkPrayed(find(s.Today(), Fajr), true)
	c.t = lateNight
	s.Tick()
	if len(s.Completed()) != 0 {
		t.Fatal(s.Completed())
	}
}

func TestSetRegionRecomputesTimesAndKeepsNextTarget(t *testing.T) {
	_, s := schedule(morning)
	if s.Region() != MedanJohor {
		t.Fatal("default region")
	}
	sg := Region{"Singapore", "Central", "Singapore", 1.3521, 103.8198, "Asia/Singapore"}
	s.SetRegion(sg)
	if s.Region() != sg || s.Today().Region != sg {
		t.Fatal("region not set")
	}
	if cd := s.Countdown(); cd.Next.Name != Dhuhr || cd.Remaining <= 0 {
		t.Fatalf("countdown %+v", cd)
	}
}

func TestRestoreCompletedKeepsOnlyToday(t *testing.T) {
	_, s := schedule(morning)
	fajr := find(s.Today(), Fajr)
	s.RestoreCompleted([]time.Time{fajr.Instant, fajr.Instant.Add(-24 * time.Hour)})
	if got := s.Completed(); len(got) != 1 || !got[0].Equal(fajr.Instant) {
		t.Fatal(got)
	}
}

func TestDaySchedule(t *testing.T) {
	day := Calculate(Date{2026, 9, 17}, MedanJohor)
	names := []string{Fajr, Sunrise, Dhuhr, Asr, Maghrib, Isha}
	if len(day.Times) != 6 {
		t.Fatal(day.Times)
	}
	for i, tm := range day.Times {
		if tm.Name != names[i] {
			t.Errorf("entry %d is %s", i, tm.Name)
		}
		if i > 0 && !tm.Instant.After(day.Times[i-1].Instant) {
			t.Errorf("%s not after %s", tm.Name, day.Times[i-1].Name)
		}
		if DateOf(tm.Instant.In(tm.Zone)) != (Date{2026, 9, 17}) {
			t.Errorf("%s on %v", tm.Name, tm.Instant.In(tm.Zone))
		}
		if !regexp.MustCompile(`^\d{2}:\d{2}$`).MatchString(tm.DisplayTime()) {
			t.Errorf("display %q", tm.DisplayTime())
		}
	}
	if n, _ := day.Next(find(day, Sunrise).Instant); n.Name != Dhuhr {
		t.Errorf("next after sunrise is %s", n.Name)
	}
	if _, ok := day.Next(find(day, Isha).Instant); ok {
		t.Error("a prayer after Isha")
	}
}

func TestRail(t *testing.T) {
	day := Calculate(Date{2026, 9, 17}, MedanJohor)
	if p := RailPosition(day, find(day, Fajr).Instant.Add(-time.Hour)); p != 0 {
		t.Errorf("before fajr %v", p)
	}
	if p := RailPosition(day, find(day, Isha).Instant.Add(time.Hour)); p != 5 {
		t.Errorf("after isha %v", p)
	}
	if p := RailPosition(day, find(day, Asr).Instant); p != 3 {
		t.Errorf("at asr %v", p)
	}
	asr, maghrib := find(day, Asr).Instant, find(day, Maghrib).Instant
	if p := RailPosition(day, asr.Add(maghrib.Sub(asr)/2)); p < 3.49 || p > 3.51 {
		t.Errorf("midway %v", p)
	}
}

func TestDueDetail(t *testing.T) {
	asr := find(Calculate(Date{2026, 9, 17}, MedanJohor), Asr)
	cases := map[time.Duration]string{
		30 * time.Second:  "Due since " + asr.DisplayTime() + " · just now",
		48 * time.Minute:  "Due since " + asr.DisplayTime() + " · 48 min ago",
		125 * time.Minute: "Due since " + asr.DisplayTime() + " · 2h 5m ago",
		-time.Minute:      "Starts at " + asr.DisplayTime(),
	}
	for d, want := range cases {
		if got := DueDetail(asr, asr.Instant.Add(d)); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
}

func TestCompactRemaining(t *testing.T) {
	for d, want := range map[time.Duration]string{
		2*time.Hour + 5*time.Minute + 9*time.Second: "2h 5m",
		3*time.Minute + 4*time.Second:               "3m 4s",
		9 * time.Second:                             "9s",
		-time.Second:                                "0s",
	} {
		if got := CompactRemaining(d); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
}

var (
	utc     = time.UTC
	asrT    = Time{Asr, mustTime("2026-09-21T06:30:00Z"), utc}
	maghrib = Time{Maghrib, mustTime("2026-09-21T12:00:00Z"), utc}
)

func reminder() (*clock, *Reminder) {
	c := &clock{mustTime("2026-09-21T10:00:00Z")}
	return c, NewReminder(c.now)
}

func TestReminderShowsWhenPrayerBecomesDueAndResets(t *testing.T) {
	_, r := reminder()
	r.OnTick(asrT, true, true)
	if !r.Visible || !r.Current.Equal(asrT) {
		t.Fatal("not shown")
	}
	r.OnTick(Time{}, false, true)
	if r.Visible || r.HasCurrent {
		t.Fatal("not reset")
	}
	r.OnTick(asrT, true, true)
	if !r.Visible {
		t.Fatal("unmarking did not show again")
	}
}

func TestReminderSnooze(t *testing.T) {
	for _, minutes := range []int{DefaultReminderMinutes, 2} {
		c, r := reminder()
		r.OnTick(asrT, true, true)
		r.Dismiss(true, minutes)
		c.t = c.t.Add(time.Duration(minutes-1) * time.Minute)
		r.OnTick(asrT, true, true)
		if r.Visible {
			t.Fatalf("%d: visible before snooze ends", minutes)
		}
		c.t = c.t.Add(time.Minute)
		r.OnTick(asrT, true, true)
		if !r.Visible {
			t.Fatalf("%d: hidden after snooze ends", minutes)
		}
	}
}

func TestReminderDismissWithoutSnoozeStaysHidden(t *testing.T) {
	c, r := reminder()
	r.OnTick(asrT, true, true)
	r.Dismiss(false, 10)
	c.t = c.t.Add(time.Hour)
	r.OnTick(asrT, true, true)
	if r.Visible {
		t.Fatal("visible")
	}
	_, r2 := reminder()
	r2.OnTick(asrT, true, false)
	r2.Dismiss(false, 10)
	r2.OnTick(asrT, true, false)
	if r2.Visible {
		t.Fatal("visible with snooze off")
	}
}

func TestReminderNewDuePrayerShowsWhileSnoozed(t *testing.T) {
	_, r := reminder()
	r.OnTick(asrT, true, true)
	r.Dismiss(true, 10)
	r.OnTick(maghrib, true, true)
	if !r.Visible || !r.Current.Equal(maghrib) {
		t.Fatal("maghrib not shown")
	}
}

func TestLatestDueSkipsSunriseCompletedAndFuture(t *testing.T) {
	fajr := Time{Fajr, mustTime("2026-09-21T00:00:00Z"), utc}
	sunrise := Time{Sunrise, mustTime("2026-09-21T01:00:00Z"), utc}
	dhuhr := Time{Dhuhr, mustTime("2026-09-21T03:00:00Z"), utc}
	day := Day{Date{2026, 9, 21}, Region{}, []Time{fajr, sunrise, dhuhr, asrT}}
	now := mustTime("2026-09-21T04:00:00Z")
	done := func(ts ...Time) map[int64]bool {
		m := map[int64]bool{}
		for _, t := range ts {
			m[t.Instant.Unix()] = true
		}
		return m
	}
	if got, _ := LatestDue(day, done(), now); !got.Equal(dhuhr) {
		t.Errorf("got %s", got.Name)
	}
	if got, _ := LatestDue(day, done(dhuhr), now); !got.Equal(fajr) {
		t.Errorf("got %s", got.Name)
	}
	if _, ok := LatestDue(day, done(fajr, dhuhr), now); ok {
		t.Error("a due prayer")
	}
	if got, _ := LatestDue(day, done(fajr, dhuhr), asrT.Instant); !got.Equal(asrT) {
		t.Errorf("got %s", got.Name)
	}
}
