# Player Viewmodel Settings

This example shows how to use the library to extract player viewmodel settings from CS2 demos. Viewmodel settings include the viewmodel offset (X, Y, Z), field of view, and crosshair code/settings.

Use `player.CrosshairCode()` for the share code string, and `player.Crosshair()` (or `common.DecodeCrosshairShareCode(code)`) for the decoded struct with length/gap/color/etc. Useful for showing visual crosshair previews (see cmd/analyzer for canvas example).

## Running the example

`go run viewmodel_settings.go -demo /path/to/cs2-demo.dem`

### Sample output

```
Player viewmodels:
degster: Viewmodel Offset=(2.5, 0.0, -1.5), FOV=60.0
kyxsan: Viewmodel Offset=(1.0, 1.0, -1.0), FOV=60.0
NiKo: Viewmodel Offset=(-1.0, 1.5, -2.0), FOV=60.0
SOMEBODY: Viewmodel Offset=(2.5, 2.0, -2.0), FOV=60.0
Summer: Viewmodel Offset=(2.5, 0.0, -1.5), FOV=60.0
L1haNg: Viewmodel Offset=(2.5, 0.0, -1.5), FOV=60.0
ChildKing: Viewmodel Offset=(2.5, 1.0, -1.5), FOV=60.0
TeSeS: Viewmodel Offset=(2.5, 0.0, -1.5), FOV=60.0
Magisk: Viewmodel Offset=(2.5, 0.0, -1.5), FOV=60.0
kaze: Viewmodel Offset=(2.5, 0.0, -1.5), FOV=60.0
```

Note: Viewmodel + crosshair settings are only available in CS2 demos (player pawn properties). CS:GO demos will show zero/empty values. The analyzer cmd/ uses the new visual preview rendering for crosshairs.