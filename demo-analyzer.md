# Demo Analyzer (new app)

This is a brand new application created **without touching any existing source files**.

Location:
- `demos/` — drop your `.dem` files here
- `cmd/analyzer/` — the full app (Go server + embedded HTML/JS frontend)

## Run

```powershell
# from repo root
go run ./cmd/analyzer
```

Then visit http://localhost:8080

See [cmd/analyzer/README.md](cmd/analyzer/README.md) for full details.

## Capabilities (all powered directly by the demoinfocs-golang library)

- Chat extraction (`events.ChatMessage`)
- Kills + stats aggregation (`events.Kill`, `events.PlayerHurt`)
- Round-by-round (`events.RoundStart` / `RoundEnd`)
- Heatmaps from `events.WeaponFire` + map coordinate translation (using library radar assets)
- Full grenade trajectories + lineups (`events.GrenadeProjectileThrow` / `Destroy` + `Projectile.Trajectory`)
- Player Viewmodel Settings + visual crosshair previews (via `common.Player.ViewmodelOffset()` / `ViewmodelFOV()` / `CrosshairCode()` / `Crosshair()` + `DecodeCrosshairShareCode` on CS2 demos). The web UI shows small in-game style canvas renders + modal detail view.
- Voice comms via net messages (`msg.CSVCMsg_VoiceData`)
- Everything else: game state, participants, equipment, infernos, bomb events etc. (easily extendable in `parseDemo`)

The frontend is a zero-dependency SPA (Tailwind CDN) with interactive canvas visualizers for heatmaps and nade trajectories, filters, live search, and one-click exports.

Pure Go + stdlib + the existing dependencies of this library only.
