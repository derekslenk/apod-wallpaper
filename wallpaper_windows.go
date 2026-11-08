//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IDesktopWallpaper COM interface (Windows 8+), which supports a separate
// wallpaper per monitor.
var (
	clsidDesktopWallpaper = windows.GUID{Data1: 0xC2CF3110, Data2: 0x460E, Data3: 0x4FC1, Data4: [8]byte{0xB9, 0xD0, 0x8A, 0x1C, 0x0C, 0x9C, 0xC4, 0xBD}}
	iidIDesktopWallpaper  = windows.GUID{Data1: 0xB92B56A9, Data2: 0x8B55, Data3: 0x4E14, Data4: [8]byte{0x9A, 0x89, 0x01, 0x99, 0xBB, 0xB6, 0xF9, 0x3B}}

	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

const (
	clsctxAll  = 0x17
	dwposFill  = 4
	vRelease   = 2
	vSetWall   = 3
	vPathAt    = 5
	vPathCount = 6
	vGetRect   = 7
	vSetPos    = 10
)

type desktopWallpaper struct{ vtbl *[16]uintptr }

func (d *desktopWallpaper) call(idx int, args ...uintptr) error {
	r, _, _ := syscall.SyscallN(d.vtbl[idx], append([]uintptr{uintptr(unsafe.Pointer(d))}, args...)...)
	if int32(r) < 0 {
		return fmt.Errorf("IDesktopWallpaper method %d: HRESULT 0x%08X", idx, uint32(r))
	}
	return nil
}

type rect struct{ Left, Top, Right, Bottom int32 }

// setWallpapers lists active monitors, asks prepare for a wallpaper file per
// monitor ID, and applies them.
func setWallpapers(prepare func([]Monitor) (map[string]string, error)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		return fmt.Errorf("CoInitializeEx: %w", err)
	}
	defer windows.CoUninitialize()

	var dw *desktopWallpaper
	r, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidDesktopWallpaper)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIDesktopWallpaper)), uintptr(unsafe.Pointer(&dw)))
	if int32(r) < 0 || dw == nil {
		return fmt.Errorf("CoCreateInstance(DesktopWallpaper): HRESULT 0x%08X", uint32(r))
	}
	defer dw.call(vRelease)

	var count uint32
	if err := dw.call(vPathCount, uintptr(unsafe.Pointer(&count))); err != nil {
		return err
	}
	var monitors []Monitor
	for i := uint32(0); i < count; i++ {
		var p *uint16
		if err := dw.call(vPathAt, uintptr(i), uintptr(unsafe.Pointer(&p))); err != nil || p == nil {
			continue
		}
		id := windows.UTF16PtrToString(p)
		windows.CoTaskMemFree(unsafe.Pointer(p))
		var rc rect
		if err := dw.call(vGetRect, uintptr(unsafe.Pointer(mustUTF16(id))), uintptr(unsafe.Pointer(&rc))); err != nil {
			continue // monitor listed but not active
		}
		monitors = append(monitors, Monitor{ID: id, Width: int(rc.Right - rc.Left), Height: int(rc.Bottom - rc.Top)})
	}
	if len(monitors) == 0 {
		return fmt.Errorf("no active monitors found")
	}

	files, err := prepare(monitors)
	if err != nil {
		return err
	}
	// Files are pre-rendered at each monitor's exact resolution, so Fill is a
	// 1:1 mapping (and still behaves sensibly if a resolution changes later).
	if err := dw.call(vSetPos, dwposFill); err != nil {
		return err
	}
	for id, path := range files {
		if err := dw.call(vSetWall, uintptr(unsafe.Pointer(mustUTF16(id))), uintptr(unsafe.Pointer(mustUTF16(path)))); err != nil {
			return fmt.Errorf("set wallpaper on %s: %w", id, err)
		}
	}
	return nil
}

func mustUTF16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}
