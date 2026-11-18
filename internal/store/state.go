// Package store persists Pray Time's state as key=value lines in
// $XDG_CONFIG_HOME/praytime/state.properties (falling back to
// ~/.config/praytime), the same file and keys the Kotlin app used, so either
// app reads the other's selected region, completions, recents and settings.
package store

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/uzuki-P/pray-time-mygo/internal/prayer"
)

const (
	recentLimit = 10
	// MinReminderMinutes and MaxReminderMinutes bound the snooze length.
	MinReminderMinutes = 1
	MaxReminderMinutes = 120
)

// Anchor is where a popup goes on the screen.
type Anchor string

const (
	TopLeft     Anchor = "TOP_LEFT"
	TopRight    Anchor = "TOP_RIGHT"
	MiddleLeft  Anchor = "MIDDLE_LEFT"
	MiddleRight Anchor = "MIDDLE_RIGHT"
	BottomLeft  Anchor = "BOTTOM_LEFT"
	BottomRight Anchor = "BOTTOM_RIGHT"
)

// Anchors lists every anchor, in the settings grid's order.
var Anchors = []Anchor{TopLeft, TopRight, MiddleLeft, MiddleRight, BottomLeft, BottomRight}

// Label is how settings name the anchor.
func (a Anchor) Label() string {
	switch a {
	case TopLeft:
		return "Top left"
	case MiddleLeft:
		return "Middle left"
	case MiddleRight:
		return "Middle right"
	case BottomLeft:
		return "Bottom left"
	case BottomRight:
		return "Bottom right"
	default:
		return "Top right"
	}
}

// ThemeMode picks light, dark or the system's appearance.
type ThemeMode string

const (
	ThemeSystem ThemeMode = "SYSTEM"
	ThemeLight  ThemeMode = "LIGHT"
	ThemeDark   ThemeMode = "DARK"
)

// State reads and writes the state file. Its methods are safe from any
// goroutine; writes go to a temporary file renamed over the old one.
type State struct {
	mu     sync.Mutex
	dir    string
	cached map[string]string
}

// ConfigDir is $XDG_CONFIG_HOME/praytime, or ~/.config/praytime.
func ConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if strings.TrimSpace(base) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "praytime")
}

// Open uses dir, creating it; an empty dir means ConfigDir.
func Open(dir string) *State {
	if dir == "" {
		dir = ConfigDir()
	}
	_ = os.MkdirAll(dir, 0o755)
	return &State{dir: dir}
}

// Dir is the folder the state lives in.
func (s *State) Dir() string { return s.dir }

func (s *State) file() string { return filepath.Join(s.dir, "state.properties") }

func (s *State) read() map[string]string {
	if s.cached != nil {
		return s.cached
	}
	m := map[string]string{}
	if data, err := os.ReadFile(s.file()); err == nil {
		for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			if i := strings.IndexByte(line, '='); i > 0 {
				m[line[:i]] = line[i+1:]
			}
		}
	}
	s.cached = m
	return m
}

func (s *State) get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.read()[key]
	return v, ok
}

func (s *State) update(fn func(m map[string]string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.read()
	m := make(map[string]string, len(old)+2)
	for k, v := range old {
		m[k] = v
	}
	fn(m)
	s.cached = m
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(k + "=" + m[k])
	}
	tmp := s.file() + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, s.file()); err != nil {
		_ = os.WriteFile(s.file(), []byte(b.String()), 0o644)
		_ = os.Remove(tmp)
	}
}

func (s *State) set(key, value string) { s.update(func(m map[string]string) { m[key] = value }) }

// Region is the saved region, looked up by name among regions.
func (s *State) Region(regions []prayer.Region) (prayer.Region, bool) {
	name, ok := s.get("region")
	if !ok {
		return prayer.Region{}, false
	}
	for _, r := range regions {
		if r.Name == name {
			return r, true
		}
	}
	return prayer.Region{}, false
}

func (s *State) SaveRegion(r prayer.Region) { s.set("region", r.Name) }

// Completed is the completed prayers' instants, if saved for today.
func (s *State) Completed(today prayer.Date) []time.Time {
	if d, _ := s.get("completedDate"); d != today.String() {
		return nil
	}
	v, _ := s.get("completed")
	var out []time.Time
	for _, part := range strings.Split(v, ",") {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(part)); err == nil {
			out = append(out, t.UTC())
		}
	}
	return out
}

