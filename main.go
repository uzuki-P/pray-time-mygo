// Pray Time shows today's prayer times for a chosen region in a tray popup,
// counts down to the next prayer, and reminds the user when a prayer is due.
// It is the MyGo (native UI) port of the Kotlin Compose Desktop app.
package main

import (
	"log"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"
	_ "time/tzdata" // region timezones on Windows, which has no zoneinfo

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/uzuki-P/pray-time-mygo/internal/cities"
	"github.com/uzuki-P/pray-time-mygo/internal/prayer"
	"github.com/uzuki-P/pray-time-mygo/internal/store"
)

const (
	popupWidth  = 280
	popupHeight = 372
	// The reminder's window holds its card with room around it for the
	// shadow, and below it for the snooze droop and sink.
	reminderWidth  = cardWidth + 2*cardInset
	reminderHeight = 148 + 28
	settingsWidth  = 380
	settingsHeight = 580
	screenMargin   = 12
)

type outcome int

const (
	outcomeNone outcome = iota
	outcomePrayed
	outcomeSnoozed
)

// app is all of Pray Time's state. Everything but searchGen is read and
// changed on the main thread: by the views, window and tray callbacks, and
// tick, which the ticker goroutine runs there.
type app struct {
	state   *store.State
	sched   *prayer.Schedule
	rem     *prayer.Reminder
	catalog *cities.Catalog

	tray         *mygo.Tray
	showItem     *mygo.MenuItem
	trayMenu     *mygo.Menu
	popup        *mygo.Window
	reminderWin  *mygo.Window
	settingsWin  *mygo.Window
	quitting     bool
	noTray       bool
	popupHidAt   time.Time // when blur last hid the popup
	suppressBlur time.Time

	// The reminder window.
	testReminder bool
	testPrayer   prayer.Time
	anim         outcome    // the card's mark-as-prayed or snooze sequence
	animStart    time.Time  // set by the first frame of the sequence
	burst        [2]float32 // the mark-as-prayed button's center in the card

	// The settings window.
	tab           int
	query         string
	searchGen     atomic.Int64
	results       []prayer.Region
	resultsFor    string
	searchErr     string
	recents       []prayer.Region
	snoozeOn      bool
	snoozeMinutes float64
	reminderPos   string
	popupPos      string
	themeIndex    int
	autostart     bool
	autostartErr  string
}

func newApp(state *store.State, now func() time.Time) *app {
	recents := state.Recent()
	region, ok := state.Region(append(append([]prayer.Region{}, prayer.BuiltInRegions...), recents...))
	if !ok {
		region = prayer.MedanJohor
	}
	sched := prayer.NewSchedule(now, region)
	// Restore before anything can save the empty initial set.
	sched.RestoreCompleted(state.Completed(sched.Today().Date))
	a := &app{
		state:   state,
		sched:   sched,
		rem:     prayer.NewReminder(now),
		catalog: cities.New(state.Dir()),
		recents: recents,
	}
	a.loadSettings()
	return a
}

func (a *app) loadSettings() {
	a.snoozeOn = a.state.SnoozeEnabled()
	a.snoozeMinutes = float64(a.state.SnoozeMinutes())
	a.reminderPos = a.state.ReminderAnchor().Label()
	a.popupPos = a.state.MainAnchor().Label()
	switch a.state.ThemeMode() {
	case store.ThemeLight:
		a.themeIndex = 1
	case store.ThemeDark:
		a.themeIndex = 2
	default:
		a.themeIndex = 0
	}
}

func main() {
	preferX11OnWayland()
	mygo.App.SetName("Pray Time")
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running instance opens its popup
	}
	registerFonts()
	a := newApp(store.Open(""), nil)
	mygo.App.OnSecondInstance(func([]string, string) { a.showPopup() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnQuitCanceled(func() { a.quitting = false })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.tray != nil {
			a.tray.Destroy()
		}
		a.catalog.Close()
	})
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// panelOptions are the options of the app's frameless, always-on-top
// windows, which stay out of the taskbar as the Kotlin app's did.
func panelOptions(title string, w, h int, view func(*ui.Context)) mygo.WindowOptions {
	return mygo.WindowOptions{
		Title:             title,
		Width:             w,
		Height:            h,
		Hidden:            true,
		Frameless:         true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		CornerRadius:      14,
		Content:           ui.View(view),
	}
}

