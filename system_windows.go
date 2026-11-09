//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
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

// makeIcon draws a small night-sky disc with a few stars, wrapped as a .ico.
func makeIcon() []byte {
	const n = 32
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := float64(x)-15.5, float64(y)-15.5
			d := math.Hypot(dx, dy)
			if d <= 15 {
				t := d / 15
				img.Set(x, y, color.NRGBA{uint8(20 + 40*t), uint8(25 + 30*t), uint8(90 + 60*(1-t)), 255})
			}
		}
	}
	star := func(cx, cy int, big bool) {
		img.Set(cx, cy, color.White)
		if big {
			for _, o := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				img.Set(cx+o[0], cy+o[1], color.NRGBA{255, 240, 200, 255})
			}
		}
	}
	star(10, 10, true)
	star(21, 14, true)
	star(14, 22, false)
	star(23, 23, false)
	star(8, 18, false)

	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, img)

	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, []uint16{0, 1, 1})                  // ICONDIR
	ico.Write([]byte{n, n, 0, 0})                                               // w, h, colors, reserved
	binary.Write(&ico, binary.LittleEndian, []uint16{1, 32})                    // planes, bpp
	binary.Write(&ico, binary.LittleEndian, []uint32{uint32(pngBuf.Len()), 22}) // size, offset
	ico.Write(pngBuf.Bytes())
	return ico.Bytes()
}
