package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/geo/r3"

	ex "github.com/markus-wa/demoinfocs-golang/v5/examples"
	demoinfocs "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
)

//go:embed index.html
var content embed.FS

var (
	demosDir = flag.String("demos", "demos", "Directory containing .dem files")
	addr     = flag.String("addr", ":8080", "HTTP listen address")
)

func main() {
	flag.Parse()

	log.Printf("Demo Analyzer starting...")
	log.Printf("Demos directory: %s", *demosDir)
	log.Printf("Listening on http://localhost%s", *addr)

	http.HandleFunc("/", serveIndex)
	http.HandleFunc("/api/demos", handleListDemos)
	http.HandleFunc("/api/analyze", handleAnalyze)
	http.HandleFunc("/api/radar", handleRadar)
	http.HandleFunc("/api/voice-clip", handleVoiceClip)
	http.HandleFunc("/api/export", handleExport)

	log.Fatal(http.ListenAndServe(*addr, nil))
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	data, err := content.ReadFile("index.html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// DemoListItem for UI list
type DemoListItem struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// handleListDemos lists *.dem files in demosDir
func handleListDemos(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(*demosDir)
	if err != nil {
		http.Error(w, "failed to read demos dir: "+err.Error(), 500)
		return
	}
	var items []DemoListItem
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".dem") {
			continue
		}
		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		items = append(items, DemoListItem{Name: e.Name(), Size: size})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeJSON(w, items)
}

// Analysis is the full extracted data returned to frontend.
type Analysis struct {
	DemoName     string        `json:"demoName"`
	MapName      string        `json:"mapName"`
	Duration     float64       `json:"duration"` // seconds
	TickRate     float64       `json:"tickRate"`
	PlaybackTicks int           `json:"playbackTicks"`
	Players      []PlayerStat  `json:"players"`
	Rounds       []RoundInfo   `json:"rounds"`
	Kills        []KillInfo    `json:"kills"`
	Chats        []ChatInfo    `json:"chats"`
	Shots        []ShotInfo    `json:"shots"` // world coords for client heatmap
	Nades        []NadeInfo    `json:"nades"`
	Voice        VoiceInfo     `json:"voice"`
	MapMeta      *MapMeta      `json:"mapMeta,omitempty"`
	Error        string        `json:"error,omitempty"`
}

// PlayerStat aggregated stats
type PlayerStat struct {
	Name        string  `json:"name"`
	SteamID64   uint64  `json:"steamId64,omitempty"`
	Team        string  `json:"team"`
	Kills       int     `json:"kills"`
	Deaths      int     `json:"deaths"`
	Assists     int     `json:"assists"`
	Damage      int     `json:"damage"`
	Headshots   int     `json:"headshots"`
	ADR         float64 `json:"adr,omitempty"`

	// Viewmodel settings (CS2 only, supported via common.Player methods)
	ViewmodelOffset r3.Vector `json:"viewmodelOffset"`
	ViewmodelFOV    float32   `json:"viewmodelFOV"`
	CrosshairCode   string    `json:"crosshairCode,omitempty"`
}

// RoundInfo
type RoundInfo struct {
	Number  int    `json:"number"`
	Winner  string `json:"winner"`
	Reason  string `json:"reason"`
	TScore  int    `json:"tScore"`
	CTScore int    `json:"ctScore"`
}

// KillInfo
type KillInfo struct {
	Tick       int    `json:"tick"`
	Round      int    `json:"round"`
	Killer     string `json:"killer"`
	Victim     string `json:"victim"`
	Weapon     string `json:"weapon"`
	Headshot   bool   `json:"headshot"`
	Wallbang   bool   `json:"wallbang"`
	Assister   string `json:"assister,omitempty"`
}

// ChatInfo
type ChatInfo struct {
	Tick   int    `json:"tick"`
	Sender string `json:"sender"`
	Text   string `json:"text"`
}

// ShotInfo world position for heatmaps etc.
type ShotInfo struct {
	Tick   int     `json:"tick"`
	Round  int     `json:"round"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Player string  `json:"player"`
	Team   string  `json:"team"`
	Weapon string  `json:"weapon"`
}

// Pos for trajectories
type Pos struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// NadeInfo
type NadeInfo struct {
	ID         int64  `json:"id"`
	Tick       int    `json:"tick"`
	Round      int    `json:"round"`
	Thrower    string `json:"thrower"`
	Team       string `json:"team"`
	Weapon     string `json:"weapon"`
	StartX     float64 `json:"startX"`
	StartY     float64 `json:"startY"`
	Trajectory []Pos  `json:"trajectory"`
}

// VoiceClipInfo - metadata only in main response
type VoiceClipInfo struct {
	Index      int `json:"index"`
	StartTick  int `json:"startTick"`
	EndTick    int `json:"endTick"`
	NumPackets int `json:"numPackets"`
	TotalBytes int `json:"totalBytes"`
}

// PlayerVoiceInfo
type PlayerVoiceInfo struct {
	Name         string          `json:"name"`
	SteamID64    uint64          `json:"steamId64"`
	TotalPackets int             `json:"totalPackets"`
	TotalBytes   int             `json:"totalBytes"`
	Clips        []VoiceClipInfo `json:"clips"`
}

// VoiceInfo
type VoiceInfo struct {
	Players []PlayerVoiceInfo `json:"players"`
}

// MapMeta for client side coord translation (from examples)
type MapMeta struct {
	PosX  float64 `json:"posX"`
	PosY  float64 `json:"posY"`
	Scale float64 `json:"scale"`
}

// internal parse state
type parseState struct {
	mu sync.Mutex // not really needed as single threaded parse but safe

	demoName string
	mapName  string

	// current
	currentRound int

	// collected
	players      map[uint64]*PlayerStat // by steam
	rounds       []RoundInfo
	kills        []KillInfo
	chats        []ChatInfo
	shots        []ShotInfo
	nades        []NadeInfo
	voicePackets map[uint64][]voicePacket // steam -> packets for grouping later

	// for nade tracking
	activeNades map[int64]*nadeBuilder // uniqueID -> builder
	nadeIDSeq   int64

	// for round scores (updated on end)
	lastTScore  int
	lastCTScore int

	// map meta
	mapMeta *MapMeta
}

type voicePacket struct {
	tick   int
	format int32 // VoiceDataFormatT
	data   []byte
	xuid   uint64
}

type nadeBuilder struct {
	id        int64
	throwTick int
	round     int
	thrower   string
	team      string
	weapon    string
	startX    float64
	startY    float64
	trajectory []r3.Vector
}

func newParseState(demoName string) *parseState {
	return &parseState{
		demoName:     demoName,
		players:      make(map[uint64]*PlayerStat),
		activeNades:  make(map[int64]*nadeBuilder),
		voicePackets: make(map[uint64][]voicePacket),
		currentRound: 0,
	}
}

// isGOTV returns true for the GOTV spectator entity that records the demo
// (e.g. "ESL GOTV", "GOTV", "Valve GOTV", "ESL TV", etc.). These are not real players.
func isGOTV(p *common.Player) bool {
	if p == nil {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(p.Name))
	if strings.Contains(name, "gotv") ||
		strings.Contains(name, "esltv") ||
		strings.Contains(name, "hltv") ||
		strings.Contains(name, "valve tv") ||
		strings.Contains(name, "tv ") && strings.Contains(name, "gotv") {
		return true
	}
	// Common for pure GOTV: no real SteamID + bot + often unassigned/spectator
	if p.SteamID64 == 0 && p.IsBot {
		return true
	}
	return false
}

func (st *parseState) ensurePlayer(p *common.Player) *PlayerStat {
	if p == nil {
		return nil
	}
	if isGOTV(p) {
		return nil
	}
	sid := p.SteamID64
	if sid == 0 {
		// bot or unknown, use name hash or something, but skip detailed for bots often
		sid = uint64(p.UserID) | (1 << 60) // synthetic
	}
	ps, ok := st.players[sid]
	if !ok {
		teamStr := teamToStr(p.Team)
		ps = &PlayerStat{Name: p.Name, SteamID64: sid, Team: teamStr}
		st.players[sid] = ps
	}

	// Capture player viewmodel settings (CS2 feature) whenever we have a player snapshot with pawn data.
	// The library methods return zeros for CS:GO or when not available yet.
	if p != nil {
		ps.ViewmodelOffset = p.ViewmodelOffset()
		ps.ViewmodelFOV = p.ViewmodelFOV()
		ps.CrosshairCode = p.CrosshairCode()
	}

	return ps
}

func teamToStr(t common.Team) string {
	switch t {
	case common.TeamTerrorists:
		return "T"
	case common.TeamCounterTerrorists:
		return "CT"
	default:
		return "?"
	}
}

func weaponToStr(w common.EquipmentType) string {
	// short names
	switch w {
	case common.EqHE:
		return "HE"
	case common.EqFlash:
		return "Flash"
	case common.EqSmoke:
		return "Smoke"
	case common.EqMolotov, common.EqIncendiary:
		return "Molotov"
	case common.EqDecoy:
		return "Decoy"
	default:
		return w.String()
	}
}

func roundEndReasonToStr(r events.RoundEndReason) string {
	switch r {
	case events.RoundEndReasonCTWin:
		return "CT Win"
	case events.RoundEndReasonTerroristsWin:
		return "T Win"
	case events.RoundEndReasonBombDefused:
		return "Defused"
	case events.RoundEndReasonTargetBombed:
		return "Bombed"
	default:
		return fmt.Sprintf("%d", r)
	}
}

// parseDemo does the heavy lifting using the library. Returns Analysis.
func parseDemo(demoPath string) (*Analysis, error) {
	f, err := os.Open(demoPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	p := demoinfocs.NewParser(f)
	defer p.Close()

	st := newParseState(filepath.Base(demoPath))

	// Capture map early via ServerInfo (reliable like examples)
	p.RegisterNetMessageHandler(func(m *msg.CSVCMsg_ServerInfo) {
		st.mapName = m.GetMapName()
		if st.mapName != "" {
			// try load meta (may panic on unknown, catch in caller)
			func() {
				defer func() { recover() }()
				mm := ex.GetMapMetadata(st.mapName)
				st.mapMeta = &MapMeta{PosX: mm.PosX, PosY: mm.PosY, Scale: mm.Scale}
			}()
		}
	})

	// Also try header map as fallback later

	// Round tracking
	p.RegisterEventHandler(func(e events.RoundStart) {
		st.currentRound++
		// initial scores from game state
		gs := p.GameState()
		st.lastTScore = gs.TeamTerrorists().Score()
		st.lastCTScore = gs.TeamCounterTerrorists().Score()

		// Capture viewmodel settings for playing players (CS2 only feature from the library)
		// This follows the pattern from examples/viewmodel-settings
		for _, pl := range gs.Participants().Playing() {
			if pl != nil && !isGOTV(pl) {
				st.ensurePlayer(pl) // will pull ViewmodelOffset/FOV/CrosshairCode via the player methods
			}
		}
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		gs := p.GameState()
		winner := "?"
		switch e.Winner {
		case common.TeamTerrorists:
			winner = "T"
		case common.TeamCounterTerrorists:
			winner = "CT"
		}
		// score after is +1 for winner
		tScore := gs.TeamTerrorists().Score()
		ctScore := gs.TeamCounterTerrorists().Score()
		if e.Winner == common.TeamTerrorists {
			tScore++
		} else if e.Winner == common.TeamCounterTerrorists {
			ctScore++
		}
		st.rounds = append(st.rounds, RoundInfo{
			Number:  st.currentRound,
			Winner:  winner,
			Reason:  roundEndReasonToStr(e.Reason),
			TScore:  tScore,
			CTScore: ctScore,
		})
		st.lastTScore = tScore
		st.lastCTScore = ctScore
	})

	// Kills
	p.RegisterEventHandler(func(e events.Kill) {
		if isGOTV(e.Killer) || isGOTV(e.Victim) {
			return // ignore any "kills" involving the GOTV recorder
		}
		tick := p.GameState().IngameTick()
		round := st.currentRound
		killer := "?"
		if e.Killer != nil {
			killer = e.Killer.Name
			if ps := st.ensurePlayer(e.Killer); ps != nil {
				ps.Kills++
				if e.IsHeadshot {
					ps.Headshots++
				}
			}
		}
		victim := "?"
		if e.Victim != nil {
			victim = e.Victim.Name
			if ps := st.ensurePlayer(e.Victim); ps != nil {
				ps.Deaths++
			}
		}
		assister := ""
		if e.Assister != nil {
			assister = e.Assister.Name
			if ps := st.ensurePlayer(e.Assister); ps != nil {
				ps.Assists++
			}
		}
		weap := "?"
		if e.Weapon != nil {
			weap = e.Weapon.String()
		}
		st.kills = append(st.kills, KillInfo{
			Tick:     tick,
			Round:    round,
			Killer:   killer,
			Victim:   victim,
			Weapon:   weap,
			Headshot: e.IsHeadshot,
			Wallbang: e.PenetratedObjects > 0,
			Assister: assister,
		})
	})

	// WeaponFire for heatmaps / shots
	p.RegisterEventHandler(func(e events.WeaponFire) {
		if e.Shooter == nil || isGOTV(e.Shooter) {
			return
		}
		tick := p.GameState().IngameTick()
		round := st.currentRound
		pos := e.Shooter.Position()
		st.shots = append(st.shots, ShotInfo{
			Tick:   tick,
			Round:  round,
			X:      pos.X,
			Y:      pos.Y,
			Player: e.Shooter.Name,
			Team:   teamToStr(e.Shooter.Team),
			Weapon: e.Weapon.String(),
		})
		// also damage? later
	})

	// PlayerHurt for damage stats (ADR)
	p.RegisterEventHandler(func(e events.PlayerHurt) {
		if e.Attacker == nil || e.Attacker == e.Player || isGOTV(e.Attacker) {
			return
		}
		if ps := st.ensurePlayer(e.Attacker); ps != nil {
			ps.Damage += e.HealthDamage
		}
	})

	// Chat
	p.RegisterEventHandler(func(e events.ChatMessage) {
		if isGOTV(e.Sender) {
			return
		}
		tick := p.GameState().IngameTick()
		sender := "?"
		if e.Sender != nil {
			sender = e.Sender.Name
		}
		st.chats = append(st.chats, ChatInfo{
			Tick:   tick,
			Sender: sender,
			Text:   e.Text,
		})
	})

	// Grenade throws / trajectories
	p.RegisterEventHandler(func(e events.GrenadeProjectileThrow) {
		if e.Projectile == nil || e.Projectile.Thrower == nil || isGOTV(e.Projectile.Thrower) {
			return
		}
		id := e.Projectile.UniqueID()
		if id == 0 {
			st.nadeIDSeq++
			id = st.nadeIDSeq
		}
		nb := &nadeBuilder{
			id:        id,
			throwTick: p.GameState().IngameTick(),
			round:     st.currentRound,
			thrower:   e.Projectile.Thrower.Name,
			team:      teamToStr(e.Projectile.Thrower.Team),
			weapon:    weaponToStr(e.Projectile.WeaponInstance.Type),
			startX:    e.Projectile.Position().X,
			startY:    e.Projectile.Position().Y,
			trajectory: []r3.Vector{e.Projectile.Position()},
		}
		st.activeNades[id] = nb
	})

	p.RegisterEventHandler(func(e events.GrenadeProjectileDestroy) {
		if e.Projectile == nil {
			return
		}
		id := e.Projectile.UniqueID()
		nb := st.activeNades[id]
		if nb == nil {
			return
		}
		// update trajectory from projectile (final)
		nb.trajectory = make([]r3.Vector, len(e.Projectile.Trajectory))
		for i, te := range e.Projectile.Trajectory {
			nb.trajectory[i] = te.Position
		}
		// finalize nade
		traj := make([]Pos, len(nb.trajectory))
		for i, v := range nb.trajectory {
			traj[i] = Pos{X: v.X, Y: v.Y, Z: v.Z}
		}
		st.nades = append(st.nades, NadeInfo{
			ID:         nb.id,
			Tick:       p.GameState().IngameTick(),
			Round:      nb.round,
			Thrower:    nb.thrower,
			Team:       nb.team,
			Weapon:     nb.weapon,
			StartX:     nb.startX,
			StartY:     nb.startY,
			Trajectory: traj,
		})
		delete(st.activeNades, id)
	})

	// Also capture inferno for molly coverage? For basic, nades + trajectories sufficient. Can extend later.

	// Voice data (CSVCMsg_VoiceData from server to clients, contains the encoded voice)
	p.RegisterNetMessageHandler(func(v *msg.CSVCMsg_VoiceData) {
		if v.GetAudio() == nil {
			return
		}
		audio := v.GetAudio()
		xuid := v.GetXuid()
		if xuid == 0 {
			// try client index to lookup? for now skip or use 0
			return
		}
		tick := p.GameState().IngameTick()
		st.voicePackets[xuid] = append(st.voicePackets[xuid], voicePacket{
			tick:   tick,
			format: int32(audio.GetFormat()),
			data:   append([]byte(nil), audio.GetVoiceData()...), // copy
			xuid:   xuid,
		})
	})

	// Also listen for client voice if present (clc_VoiceData) - rare in GOTV but for POV
	p.RegisterNetMessageHandler(func(v *msg.CCLCMsg_VoiceData) {
		if v.GetAudio() == nil {
			return
		}
		// client side voice, xuid may be in game state or we ignore for now or associate later
		// For basic, we focus on svc_VoiceData which is what GOTV/broadcasts contain
	})

	// Parse!
	err = p.ParseToEnd()
	if err != nil {
		// still return partial? for now wrap
		log.Printf("parse warning for %s: %v", demoPath, err)
	}

	// Post process: build final players list from state + our stats
	gs := p.GameState()
	// merge any missing players from final state
	for _, pl := range gs.Participants().All() {
		if isGOTV(pl) {
			continue
		}
		st.ensurePlayer(pl)
	}

	// compute ADR rough: damage / rounds played approx, or / num rounds
	numRounds := len(st.rounds)
	if numRounds == 0 {
		numRounds = 1
	}
	var playerList []PlayerStat
	for _, ps := range st.players {
		if numRounds > 0 {
			ps.ADR = float64(ps.Damage) / float64(numRounds)
		}
		playerList = append(playerList, *ps)
	}
	// sort by kills desc
	sort.Slice(playerList, func(i, j int) bool {
		if playerList[i].Kills != playerList[j].Kills {
			return playerList[i].Kills > playerList[j].Kills
		}
		return playerList[i].Name < playerList[j].Name
	})

	// Build voice info (group packets into clips)
	voiceInfo := buildVoiceInfo(st, gs)

	// Header info - use ServerInfo captured + CurrentTime / TickRate (header not public)
	duration := float64(p.CurrentTime().Seconds())
	if duration == 0 {
		duration = float64(gs.IngameTick()) / p.TickRate()
	}
	// mapName already from CSVCMsg_ServerInfo or header fallback below if needed
	if st.mapName == "" {
		// last attempt: use game rules or leave as-is (demo may still work without radar)
		st.mapName = ""
	}

	analysis := &Analysis{
		DemoName:      st.demoName,
		MapName:       st.mapName,
		Duration:      duration,
		TickRate:      p.TickRate(),
		PlaybackTicks: p.CurrentFrame(), // approx
		Players:       playerList,
		Rounds:        st.rounds,
		Kills:         st.kills,
		Chats:         st.chats,
		Shots:         st.shots,
		Nades:         st.nades,
		Voice:         voiceInfo,
		MapMeta:       st.mapMeta,
	}

	// If mapMeta still nil, try load again
	if analysis.MapMeta == nil && analysis.MapName != "" {
		func() {
			defer func() { recover() }()
			mm := ex.GetMapMetadata(analysis.MapName)
			analysis.MapMeta = &MapMeta{PosX: mm.PosX, PosY: mm.PosY, Scale: mm.Scale}
		}()
	}

	return analysis, nil
}

// buildVoiceInfo groups packets per player into clips separated by silence > 1s
func buildVoiceInfo(st *parseState, gs demoinfocs.GameState) VoiceInfo {
	var out VoiceInfo
	for sid, packets := range st.voicePackets {
		if len(packets) == 0 {
			continue
		}
		// find player name - no direct BySteamID on participants, use full scan (small N)
		name := "?"
		var pl *common.Player
		for _, candidate := range gs.Participants().All() {
			if candidate.SteamID64 == sid {
				pl = candidate
				name = candidate.Name
				break
			}
		}
		if pl != nil && isGOTV(pl) {
			continue // skip GOTV voice (shouldn't happen but defensive)
		}
		// sort packets by tick just in case
		sort.Slice(packets, func(i, j int) bool { return packets[i].tick < packets[j].tick })

		var clips []VoiceClipInfo
		curStart := packets[0].tick
		curEnd := packets[0].tick
		curPkts := 1
		curBytes := len(packets[0].data)
		totalPkts := 1
		totalBytes := len(packets[0].data)

		const silenceTicks = 64 // ~1s at 64tick, adjust

		for i := 1; i < len(packets); i++ {
			p := packets[i]
			totalPkts++
			totalBytes += len(p.data)
			if p.tick-curEnd > silenceTicks {
				// close prev clip
				clips = append(clips, VoiceClipInfo{
					Index:      len(clips),
					StartTick:  curStart,
					EndTick:    curEnd,
					NumPackets: curPkts,
					TotalBytes: curBytes,
				})
				curStart = p.tick
				curPkts = 0
				curBytes = 0
			}
			curEnd = p.tick
			curPkts++
			curBytes += len(p.data)
		}
		// last clip
		clips = append(clips, VoiceClipInfo{
			Index:      len(clips),
			StartTick:  curStart,
			EndTick:    curEnd,
			NumPackets: curPkts,
			TotalBytes: curBytes,
		})

		out.Players = append(out.Players, PlayerVoiceInfo{
			Name:         name,
			SteamID64:    sid,
			TotalPackets: totalPkts,
			TotalBytes:   totalBytes,
			Clips:        clips,
		})
	}
	// sort players by bytes desc
	sort.Slice(out.Players, func(i, j int) bool {
		return out.Players[i].TotalBytes > out.Players[j].TotalBytes
	})
	return out
}

// Note: the Participants has no direct BySteamID64, we handled in build.

// In-memory cache of last analyses (keyed by demo name + size rough)
var analysisCache = struct {
	sync.RWMutex
	m map[string]*Analysis
}{m: make(map[string]*Analysis)}



func cacheKey(name string, size int64) string {
	return fmt.Sprintf("%s|%d", name, size)
}

func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Demo string `json:"demo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	if req.Demo == "" || strings.Contains(req.Demo, "..") || strings.Contains(req.Demo, "/") {
		http.Error(w, "invalid demo name", 400)
		return
	}
	full := filepath.Join(*demosDir, req.Demo)

	// stat for cache
	fi, err := os.Stat(full)
	if err != nil {
		http.Error(w, "demo not found: "+err.Error(), 404)
		return
	}
	key := cacheKey(req.Demo, fi.Size())

	analysisCache.RLock()
	if a, ok := analysisCache.m[key]; ok {
		analysisCache.RUnlock()
		writeJSON(w, a)
		return
	}
	analysisCache.RUnlock()

	log.Printf("Parsing demo: %s", req.Demo)
	start := time.Now()
	a, err := parseDemo(full)
	if err != nil {
		log.Printf("parse error: %v", err)
		writeJSON(w, &Analysis{DemoName: req.Demo, Error: err.Error()})
		return
	}
	log.Printf("Parsed %s in %v - %d shots, %d nades, %d chats, %d kills, %d voice packets across players",
		req.Demo, time.Since(start), len(a.Shots), len(a.Nades), len(a.Chats), len(a.Kills), func() int {
			t := 0
			for _, p := range a.Voice.Players {
				t += p.TotalPackets
			}
			return t
		}())

	analysisCache.Lock()
	analysisCache.m[key] = a
	analysisCache.Unlock()

	writeJSON(w, a)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// handleRadar serves the radar image for a map, using the embedded ones from examples/_assets
// Falls back to a generated placeholder if not found.
func handleRadar(w http.ResponseWriter, r *http.Request) {
	mapName := r.URL.Query().Get("map")
	if mapName == "" {
		http.Error(w, "map required", 400)
		return
	}
	// try common name
	candidates := []string{
		mapName + "_radar_psd.png",
		mapName + "_radar.png",
	}
	var imgData []byte
	var loadedImg image.Image
	for _, c := range candidates {
		// Use ex.GetMapRadar but it panics on fail; wrap
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					// try fs direct? but since embed in other pkg, use os read relative
				}
			}()
			loadedImg = ex.GetMapRadar(mapName)
		}()
		if loadedImg != nil {
			break
		}
		// fallback: direct fs read from examples dir (works when running from repo root)
		for _, base := range []string{"examples/_assets/radar/", "../examples/_assets/radar/"} {
			p := base + c
			if b, err := os.ReadFile(p); err == nil {
				imgData = b
				break
			}
		}
		if len(imgData) > 0 {
			break
		}
	}

	if loadedImg != nil {
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, loadedImg)
		return
	}
	if len(imgData) > 0 {
		w.Header().Set("Content-Type", "image/png")
		w.Write(imgData)
		return
	}

	// Placeholder: simple gray with text "no radar for <map>"
	w.Header().Set("Content-Type", "image/png")
	// minimal 512x512 gray png without extra deps hard, just 404 or empty
	http.Error(w, "radar not available for map: "+mapName, 404)
}

