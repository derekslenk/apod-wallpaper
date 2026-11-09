//go:build windows

package main

import (
	"log"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// ensureAutostart registers the current exe to run at login (per-user, no admin).
func ensureAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("Nightglass", `"`+exe+`"`); err != nil {
		return err
	}
	k.DeleteValue("APOD Wallpaper") // entry from before the rename; would start the old exe too
	return nil
}

// singleInstance returns false if another copy is already running.
func singleInstance() bool {
	// Name kept from before the rename to Nightglass, so an old copy and a new
	// one never run at the same time.
	_, err := windows.CreateMutex(nil, false, mustUTF16(`Local\APODWallpaperSingleInstance`))
	return err != windows.ERROR_ALREADY_EXISTS
}

// processRunning reports whether a process with this exe name (case-insensitive) exists.
func processRunning(exe string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), exe) {
			return true
		}
	}
	return false
}

// Apps known to overwrite the wallpaper on all monitors.
var competingWallpaperApps = []string{"BingWallpaper.exe", "wallpaper32.exe", "wallpaper64.exe", "Lively.exe"}

func warnCompetingApps() {
	for _, exe := range competingWallpaperApps {
		if processRunning(exe) {
			log.Printf("WARNING: %s is running and will likely overwrite the Nightglass wallpaper; quit it and disable its startup", exe)
		}
	}
}

func openURL(u string) {
	windows.ShellExecute(0, mustUTF16("open"), mustUTF16(u), nil, nil, windows.SW_SHOWNORMAL)
}