func (a *app) ready() {
	a.popup = mygo.NewWindow(panelOptions("Pray Time", popupWidth, popupHeight, a.popupView))
	// The reminder draws its own rounded card, with a shadow, in a
	// transparent window, so the card can move about in it.
	reminder := panelOptions("Pray Time reminder", reminderWidth, reminderHeight, a.reminderView)
	reminder.Transparent, reminder.DisableShadow, reminder.CornerRadius = true, true, 0
	a.reminderWin = mygo.NewWindow(reminder)
	// Settings is an ordinary window (in the taskbar, not kept above, not
	// hidden on blur), so its drop-downs and other apps can take the focus.
	settings := panelOptions("Pray Time Settings", settingsWidth, settingsHeight, a.settingsView)
	settings.AlwaysOnTop, settings.SkipTaskbar = false, false
	a.settingsWin = mygo.NewWindow(settings)

	for _, w := range []*mygo.Window{a.popup, a.reminderWin, a.settingsWin} {
		w := w
		w.OnClose(func(e *mygo.CloseEvent) {
			// Alt+F4 and Escape hide the panels; only Quit closes them.
			if a.quitting {
				return
			}
			e.PreventDefault()
			if w == a.popup && a.noTray {
				mygo.App.Quit()
				return
			}
			w.Hide()
			a.syncMenu()
		})
	}
	a.popup.OnBlur(func() {
		// Opening settings from the popup moves the focus there: keep the popup.
		if a.noTray || time.Now().Before(a.suppressBlur) {
			return
		}
		a.popup.Hide()
		a.popupHidAt = time.Now()
		a.syncMenu()
	})

	a.setupTray()
	a.applyIcons()
	mygo.Theme.OnUpdated(func() {
		a.applyIcons()
		a.invalidateAll()
	})
	mygo.Screen.OnDisplaysChanged(func() {
		// A monitor came or went: put visible panels back on a screen.
		if a.popup.IsVisible() {
			a.place(a.popup, a.state.MainAnchor(), popupWidth, popupHeight)
		}
		if a.reminderWin.IsVisible() {
			a.place(a.reminderWin, a.state.ReminderAnchor(), reminderWidth, reminderHeight)
		}
	})

	a.autostart = mygo.App.OpenAtLogin()
	if a.noTray && !mygo.App.WasOpenedAtLogin() {
		a.showPopup()
	}
	a.tick()
	go func() {
		// Tick on second boundaries, so the countdown changes with the clock.
		time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
		t := time.NewTicker(time.Second)
		for range t.C {
			mygo.RunOnMain(a.tick)
		}
	}()
}

