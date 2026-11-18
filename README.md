# Pray Time (MyGo)

Pray Time is a small tray app for Windows and Linux that shows today's prayer
times for a chosen region, counts down to the next prayer, and reminds you when
a prayer is due. This is a port of the Kotlin Compose Desktop app
([uzuki-P/pray-time](https://github.com/uzuki-P/pray-time)) to
[MyGo](https://mygo.run/) with its native UI: one Go binary that draws its own
interface, with no JVM, webview or JavaScript.

## Features

- **Prayer times:** Adhan's Singapore method with the Shafi madhab. `internal/adhan` is a
  Go port of Adhan Java 1.2.1, checked against 64 dates and places computed by
  the Java library (`internal/adhan/testdata`). The default region is Medan Johor.
- **Countdown:** a live countdown to the next prayer. Sunrise is shown but never
  targeted; after Isha the target is tomorrow's Fajr.
- **Completed prayers:** mark prayers that have arrived as prayed from the popup or the
  reminder. Completion is per day.
- **Due reminder:** appears when a prayer becomes due, without taking the
  keyboard. Snooze it for 1–120 minutes, or dismiss it until the next prayer.
- **Tray:** a popup with the schedule rail and checklist, plus a menu with
  Show/Hide prayer times, Settings and Quit. Closing a window keeps the app in
  the tray.
- **Settings:** region search, snooze, reminder and popup position (six
  anchors), theme (System, Light, Dark), start on login, and a test reminder.
- **Region search:** recent regions, the built-in Medan list, and the worldwide
  GeoNames list (~169k places in SQLite through MyGo's cgo-free SQLite plugin).
  The list ships gzipped in the binary, is extracted to the state folder on the
  first search, and is never held in memory.
- **State:** `$XDG_CONFIG_HOME/praytime/state.properties` (or
  `~/.config/praytime/`), with the same file, keys and formats as the Kotlin app,
  so the two read each other's region, completions, recents and settings.
- **Single instance:** starting the app again opens the running one's popup.
- **Look:** the same palette, sky colours, and fonts (Reem Kufi and IBM Plex Sans, bundled
  under the SIL OFL; licenses in `assets/licenses`) and icons as the Kotlin app.

## Differences from the Kotlin app

| Kotlin app | This port | Why |
|---|---|---|
| Linux tray: hand-rolled StatusNotifierItem with `ItemIsMenu=false`, so a left click toggles the popup | Linux tray uses AppIndicator: a click opens the menu, whose first item shows or hides the popup | MyGo's Linux tray is AppIndicator (`libayatana-appindicator3`), which always shows its menu on click and reports activation only on some desktops |
| Custom-drawn right-click menu window | The system tray menu (Windows: right-click; Linux: any click) | MyGo's native tray menu replaces the hand-drawn one |
| Reminder animations (confetti flood, check drawing, shake, droop and sink) | A short "marked as prayed" or "Okay… back in N min" confirmation, then the reminder hides | Simplified for the port; MyGo's animation APIs can bring them back later |
| Settings: an undecorated window kept above others, closed when it loses focus | Settings: a frameless window in the taskbar that stays open until closed (✕ or Escape) | Its drop-downs open their own popup surface, which would take the focus and close it |
| Transparent, rounded windows with drop shadows | Frameless windows with rounded corners on Windows 11; square on Linux | MyGo's transparent windows are for macOS and Windows 11 |
| Wayland: AWT runs on XWayland | On a Wayland session the app restarts itself with `GDK_BACKEND=x11` (XWayland) | Wayland ignores window positions and keep-above; the corner anchors and the always-on-top reminder need X11, or `libgtk-layer-shell`. `PRAYTIME_WAYLAND=1` keeps native Wayland |
| Display-change recovery for Compose's rendering | Panels are placed again whenever the displays change | MyGo has no stalled-renderer problem to recover from |
| Start on login: XDG autostart file / HKCU Run key, written by the app | `App.SetOpenAtLogin` | MyGo manages its own XDG autostart entry (Linux) and HKCU Run entry (Windows); turn the Kotlin app's off if both are installed |
| AppImage in `~/apps` | `install.sh` installs into `~/.local` (app, `pray-time` command, menu entry); `.deb`, `.rpm` and a PKGBUILD are built too | MyGo packages Linux apps as a tarball with install.sh and as native packages, not AppImage |

## Requirements

- Go 1.27 or newer (mise: `mise use -g go@latest`).
- Linux at run time: GTK 3 and `libayatana-appindicator3` for the tray.
  Without the tray library, the popup opens at launch and closing it quits.
- Windows 10 or 11: nothing extra (Direct3D 11).
- Windows installer builds: NSIS (`makensis`), which CI installs.

## Commands

```bash
just dev          # run with rebuild and restart on save
just test         # unit tests and headless view tests
just snapshots    # render every view to build/snapshots/*.png
just build        # build Linux into build/linux-amd64/
just build-linux  # build and stage the Linux tarball in _apk/
just build-exe    # build and stage the Windows installer in _apk/ (needs makensis)
just update       # build, stage, and install for this user (~/.local)
just ci-exe       # wait for CI's build of the pushed HEAD, fetch and share it
just release      # tag v<version>: CI publishes a GitHub release
```

The version is `<major.minor from mygo.json>.<commit count>` (`scripts/version`).

## CI

`.github/workflows/windows-exe.yml` runs on `ubuntu-latest`, because MyGo
cross-compiles without cgo. It tests, then builds the Windows installer and the
Linux tarball:

- **Pushes to main:** upload the builds as artifacts. `ci-fetch.timer` downloads
  them into `_apk/` and shares them through Gokapi.
- **`v*` tags:** publish a GitHub release with the installer, tarball,
  `install.sh`, `.deb` and `.rpm`.

## Layout

```text
main.go            app lifecycle, windows, tray, the one-second tick
view_*.go          popup, reminder and settings views (package ui)
theme.go           palette, fonts and icons
internal/adhan     prayer-time astronomy (Adhan Java port)
internal/prayer    regions, day schedule, countdown, completions, reminder rules
internal/store     state.properties
internal/cities    GeoNames city search (SQLite)
```
