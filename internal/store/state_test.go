package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uzuki-P/pray-time-mygo/internal/prayer"
)

// A file the Kotlin app wrote reads back, and what this app writes keeps
// the same keys and formats.
func TestReadsKotlinStateFile(t *testing.T) {
	dir := t.TempDir()
	kotlin := "region=Binjai\ncompletedDate=2026-09-17\ncompleted=2026-09-16T22:08:00Z,2026-09-17T05:24:00Z\n" +
		"recent.0=Binjai|Binjai|Indonesia|3.6042|98.4892|Asia/Jakarta\nreminder.enabled=false\nreminder.minutes=15\n" +
		"main.anchor=BOTTOM_LEFT\ntheme=DARK"
	if err := os.WriteFile(filepath.Join(dir, "state.properties"), []byte(kotlin), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Open(dir)
	if r, ok := s.Region(append(prayer.BuiltInRegions, s.Recent()...)); !ok || r.Name != "Binjai" {
		t.Fatalf("region %v %v", r, ok)
	}
	if got := s.Completed(prayer.Date{Year: 2026, Month: 9, Day: 17}); len(got) != 2 {
		t.Fatalf("completed %v", got)
	}
	if got := s.Completed(prayer.Date{Year: 2026, Month: 9, Day: 18}); got != nil {
		t.Fatalf("completed on another day %v", got)
	}
	if s.SnoozeEnabled() || s.SnoozeMinutes() != 15 || s.MainAnchor() != BottomLeft || s.ReminderAnchor() != TopRight || s.ThemeMode() != ThemeDark {
		t.Fatal("settings")
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	day := prayer.Date{Year: 2026, Month: 9, Day: 17}
	at := time.Date(2026, 9, 16, 22, 8, 0, 0, time.UTC)
	s.SaveCompleted(day, []time.Time{at})
	for i := 0; i < 12; i++ {
		s.SaveRecent(prayer.Region{Name: string(rune('A' + i)), Country: "X", TimezoneID: "UTC", Latitude: 1.5})
	}
	s.SaveRecent(prayer.Region{Name: "C", Country: "X", TimezoneID: "UTC", Latitude: 1.5})
	s.SaveSnoozeMinutes(500)

	again := Open(dir)
	if got := again.Completed(day); len(got) != 1 || !got[0].Equal(at) {
		t.Fatal(got)
	}
	recent := again.Recent()
	if len(recent) != 10 || recent[0].Name != "C" || recent[1].Name != "L" {
		t.Fatalf("recent %v", recent)
	}
	if again.SnoozeMinutes() != MaxReminderMinutes {
		t.Fatal(again.SnoozeMinutes())
	}
}
