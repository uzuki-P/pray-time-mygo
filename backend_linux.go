package main

import (
	"os"
	"syscall"
)

// preferX11OnWayland runs the app through XWayland on Wayland sessions.
// Wayland compositors ignore the positions apps give their windows and
// their keep-above hint (MyGo uses the layer shell for that only where
// libgtk-layer-shell is installed), so the popup and the reminder could not
// open at their chosen corner or stay above other windows. Under XWayland
// both work, as they did for the Kotlin app, which AWT ran there.
// PRAYTIME_WAYLAND=1 keeps native Wayland; a GDK_BACKEND set by the user is
// left alone.
//
// GTK reads the variable from libc's environment, which os.Setenv does not
// change in a build without cgo, so the process starts itself again with it.
func preferX11OnWayland() {
	if os.Getenv("PRAYTIME_WAYLAND") == "1" || os.Getenv("GDK_BACKEND") != "" {
		return
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" || os.Getenv("DISPLAY") == "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	env := append(os.Environ(), "GDK_BACKEND=x11")
	// On success Exec does not return; on failure the app runs on Wayland.
	_ = syscall.Exec(exe, os.Args, env)
}
