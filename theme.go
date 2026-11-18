package main

import (
	"embed"
	"log"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/fonts/*.ttf
var fontFiles embed.FS

//go:embed assets/icons/pray_time_tray_light.png
var trayIconLight []byte

//go:embed assets/icons/pray_time_tray_dark.png
var trayIconDark []byte

//go:embed assets/icons/pray_time_app_light.png
var appIconLight []byte

//go:embed assets/icons/pray_time_app_dark.png
var appIconDark []byte

// Reem Kufi for prayer names (Latin and Arabic), IBM Plex Sans for
// everything else, as in the Kotlin app.
const (
	bodyFont    = "Pray Time Plex"
	displayFont = "Pray Time Kufi"
)

func registerFonts() {
	for _, f := range []struct{ file, family string }{
		{"ibm_plex_sans_regular.ttf", bodyFont},
		{"ibm_plex_sans_medium.ttf", bodyFont},
		{"ibm_plex_sans_semibold.ttf", bodyFont},
		{"ibm_plex_sans_bold.ttf", bodyFont},
		{"reem_kufi_medium.ttf", displayFont},
		{"reem_kufi_bold.ttf", displayFont},
	} {
		data, err := fontFiles.ReadFile("assets/fonts/" + f.file)
		if err == nil {
			err = ui.RegisterFont(data, f.family)
		}
		if err != nil {
			log.Printf("font %s: %v", f.file, err)
		}
	}
}

// Palette is the app's own colors: night-sky ink with a brass lamp accent;
// coral marks a due prayer, teal a completed one.
type Palette struct {
	Dark                       bool
	Surface, Raised, OnSurface ui.Color
	Accent, OnAccent           ui.Color
	Due, Prayed, OnPrayed      ui.Color
}

func (p Palette) Muted() ui.Color   { return p.OnSurface.Alpha(0.62) }
func (p Palette) Faint() ui.Color   { return p.OnSurface.Alpha(0.38) }
func (p Palette) Divider() ui.Color { return p.OnSurface.Alpha(0.09) }

var darkPalette = Palette{
	Dark:      true,
	Surface:   ui.Hex("#111B2B"),
	Raised:    ui.Hex("#1B2739"),
	OnSurface: ui.Hex("#E8EDF4"),
	Accent:    ui.Hex("#E6B35A"),
	OnAccent:  ui.Hex("#1F1606"),
	Due:       ui.Hex("#F08A6C"),
	Prayed:    ui.Hex("#5CC4A6"),
	OnPrayed:  ui.Hex("#07261D"),
}

var lightPalette = Palette{
	Surface:   ui.Hex("#F6F8FB"),
	Raised:    ui.Hex("#E6EBF2"),
	OnSurface: ui.Hex("#142033"),
	Accent:    ui.Hex("#8C5A06"),
	OnAccent:  ui.Hex("#FFFFFF"),
	Due:       ui.Hex("#C2482B"),
	Prayed:    ui.Hex("#1E8A6E"),
	OnPrayed:  ui.Hex("#FFFFFF"),
}

// skyColors is the sky's colour at each time of the day schedule; the
// schedule rail blends between them.
var skyColors = map[string]ui.Color{
	"Fajr":    ui.Hex("#7B6FD0"),
	"Sunrise": ui.Hex("#F2A766"),
	"Dhuhr":   ui.Hex("#4EA3E6"),
	"Asr":     ui.Hex("#E7B04F"),
	"Maghrib": ui.Hex("#E0607A"),
	"Isha":    ui.Hex("#4656A8"),
}

// theme turns the palette into a ui.Theme, so the built-in widgets
// (switches, inputs, buttons, scroll bars) take the app's colors.
func theme(p Palette) *ui.Theme {
	t := ui.LightTheme()
	if p.Dark {
		t = ui.DarkTheme()
	}
	th := *t
	th.Background = p.Surface
	th.Surface = p.Raised
	th.SurfaceHover = p.Raised.Mix(p.OnSurface, 0.06)
	th.SurfacePressed = p.Raised.Mix(p.OnSurface, 0.12)
	th.Border = p.OnSurface.Alpha(0.14)
	th.Text = p.OnSurface
	th.TextMuted = p.Muted()
	th.Accent = p.Accent
	th.AccentHover = p.Accent.Mix(p.OnSurface, 0.1)
	th.AccentPressed = p.Accent.Mix(p.OnSurface, 0.2)
	th.AccentText = p.OnAccent
	th.InactiveAccent = p.Accent
	th.Focus = p.Accent.Alpha(0.6)
	th.Danger = p.Due
	th.Success = p.Prayed
	th.Font = bodyFont
	th.FontSize = 13
	th.Radius = 10
	return &th
}