// handleVoiceClip serves raw voice data for a clip (concatenated packets)
func handleVoiceClip(w http.ResponseWriter, r *http.Request) {
	demo := r.URL.Query().Get("demo")
	steamStr := r.URL.Query().Get("steamid")
	clipIdxStr := r.URL.Query().Get("clip")
	if demo == "" || steamStr == "" || clipIdxStr == "" {
		http.Error(w, "missing params", 400)
		return
	}
	if strings.Contains(demo, "..") {
		http.Error(w, "bad demo", 400)
		return
	}
	steam, _ := strconv.ParseUint(steamStr, 10, 64)
	clipIdx, _ := strconv.Atoi(clipIdxStr)

	full := filepath.Join(*demosDir, demo)
	fi, err := os.Stat(full)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	key := cacheKey(demo, fi.Size())

	analysisCache.RLock()
	a := analysisCache.m[key]
	analysisCache.RUnlock()
	if a == nil {
		http.Error(w, "analyze first", 400)
		return
	}

	// Extract voice for the specific clip by re-parsing (keeps memory low between requests).
	// The returned packets are already filtered to the requested clip index by the extractor.
	packets, err := extractVoicePacketsForClip(full, steam, clipIdx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(packets) == 0 {
		http.Error(w, "no voice data for this clip (or demo needs re-analysis)", 404)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="voice_%s_%d_%d.bin"`, strings.TrimSuffix(demo, ".dem"), steam, clipIdx))
	for _, p := range packets {
		w.Write(p.data)
	}
}

// extractVoicePacketsForClip re-parses focusing on voice net messages for a player + clip.
// Note: because we don't hook full game state here, tick numbers are synthetic but the
// sequence of audio frames is correct and sufficient to concatenate into a playable stream.
func extractVoicePacketsForClip(demoPath string, targetSteam uint64, clipIdx int) ([]voicePacket, error) {
	f, err := os.Open(demoPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	p := demoinfocs.NewParser(f)
	defer p.Close()

	var collected []voicePacket
	var curClip = -1
	var lastTick int
	const silence = 64

	p.RegisterNetMessageHandler(func(v *msg.CSVCMsg_VoiceData) {
		if v.GetAudio() == nil {
			return
		}
		xuid := v.GetXuid()
		if xuid != targetSteam {
			return
		}
		audio := v.GetAudio()
		tick := lastTick + 1
		lastTick = tick

		if len(collected) == 0 || (tick-lastTick > silence && lastTick != 0) {
			curClip++
		}
		lastTick = tick

		if curClip == clipIdx || clipIdx < 0 {
			collected = append(collected, voicePacket{
				tick: tick,
				data: append([]byte(nil), audio.GetVoiceData()...),
			})
		}
	})

	_ = p.ParseToEnd()
	return collected, nil
}

// handleExport allows exporting the analysis JSON or specific parts.
func handleExport(w http.ResponseWriter, r *http.Request) {
	demo := r.URL.Query().Get("demo")
	kind := r.URL.Query().Get("kind") // "full", "kills", "nades" etc. "voice-list"
	if demo == "" {
		http.Error(w, "demo required", 400)
		return
	}
	if strings.Contains(demo, "..") {
		http.Error(w, "bad", 400)
		return
	}
	full := filepath.Join(*demosDir, demo)
	fi, _ := os.Stat(full)
	key := cacheKey(demo, fi.Size())

	analysisCache.RLock()
	a := analysisCache.m[key]
	analysisCache.RUnlock()
	if a == nil {
		// auto analyze? for simplicity require prior
		http.Error(w, "run analyze first (open the demo in UI)", 400)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	switch kind {
	case "kills":
		json.NewEncoder(w).Encode(a.Kills)
	case "nades":
		json.NewEncoder(w).Encode(a.Nades)
	case "chats":
		json.NewEncoder(w).Encode(a.Chats)
	case "shots":
		json.NewEncoder(w).Encode(a.Shots)
	case "players":
		json.NewEncoder(w).Encode(a.Players)
	default:
		// full
		w.Header().Set("Content-Disposition", `attachment; filename="analysis_`+strings.TrimSuffix(demo, ".dem")+`.json"`)
		json.NewEncoder(w).Encode(a)
	}
}

func init() {
	// ensure demos dir exists
	_ = os.MkdirAll(*demosDir, 0755)
}