func (a *app) setupTray() {
	a.showItem = &mygo.MenuItem{Label: "Show prayer times", Click: func(*mygo.MenuItem, *mygo.Window) { a.togglePopup() }}
	a.trayMenu = mygo.NewMenu([]*mygo.MenuItem{
		a.showItem,
		{Label: "Settings", Click: func(*mygo.MenuItem, *mygo.Window) { a.showSettings() }},
		mygo.Separator(),
		{Label: "Quit", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "pray-time", Icon: trayIconLight, ToolTip: "Pray Time"})
	if err != nil {
		// Without a tray (no AppIndicator on Linux), the popup is the app:
		// it shows at launch and closing it quits.
		log.Println("tray:", err)
		a.noTray = true
		return
	}
	a.tray = tray
	if runtime.GOOS == "linux" {
		// AppIndicator shows a menu on click; its first item toggles the popup.
		tray.SetMenu(a.trayMenu)
		tray.OnClick(a.togglePopup)
	} else {
		// A primary click toggles the popup; the menu opens on right-click.
		tray.OnClick(a.togglePopup)
		tray.OnRightClick(func() {
			tray.SetMenu(a.trayMenu)
			defer tray.SetMenu(nil)
			tray.PopUpMenu()
		})
	}
	tray.OnAppearanceChange(a.applyIcons)
}

// applyIcons picks the light or dark artwork, as the Kotlin app did.
func (a *app) applyIcons() {
	icon := appIconLight
	if mygo.Theme.IsDark() {
		icon = appIconDark
	}
	for _, w := range []*mygo.Window{a.popup, a.reminderWin, a.settingsWin} {
		if w != nil {
			_ = w.SetIcon(icon)
		}
	}
	if a.tray != nil {
		tray := trayIconLight
		if a.tray.Dark() {
			tray = trayIconDark
		}
		_ = a.tray.SetIcon(tray, false)
	}
}

func (a *app) syncMenu() {
	if a.showItem == nil || a.popup == nil {
		return
	}
	label := "Show prayer times"
	if a.popup.IsVisible() {
		label = "Hide prayer times"
	}
	if a.showItem.Label != label {
		a.showItem.SetLabel(label)
	}
}

func (a *app) invalidateAll() {
	for _, w := range []*mygo.Window{a.popup, a.reminderWin, a.settingsWin} {
		if w != nil && w.IsVisible() {
			w.Invalidate()
		}
	}
}

// anchorPosition puts a w×h window at anchor inside area, margin DIPs in.
func anchorPosition(anchor store.Anchor, area mygo.Rectangle, w, h, margin int) (int, int) {
	x := area.X + area.Width - w - margin
	switch anchor {
	case store.TopLeft, store.MiddleLeft, store.BottomLeft:
		x = area.X + margin
	}
	y := area.Y + margin
	switch anchor {
	case store.MiddleLeft, store.MiddleRight:
		y = area.Y + (area.Height-h)/2
	case store.BottomLeft, store.BottomRight:
		y = area.Y + area.Height - h - margin
	}
	return x, y
}

// place moves a panel to its anchor on the primary display's work area.
func (a *app) place(w *mygo.Window, anchor store.Anchor, width, height int) {
	area := mygo.Screen.PrimaryDisplay().WorkArea
	if area.Width == 0 || area.Height == 0 {
		w.Center()
		return
	}
	x, y := anchorPosition(anchor, area, width, height, screenMargin)
	w.SetPosition(x, y)
}

// replace puts a panel at its anchor again once it shows: GTK applies the
// position asynchronously and the window manager may move a window as it
// maps, which left the popup cropped or off its corner. Later passes catch
// a window manager that places it after the first.
func (a *app) replace(w *mygo.Window, anchor store.Anchor, width, height int) {
	for _, d := range []time.Duration{0, 60 * time.Millisecond, 200 * time.Millisecond} {
		d := d
		time.AfterFunc(d, func() {
			mygo.RunOnMain(func() {
				if w.IsVisible() {
					a.place(w, anchor, width, height)
				}
			})
		})
	}
}

func (a *app) showPopup() {
	a.place(a.popup, a.state.MainAnchor(), popupWidth, popupHeight)
	a.suppressBlur = time.Now().Add(400 * time.Millisecond)
	a.popup.Show()
	a.popup.Focus()
	a.replace(a.popup, a.state.MainAnchor(), popupWidth, popupHeight)
	// The reminder steps aside while the popup shows, as before.
	if a.anim != outcomeNone && !a.testReminder {
		a.finishAnimation()
	}
	if a.reminderWin.IsVisible() && !a.testReminder {
		a.reminderWin.Hide()
	}
	a.syncMenu()
}

func (a *app) togglePopup() {
	switch {
	case a.popup.IsVisible():
		a.popup.Hide()
	case time.Since(a.popupHidAt) < 400*time.Millisecond:
		// The click on the tray icon blurred the popup: it meant to close it.
	default:
		a.showPopup()
	}
	a.syncMenu()
}

func (a *app) showSettings() {
	a.suppressBlur = time.Now().Add(400 * time.Millisecond)
	a.recents = a.state.Recent()
	a.loadSettings()
	a.autostart = mygo.App.OpenAtLogin()
	a.settingsWin.Center()
	a.settingsWin.Show()
	a.settingsWin.Focus()
}

func (a *app) markPrayed(t prayer.Time, prayed bool) {
	if a.sched.MarkPrayed(t, prayed) {
		a.state.SaveCompleted(a.sched.Today().Date, a.sched.Completed())
	}
	a.tick()
}

func (a *app) selectRegion(r prayer.Region) {
	a.state.SaveRecent(r)
	a.sched.SetRegion(r)
	a.state.SaveRegion(r)
	a.state.SaveCompleted(a.sched.Today().Date, a.sched.Completed())
	a.recents = a.state.Recent()
	a.tick()
}

// dismissReminder plays the snooze sequence; at its end the reminder
// snoozes, or hides until the next prayer is due.
func (a *app) dismissReminder() {
	a.startAnimation(outcomeSnoozed)
}

// prayedFromReminder plays the mark-as-prayed sequence; at its end the
// prayer is marked.
func (a *app) prayedFromReminder() {
	a.startAnimation(outcomePrayed)
}

func (a *app) startAnimation(o outcome) {
	if a.anim != outcomeNone || a.reminderPrayer().Name == "" {
		return
	}
	a.anim, a.animStart = o, time.Time{}
	if a.reminderWin != nil {
		a.reminderWin.Invalidate()
	}
}

// finishAnimation does what the card's sequence was for, and hides it.
func (a *app) finishAnimation() {
	o, p, test := a.anim, a.reminderPrayer(), a.testReminder
	a.anim, a.animStart, a.testReminder = outcomeNone, time.Time{}, false
	if a.reminderWin != nil {
		a.reminderWin.Hide()
	}
	if test {
		return
	}
	switch o {
	case outcomePrayed:
		a.markPrayed(p, true)
	case outcomeSnoozed:
		a.rem.Dismiss(a.state.SnoozeEnabled(), a.state.SnoozeMinutes())
		a.tick()
	}
}

func (a *app) snoozedLabel() string {
	if a.state.SnoozeEnabled() {
		return "Okay… back in " + strconv.Itoa(a.state.SnoozeMinutes()) + " min"
	}
	return "Okay… until the next prayer"
}

func (a *app) reminderPrayer() prayer.Time {
	if a.testReminder {
		return a.testPrayer
	}
	return a.rem.Current
}

func (a *app) showTestReminder() {
	day := a.sched.Today()
	now := a.sched.Now()
	p, ok := prayer.LatestDue(day, nil, now)
	if !ok {
		p, ok = day.Next(now)
	}
	if !ok && len(day.Times) > 0 {
		p = day.Times[len(day.Times)-1]
	}
	a.testPrayer, a.testReminder = p, true
	a.anim, a.animStart = outcomeNone, time.Time{}
	a.showReminder()
}

func (a *app) showReminder() {
	a.place(a.reminderWin, a.state.ReminderAnchor(), reminderWidth, reminderHeight)
	// Shown without taking the keyboard from what the user is doing.
	a.reminderWin.ShowInactive()
	a.replace(a.reminderWin, a.state.ReminderAnchor(), reminderWidth, reminderHeight)
}

// tick runs every second on the main thread: it rolls the day over,
// refreshes the countdown, and shows or hides the due reminder.
func (a *app) tick() {
	a.sched.Tick()
	now := a.sched.Now()
	due, ok := prayer.LatestDue(a.sched.Today(), a.sched.CompletedSet(), now)
	a.rem.OnTick(due, ok, a.state.SnoozeEnabled())

	if a.reminderWin != nil {
		// While the card plays a sequence, its view finishes it.
		if a.anim == outcomeNone && !a.testReminder {
			show := a.rem.Visible && a.rem.HasCurrent && !a.popup.IsVisible()
			if show && !a.reminderWin.IsVisible() {
				a.showReminder()
			} else if !show && a.reminderWin.IsVisible() {
				a.reminderWin.Hide()
			}
		}
	}
	a.invalidateAll()
}