func (s *State) SaveCompleted(today prayer.Date, instants []time.Time) {
	parts := make([]string, len(instants))
	for i, t := range instants {
		parts[i] = t.UTC().Format("2006-01-02T15:04:05Z")
	}
	s.update(func(m map[string]string) {
		m["completedDate"] = today.String()
		m["completed"] = strings.Join(parts, ",")
	})
}

// Recent is the recently selected regions, newest first.
func (s *State) Recent() []prayer.Region {
	var out []prayer.Region
	for i := 0; i < recentLimit; i++ {
		v, ok := s.get("recent." + strconv.Itoa(i))
		if !ok {
			continue
		}
		if r, ok := parseRegion(v); ok {
			out = append(out, r)
		}
	}
	return out
}

func (s *State) SaveRecent(r prayer.Region) {
	list := []prayer.Region{r}
	for _, old := range s.Recent() {
		if !old.SamePlace(r) {
			list = append(list, old)
		}
	}
	if len(list) > recentLimit {
		list = list[:recentLimit]
	}
	s.update(func(m map[string]string) {
		for k := range m {
			if strings.HasPrefix(k, "recent.") {
				delete(m, k)
			}
		}
		for i, x := range list {
			m["recent."+strconv.Itoa(i)] = encodeRegion(x)
		}
	})
}

// SnoozeEnabled is whether dismissing a due reminder snoozes it.
func (s *State) SnoozeEnabled() bool {
	v, _ := s.get("reminder.enabled")
	return v != "false"
}

func (s *State) SaveSnoozeEnabled(on bool) { s.set("reminder.enabled", strconv.FormatBool(on)) }

// SnoozeMinutes is the snooze length, within its bounds.
func (s *State) SnoozeMinutes() int {
	v, _ := s.get("reminder.minutes")
	if n, err := strconv.Atoi(v); err == nil && n >= MinReminderMinutes && n <= MaxReminderMinutes {
		return n
	}
	return prayer.DefaultReminderMinutes
}

func (s *State) SaveSnoozeMinutes(n int) {
	s.set("reminder.minutes", strconv.Itoa(min(max(n, MinReminderMinutes), MaxReminderMinutes)))
}

func (s *State) anchor(key string) Anchor {
	v, _ := s.get(key)
	for _, a := range Anchors {
		if string(a) == v {
			return a
		}
	}
	return TopRight
}

func (s *State) MainAnchor() Anchor           { return s.anchor("main.anchor") }
func (s *State) SaveMainAnchor(a Anchor)      { s.set("main.anchor", string(a)) }
func (s *State) ReminderAnchor() Anchor       { return s.anchor("reminder.anchor") }
func (s *State) SaveReminderAnchor(a Anchor)  { s.set("reminder.anchor", string(a)) }
func (s *State) SaveThemeMode(mode ThemeMode) { s.set("theme", string(mode)) }

func (s *State) ThemeMode() ThemeMode {
	switch v, _ := s.get("theme"); ThemeMode(v) {
	case ThemeLight:
		return ThemeLight
	case ThemeDark:
		return ThemeDark
	default:
		return ThemeSystem
	}
}

func encodeRegion(r prayer.Region) string {
	clean := func(s string) string { return strings.ReplaceAll(s, "|", " ") }
	return strings.Join([]string{
		clean(r.Name), clean(r.AdminName), clean(r.Country),
		strconv.FormatFloat(r.Latitude, 'f', -1, 64),
		strconv.FormatFloat(r.Longitude, 'f', -1, 64),
		r.TimezoneID,
	}, "|")
}

func parseRegion(v string) (prayer.Region, bool) {
	f := strings.Split(v, "|")
	if len(f) < 6 {
		return prayer.Region{}, false
	}
	lat, err1 := strconv.ParseFloat(f[3], 64)
	lon, err2 := strconv.ParseFloat(f[4], 64)
	if err1 != nil || err2 != nil {
		return prayer.Region{}, false
	}
	return prayer.Region{Name: f[0], AdminName: f[1], Country: f[2], Latitude: lat, Longitude: lon, TimezoneID: f[5]}, true
}
