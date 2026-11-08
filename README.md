# APOD Wallpaper

A small Windows tray app that sets NASA's [Astronomy Picture of the Day](https://science.nasa.gov/apod/) as your desktop wallpaper, every day, on every monitor.

- **Largest monitor** gets today's picture; **every other monitor** gets yesterday's.
- **Full resolution:** downloads NASA's original image, not the resized preview.
- **Never cropped or stretched:** the picture is centered at full height, and the bars beside it are filled with colors taken from the picture's own edges, so they blend in.
- **Video days** are skipped. If today's APOD is a video, the most recent still picture is used instead.
- Starts at login and checks for a new picture every 3 hours (every 5 minutes after a failure).

## Tray menu

Right-click the tray icon:

| Item | What it does |
|------|--------------|
| **Refresh now** | Check NASA and re-apply the wallpapers immediately |
| **Open today's APOD page** | Opens science.nasa.gov/apod in your browser |
| **Bar style → Soft blend** | Bars fade gently; sharp features like a horizon blur into a smooth gradient (default) |
| **Bar style → Crisp follow** | Bars track the picture's edge closely, keeping features like the horizon sharper |
| **Quit** | Exit the app |

Hovering over the icon shows today's picture title.

## Requirements

- Windows 10 or 11
- [Go](https://go.dev/) 1.27+ to build (e.g. `mise use go@latest`)

No NASA API key is needed.

## Build and run

```sh
go build -ldflags "-H windowsgui -s -w" -o apod-wallpaper.exe .
./apod-wallpaper.exe
```

`-H windowsgui` keeps a console window from opening. On first run the app registers itself to start at login (per-user, no admin needed). If you move the `.exe`, run it once from the new location and the startup entry updates itself.

Only one copy runs at a time; launching it again does nothing.

## Files

Everything lives in `%LOCALAPPDATA%\APOD\`:

| Path | Contents |
|------|----------|
| `YYYY-MM-DD.jpg` / `.png` | Downloaded originals (the newest 7 are kept) |
| `rendered\` | Wallpapers rendered at each monitor's exact resolution |
| `settings.json` | Your bar style choice |
| `apod.log` | Activity and errors |

## How it works

1. **Fetch:** reads the official APOD RSS feed at `science.nasa.gov/feed/apod-basic/`. The older `api.nasa.gov` APOD API broke when APOD moved to science.nasa.gov in 2026 (it returns the NASA logo for every day), so it isn't used.
2. **Full resolution:** the feed's image links point to a preview capped at about 1280 px. The app rewrites `/dynamicimage/assets/…?w=…` to `/content/dam/…` to get the original.
3. **Monitors:** uses the Windows `IDesktopWallpaper` API to set a different wallpaper per monitor. "Largest" means the most pixels, so a short, wide strip display isn't mistaken for the main screen.
4. **Render:** each picture is scaled to fit and centered. For every row along the left and right edges, the app takes the median color of a thin strip (so stars don't count), smooths those colors along the edge, and paints the bars row by row. Panoramas wider than the screen get top and bottom bars instead.

## Troubleshooting

**All monitors show the same picture, or not an APOD at all.** Another wallpaper app is probably overriding this one. Microsoft Bing Wallpaper does this, and so can Wallpaper Engine and Lively. Quit it and disable its startup entry. The app logs a `WARNING` in `apod.log` when it sees one of these running.

**Nothing changes.** Check `apod.log`. Network failures are retried after 5 minutes.

## Uninstall

1. Right-click the tray icon → **Quit**.
2. Remove the `APOD Wallpaper` entry in **Settings → Apps → Startup** (or delete it from `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`).
3. Delete `%LOCALAPPDATA%\APOD\` and the `.exe`.

## Development

```sh
go test ./...
go vet ./...
```

The rendering, feed parsing, monitor assignment and cleanup logic are plain Go with unit tests. Windows-specific code (COM wallpaper calls, tray, registry) is in the `*_windows.go` files and `main.go`.

## Credits

Pictures and descriptions come from NASA's [Astronomy Picture of the Day](https://science.nasa.gov/apod/), by Robert Nemiroff and Jerry Bonnell. Many APOD images are copyrighted by their photographers, so check each image's credit before reusing it anywhere else.
