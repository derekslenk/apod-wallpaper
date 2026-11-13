# Nightglass

A Windows tray app that turns NASA's Astronomy Picture of the Day into a per-monitor desktop wallpaper.

## Pictures

**Latest picture**:
The newest still-image APOD in the feed. On a video day, or before NASA posts the new day, it is an older day's picture.
_Avoid_: Today's picture, today's APOD

**Previous picture**:
The still-image APOD immediately before the Latest picture.
_Avoid_: Yesterday's picture

**Refresh**:
One pass that brings every monitor up to date with the feed: fetch, choose the Latest and Previous pictures, render, apply, and prune old downloads.
_Avoid_: Update (reserved for a newer Nightglass release), sync

## Monitors

**Main monitor**:
The display with the most pixels. It shows the Latest picture. Windows' "primary display" setting plays no part.
_Avoid_: Primary monitor, largest screen

**Side monitor**:
Any display other than the Main monitor. Side monitors show the Previous picture.
_Avoid_: Secondary monitor

## Rendering

**Bars**:
The filled area around a centered picture: left and right normally, top and bottom for a panorama wider than the screen.
_Avoid_: Letterbox, pillarbox, borders

**Bar style**:
How the Bars get their color from the picture's edges. Either **Soft blend** (smooth gradient, the default) or **Crisp follow** (tracks the edge closely).
