package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// Every control a click acts on shows the pointer, as the Kotlin app's did.
func TestClickableControlsShowPointer(t *testing.T) {
	a, _ := dueReminder(t)
	check := func(tt *ui.Tester, name string, x, y float32) {
		t.Helper()
		tt.Move(x, y)
		tt.Frame()
		if tt.Cursor() != ui.CursorPointer {
			t.Errorf("%s at %v,%v: cursor %v", name, x, y, tt.Cursor())
		}
	}
	named := func(tt *ui.Tester, names ...string) {
		t.Helper()
		for _, n := range names {
			r, ok := tt.Find(n)
			if !ok {
				t.Errorf("no %q", n)
				continue
			}
			check(tt, n, r.X+r.W/2, r.Y+r.H/2)
		}
	}
	// The switch and the select sit at the end of their setting's row.
	endOfRow := func(tt *ui.Tester, title string, x, dy float32) {
		t.Helper()
		r, ok := tt.Find(title)
		if !ok {
			t.Errorf("no %q", title)
			return
		}
		check(tt, title+" control", x, r.Y+r.H/2+dy)
	}

	popup := ui.NewTester(view(a, darkPalette, a.popupBody), popupWidth, popupHeight)
	named(popup, "Mark Asr as prayed", "Settings")

	reminder := ui.NewTester(view(a, darkPalette, a.reminderBody), reminderWidth, reminderHeight)
	named(reminder, "Mark as prayed", "Snooze 10 min")

	settings := ui.NewTester(view(a, darkPalette, a.settingsBody), settingsWidth, settingsHeight)
	named(settings, "Region", "Reminders", "General", "Close settings", "Medan Labuhan, Medan, Indonesia")
	a.tab = 1
	settings.Frame()
	named(settings, "Fewer snooze minutes", "More snooze minutes", "Show a test reminder")
	endOfRow(settings, "Snooze due reminders", 344, 8)
	endOfRow(settings, "Reminder position", 290, 6)
	a.tab = 2
	settings.Frame()
	named(settings, "System", "Light", "Dark")
	endOfRow(settings, "Start on login", 344, 8)
	endOfRow(settings, "Popup position", 290, 6)
}

func TestSnoozeStepperSaves(t *testing.T) {
	a := testApp(t, afternoon)
	a.tab = 1
	tt := ui.NewTester(view(a, darkPalette, a.settingsBody), settingsWidth, settingsHeight)
	if err := tt.Click("More snooze minutes"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.state.SnoozeMinutes() != 11 || !tt.HasText("11 min") {
		t.Fatalf("minutes %d, texts %q", a.state.SnoozeMinutes(), tt.Texts())
	}
	_ = tt.Click("Fewer snooze minutes")
	_ = tt.Click("Fewer snooze minutes")
	tt.Frame()
	if a.state.SnoozeMinutes() != 9 {
		t.Fatalf("minutes %d", a.state.SnoozeMinutes())
	}
}
