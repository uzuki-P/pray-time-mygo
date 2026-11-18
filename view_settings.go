package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/uzuki-P/pray-time-mygo/internal/cities"
	"github.com/uzuki-P/pray-time-mygo/internal/prayer"
	"github.com/uzuki-P/pray-time-mygo/internal/store"
)

var themeModes = []store.ThemeMode{store.ThemeSystem, store.ThemeLight, store.ThemeDark}

func anchorLabels() []string {
	out := make([]string, len(store.Anchors))
	for i, a := range store.Anchors {
		out[i] = a.Label()
	}
	return out
}

func anchorFromLabel(label string) store.Anchor {
	for _, a := range store.Anchors {
		if a.Label() == label {
			return a
		}
	}
	return store.TopRight
}

func coordinates(r prayer.Region) string {
	f := func(v float64, pos, neg rune) string {
		h := pos
		if v < 0 {
			h = neg
		}
		return fmt.Sprintf("%.2f°%c", math.Abs(v), h)
	}
	return f(r.Latitude, 'N', 'S') + " " + f(r.Longitude, 'E', 'W') + " · " + r.TimezoneID
}

func (a *app) settingsView(c *ui.Context) {
	p := a.palette()
	c.SetTheme(theme(p))
	if c.Shortcut(0, ui.KeyEscape) && a.settingsWin != nil {
		a.settingsWin.Hide()
	}
	a.settingsBody(c, p)
}

func (a *app) settingsBody(c *ui.Context, p Palette) {
	ui.Column(c).Fill().Background(p.Surface).Padding(12, 12, 16, 18).Children(func() {
		ui.Row(c).AlignItems(ui.Center).DragWindow().Padding(0, 0, 0, 0).Children(func() {
			display(c, "Settings", 22, 700).TextColor(p.OnSurface).Grow(1)
			if ui.Button(c, "✕").Label("Close settings").Size(30, 30).Clicked() && a.settingsWin != nil {
				a.settingsWin.Hide()
			}
		})
		ui.Box(c).Height(8)
		ui.Segmented(c, &a.tab, "Region", "Reminders", "General").Margin(0, 6, 0, 0)
		ui.Box(c).Height(16)
		ui.Column(c).Grow(1).Margin(0, 6, 0, 0).Children(func() {
			switch a.tab {
			case 1:
				a.remindersTab(c, p)
			case 2:
				a.generalTab(c, p)
			default:
				a.regionTab(c, p)
			}
		})
	})
}

func (a *app) regionTab(c *ui.Context, p Palette) {
	selected := a.sched.Region()
	ui.Column(c).Radius(12).Background(p.Raised).Padding(10, 14).Gap(1).Children(func() {
		body(c, "Prayer times are calculated for", 11, 400).TextColor(p.Muted())
		body(c, selected.Name, 16, 600).TextColor(p.OnSurface)
		body(c, selected.Subtitle(), 12, 400).TextColor(p.Muted())
		figures(c, coordinates(selected), 11, 400).TextColor(p.Faint())
	})
	ui.Box(c).Height(12)
	if ui.SearchField(c, &a.query).Placeholder("Search for a city or district").Label("Search regions").Changed() {
		a.search(a.query)
	}
	ui.Box(c).Height(6)

	q := strings.TrimSpace(a.query)
	ui.Scroll(c.Key("regions")).Grow(1).Children(func() {
		if q == "" {
			if len(a.recents) > 0 {
				groupLabel(c, p, "Recent")
				a.regionRows(c, "recent", p, a.recents, selected)
			}
			groupLabel(c, p, "Suggested")
			a.regionRows(c, "suggested", p, prayer.BuiltInRegions, selected)
			return
		}
		local := a.localMatches(q)
		done := a.resultsFor == q
		merged := local
		if done {
			for _, r := range a.results {
				if !containsPlace(local, r) {
					merged = append(merged, r)
				}
			}
		}
		if done && len(merged) == 0 {
			msg := fmt.Sprintf("Nothing matches \"%s\". Try a city, district, or kelurahan name.", q)
			if a.searchErr != "" {
				msg = "The world list is unavailable: " + a.searchErr
			}
			body(c, msg, 12, 400).TextColor(p.Muted()).Padding(12, 10)
			return
		}
		a.regionRows(c, "results", p, merged, selected)
		if !done {
			groupLabel(c, p, "Searching the world list…")
		}
	})
}

func containsPlace(list []prayer.Region, r prayer.Region) bool {
	for _, x := range list {
		if x.SamePlace(r) {
			return true
		}
	}
	return false
}

// localMatches is the built-in list and the recents matching q, as the
// Kotlin app showed them before the world list answers.
func (a *app) localMatches(q string) []prayer.Region {
	lq := strings.ToLower(q)
	var out []prayer.Region
	for _, r := range prayer.BuiltInRegions {
		if strings.Contains(strings.ToLower(r.DisplayName()), lq) {
			out = append(out, r)
		}
	}
	for _, r := range a.recents {
		if strings.Contains(strings.ToLower(r.DisplayName()), lq) && !containsPlace(prayer.BuiltInRegions, r) {
			out = append(out, r)
		}
	}
	return out
}

