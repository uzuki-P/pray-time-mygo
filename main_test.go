package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/uzuki-P/pray-time-mygo/internal/prayer"
	"github.com/uzuki-P/pray-time-mygo/internal/store"
)

// The views run without windows here: the tester clicks and types as a user
// would. PRAYTIME_SNAPSHOTS=<dir> also saves what each view draws as PNGs.

func init() { registerFonts() }

// 2026-09-17 16:30 WIB: Asr (15:5x) has arrived and is due.
var afternoon = time.Date(2026, 9, 17, 9, 30, 0, 0, time.UTC)

func testApp(t *testing.T, at time.Time) *app {
	t.Helper()
	now := func() time.Time { return at }
	return newApp(store.Open(t.TempDir()), now)
}

func snapshot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("PRAYTIME_SNAPSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func view(a *app, p Palette, fn func(*ui.Context, Palette)) func(*ui.Context) {
	return func(c *ui.Context) {
		c.SetTheme(theme(p))
		fn(c, p)
	}
}

func TestPopupShowsScheduleAndMarksPrayed(t *testing.T) {
	a := testApp(t, afternoon)
	tt := ui.NewTester(view(a, darkPalette, a.popupBody), popupWidth, popupHeight)
	for _, s := range []string{"Next prayer", "Maghrib", "Fajr", "Sunrise", "Isha", "Due", "Medan Johor, Medan, Indonesia", "Settings"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "popup-dark")

	if err := tt.Click("Mark Asr as prayed"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	var asr prayer.Time
	for _, x := range a.sched.Today().Times {
		if x.Name == prayer.Asr {
			asr = x
		}
	}
	if !a.sched.IsCompleted(asr) {
		t.Fatal("asr not completed")
	}
	if got := a.state.Completed(a.sched.Today().Date); len(got) != 1 {
		t.Fatalf("saved %v", got)
	}
	// Dhuhr is now the latest due prayer.
	if !tt.HasText("Due") {
		t.Error("no due prayer after Asr")
	}
	// A future prayer cannot be marked.
	_ = tt.Click("Mark Isha as prayed")
	if len(a.sched.Completed()) != 1 {
		t.Error("future prayer marked")
	}

	light := ui.NewTester(view(a, lightPalette, a.popupBody), popupWidth, popupHeight)
	snapshot(t, light, "popup-light")
}

func TestReminderShowsDuePrayer(t *testing.T) {
	a := testApp(t, afternoon)
	due, ok := prayer.LatestDue(a.sched.Today(), a.sched.CompletedSet(), afternoon)
	a.rem.OnTick(due, ok, true)
	tt := ui.NewTester(view(a, darkPalette, a.reminderBody), reminderWidth, reminderHeight)
	for _, s := range []string{"Asr", "Mark as prayed", "Snooze 10 min"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "reminder-dark")
	a.state.SaveSnoozeEnabled(false)
	tt.Frame()
	if !tt.HasText("Dismiss") {
		t.Errorf("no Dismiss in %q", tt.Texts())
	}
	light := ui.NewTester(view(a, lightPalette, a.reminderBody), reminderWidth, reminderHeight)
	snapshot(t, light, "reminder-light")
}

func dueReminder(t *testing.T) (*app, prayer.Time) {
	t.Helper()
	a := testApp(t, afternoon)
	due, ok := prayer.LatestDue(a.sched.Today(), a.sched.CompletedSet(), afternoon)
	if !ok {
		t.Fatal("nothing due")
	}
	a.rem.OnTick(due, ok, true)
	return a, due
}

func TestReminderMarkAsPrayedPlaysThenMarks(t *testing.T) {
	a, due := dueReminder(t)
	tt := ui.NewTester(view(a, darkPalette, a.reminderBody), reminderWidth, reminderHeight)
	if err := tt.Click("Mark as prayed"); err != nil {
		t.Fatal(err)
	}
	if a.anim != outcomePrayed {
		t.Fatal("no mark-as-prayed sequence")
	}
	tt.Advance(300 * time.Millisecond)
	snapshot(t, tt, "reminder-prayed-flood")
	if a.sched.IsCompleted(due) {
		t.Fatal("marked before the sequence ended")
	}
	tt.Advance(600 * time.Millisecond)
	snapshot(t, tt, "reminder-prayed-check")
	if !tt.HasText("Asr marked as prayed") {
		t.Errorf("texts %q", tt.Texts())
	}
	tt.Advance(time.Second)
	if a.anim != outcomeNone || !a.sched.IsCompleted(due) {
		t.Fatalf("anim %v, completed %v", a.anim, a.sched.IsCompleted(due))
	}
}

func TestReminderSnoozePlaysThenSnoozes(t *testing.T) {
	a, due := dueReminder(t)
	tt := ui.NewTester(view(a, lightPalette, a.reminderBody), reminderWidth, reminderHeight)
	if err := tt.Click("Snooze 10 min"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(700 * time.Millisecond)
	snapshot(t, tt, "reminder-snoozed-droop")
	if !tt.HasText("Okay… back in 10 min") {
		t.Errorf("texts %q", tt.Texts())
	}
	if !a.rem.Visible {
		t.Fatal("snoozed before the sequence ended")
	}
	tt.Advance(2 * time.Second)
	if a.anim != outcomeNone || a.rem.Visible || a.sched.IsCompleted(due) {
		t.Fatalf("anim %v, visible %v", a.anim, a.rem.Visible)
	}
}

func TestSpringSettles(t *testing.T) {
	if d := droopSpring.settle(1); d < 900 || d > 1300 {
		t.Errorf("droop settles in %v ms", d)
	}
	if v := droopSpring.at(droopMillis+440, 440, 0, 1); v < 0.99 || v > 1.01 {
		t.Errorf("droop at its end %v", v)
	}
	if v := pressSpring.at(80, 80, 0.92, 1); v != 0.92 {
		t.Errorf("press starts at %v", v)
	}
}

func TestSettingsSearchAndSelectRegion(t *testing.T) {
	a := testApp(t, afternoon)
	tt := ui.NewTester(view(a, darkPalette, a.settingsBody), settingsWidth, settingsHeight)
	for _, s := range []string{"Settings", "Prayer times are calculated for", "SUGGESTED", "Medan Labuhan"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "settings-region")

	if err := tt.Click("Medan Labuhan, Medan, Indonesia"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.sched.Region().Name != "Medan Labuhan" {
		t.Fatalf("region %v", a.sched.Region())
	}
	if r, ok := a.state.Region(append(prayer.BuiltInRegions, a.state.Recent()...)); !ok || r.Name != "Medan Labuhan" {
		t.Fatal("region not saved")
	}
	if !tt.HasText("RECENT") {
		t.Error("no recents")
	}

	// Results the world list returned for the query show after the local matches.
	a.query = "tokyo"
	a.results = []prayer.Region{{Name: "Tokyo", AdminName: "Tokyo", Country: "Japan", Latitude: 35.6895, Longitude: 139.6917, TimezoneID: "Asia/Tokyo"}}
	a.resultsFor = "tokyo"
	tt.Frame()
	if !tt.HasText("Tokyo") {
		t.Errorf("no Tokyo in %q", tt.Texts())
	}
	a.results, a.resultsFor, a.query = nil, "zzzz", "zzzz"
	tt.Frame()
	if !tt.HasText(`Nothing matches "zzzz". Try a city, district, or kelurahan name.`) {
		t.Errorf("texts %q", tt.Texts())
	}

	a.query = ""
	a.tab = 1
	tt.Frame()
	snapshot(t, tt, "settings-reminders")
	if !tt.HasText("Snooze due reminders") || !tt.HasText("10 min") {
		t.Errorf("texts %q", tt.Texts())
	}
	a.tab = 2
	tt.Frame()
	snapshot(t, tt, "settings-general")
	if !tt.HasText("Start on login") || !tt.HasText("Popup position") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestAnchorPosition(t *testing.T) {
	area := mygo.Rectangle{X: 0, Y: 30, Width: 1920, Height: 1050}
	cases := map[store.Anchor][2]int{
		store.TopLeft:     {12, 42},
		store.TopRight:    {1920 - 280 - 12, 42},
		store.MiddleLeft:  {12, 30 + (1050-372)/2},
		store.BottomRight: {1920 - 280 - 12, 30 + 1050 - 372 - 12},
	}
	for anchor, want := range cases {
		x, y := anchorPosition(anchor, area, popupWidth, popupHeight, screenMargin)
		if x != want[0] || y != want[1] {
			t.Errorf("%s: %d,%d want %v", anchor, x, y, want)
		}
	}
}
