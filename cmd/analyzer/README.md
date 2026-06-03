# Demo Analyzer App

A self-contained web application built **on top of** demoinfocs-golang that extracts and visualizes **everything** the library can provide from a demo file.

- In-game chat (and console messages via events)
- Kills, headshots, wallbangs, assists, round outcomes
- Full player statistics (K/D/A + ADR from hurt events)
- Weapon fire positions → interactive heatmaps (T / CT / all)
- Grenade throws + full trajectories (lineups) with colored paths on radar
- Player Viewmodel Settings (offset X/Y/Z, FOV, crosshair code via `Player.ViewmodelOffset()`, `ViewmodelFOV()`, `CrosshairCode()` — CS2 only)
- Voice communications (raw packets from `CSVCMsg_VoiceData` / `CCLCMsg_VoiceData`)
- Rounds, scores, timings
- Map radar overlays (loaded from the library's own `examples/_assets`)
- Export everything as JSON or images

**Zero modification** was made to any existing file in the repository. Everything lives under `cmd/analyzer/`.

## How to use

1. Place one or more `.dem` files into the `demos/` folder at the repository root.

2. From the repo root run:

   ```powershell
   go run ./cmd/analyzer
   ```

   Or with flags:

   ```powershell
   go run ./cmd/analyzer -demos ./demos -addr :8081
   ```

3. Open http://localhost:8080 in your browser.

4. Pick a demo from the dropdown → click **ANALYZE**.

The parser runs inside the Go process using `demoinfocs.NewParser` + full suite of `RegisterEventHandler` + `RegisterNetMessageHandler`.

## What the library powers here

- `events.Kill`, `events.WeaponFire`, `events.PlayerHurt`, `events.ChatMessage`, `events.Round*`, `events.GrenadeProjectile*`
- `common.Player`, `common.Team`, `common.EquipmentType`
- `GameState.Participants()`, `IngameTick()`, team scores
- `msg.CSVCMsg_ServerInfo` (map name)
- `msg.CSVCMsg_VoiceData` + `CCLCMsg_VoiceData` (voice)
- Trajectory data via `GrenadeProjectile.Trajectory`
- Map metadata + radar images via the shared examples helpers (no duplication of assets)

## Voice notes

Voice is delivered as raw encoded packets (usually Opus for CS2). The UI groups them into "clips" separated by silence gaps.

- Click any **Clip** button to download the concatenated bytes for that utterance.
- To actually listen you will typically need a tool such as the referenced CS2VoiceData project or custom ffmpeg / opus tooling + the correct header.

## Architecture (new files only)

- `cmd/analyzer/main.go` — HTTP server + all parsing logic + data aggregation (no other files touched)
- `cmd/analyzer/index.html` — completely self-contained frontend (Tailwind via CDN + vanilla JS + canvas)
- `demos/` — user drops demos here (with small README)

Everything is one binary when built (`go build -o demo-analyzer ./cmd/analyzer`).

## Extending

All data is in the `Analysis` struct returned by `/api/analyze`. Add new event handlers in `parseDemo()`, new fields, and expose them in the JSON + a new tab/panel. The frontend is deliberately simple to hack on.

Enjoy exploring your demos!