// search looks q up in the world list 250 ms after the last keystroke, off
// the main thread; only the latest search's results are kept.
func (a *app) search(q string) {
	gen := a.searchGen.Add(1)
	q = strings.TrimSpace(q)
	if q == "" {
		a.results, a.resultsFor, a.searchErr = nil, "", ""
		return
	}
	win := a.settingsWin
	go func() {
		time.Sleep(250 * time.Millisecond)
		if a.searchGen.Load() != gen {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		results, err := a.catalog.Search(ctx, q, cities.DefaultLimit)
		apply := func() {
			if a.searchGen.Load() != gen {
				return
			}
			a.results, a.resultsFor, a.searchErr = results, q, ""
			if err != nil {
				a.searchErr = err.Error()
			}
		}
		if win != nil {
			win.Update(apply)
		} else {
			mygo.RunOnMain(apply)
		}
	}()
}

func groupLabel(c *ui.Context, p Palette, s string) {
	body(c, strings.ToUpper(s), 10, 700).TextColor(p.Faint()).LetterSpacing(0.8).Padding(12, 10, 4, 10)
}

func (a *app) regionRows(c *ui.Context, group string, p Palette, list []prayer.Region, selected prayer.Region) {
	for i, r := range list {
		cc := c.Key(group + ":" + strconv.Itoa(i))
		isSelected := r.SamePlace(selected)
		row := ui.Row(cc).Padding(7, 10).Radius(10).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(r.DisplayName())
		if isSelected {
			row.Background(p.Accent.Alpha(0.14))
		} else if row.Hovered() {
			row.Background(p.OnSurface.Alpha(0.05))
		}
		row.Children(func() {
			ui.Column(cc).Grow(1).Shrink(1).Children(func() {
				color := p.OnSurface
				if isSelected {
					color = p.Accent
				}
				body(cc, r.Name, 13, 600).TextColor(color).SingleLine()
				body(cc, r.Subtitle(), 11, 400).TextColor(p.Muted()).SingleLine()
			})
			if isSelected {
				body(cc, "✓", 14, 700).TextColor(p.Accent)
			}
		})
		if row.Clicked() {
			a.selectRegion(r)
		}
	}
}

// settingRow is a title and description with a control at the end.
func settingRow(c *ui.Context, p Palette, title, description string, enabled bool, control func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			color := p.OnSurface
			if !enabled {
				color = p.Faint()
			}
			body(c, title, 13, 600).TextColor(color)
			if description != "" {
				body(c, description, 11, 400).TextColor(p.Muted())
			}
		})
		control()
	})
}

func (a *app) remindersTab(c *ui.Context, p Palette) {
	ui.Scroll(c.Key("reminders")).Grow(1).Gap(18).Children(func() {
		desc := "Dismissing hides the reminder until the next prayer is due."
		if a.snoozeOn {
			desc = "Snoozing hides the reminder, and it returns while the prayer is still not marked as prayed."
		}
		settingRow(c, p, "Snooze due reminders", desc, true, func() {
			if ui.Switch(c, &a.snoozeOn).Label("Snooze due reminders").Changed() {
				a.state.SaveSnoozeEnabled(a.snoozeOn)
			}
		})
		settingRow(c, p, "Snooze for", "", a.snoozeOn, func() {
			figures(c, fmt.Sprintf("%d min", int(a.snoozeMinutes)), 13, 600).TextColor(p.OnSurface)
			if ui.Stepper(c, &a.snoozeMinutes, store.MinReminderMinutes, store.MaxReminderMinutes, 1).
				Label("Snooze minutes").Disabled(!a.snoozeOn).Changed() {
				a.state.SaveSnoozeMinutes(int(a.snoozeMinutes))
				a.snoozeMinutes = float64(a.state.SnoozeMinutes())
			}
		})
		settingRow(c, p, "Reminder position", "Where due reminders appear.", true, func() {
			if ui.Select(c, &a.reminderPos, anchorLabels()).Label("Reminder position").Width(150).Changed() {
				a.state.SaveReminderAnchor(anchorFromLabel(a.reminderPos))
			}
		})
		if ui.Button(c, "Show a test reminder").Clicked() {
			a.showTestReminder()
		}
	})
}

func (a *app) generalTab(c *ui.Context, p Palette) {
	ui.Scroll(c.Key("general")).Grow(1).Gap(18).Children(func() {
		settingRow(c, p, "Theme", "", true, func() {
			if ui.Segmented(c, &a.themeIndex, "System", "Light", "Dark").Changed() {
				a.state.SaveThemeMode(themeModes[a.themeIndex])
				a.applyIcons()
				a.invalidateAll()
			}
		})
		settingRow(c, p, "Start on login", "Opens Pray Time in the tray when you sign in.", true, func() {
			if ui.Switch(c, &a.autostart).Label("Start on login").Changed() {
				a.autostartErr = ""
				if err := mygo.App.SetOpenAtLogin(a.autostart); err != nil {
					a.autostartErr = err.Error()
				}
				a.autostart = mygo.App.OpenAtLogin()
			}
		})
		if a.autostartErr != "" {
			body(c, a.autostartErr, 11, 400).TextColor(p.Due)
		}
		settingRow(c, p, "Popup position", "Where the prayer times popup opens.", true, func() {
			if ui.Select(c, &a.popupPos, anchorLabels()).Label("Popup position").Width(150).Changed() {
				a.state.SaveMainAnchor(anchorFromLabel(a.popupPos))
			}
		})
	})
}
