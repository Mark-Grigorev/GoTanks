package game

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mark-Grigorev/GoTanks/internal/loader"
)

type RoomState int

const (
	RoomWaiting  RoomState = 0
	RoomPlaying  RoomState = 1
	RoomFinished RoomState = 2
)

// Power-up tuning
const (
	powerUpSpawnInterval    = 600 // ticks between spawn attempts (~30 s at 20 TPS)
	powerUpMaxOnMap         = 3
	powerUpDamageBoostTicks = 300 // 15 s
	powerUpArmorCharges     = 3
	powerUpSpeedBoostTicks  = 200 // 10 s
	powerUpHealAmount       = 2
)

type PlayerInput struct {
	Move  string `json:"move"`
	Shoot bool   `json:"shoot"`
}

type Player struct {
	ID       string
	UserID   int64
	Username string
	TankType string
	Tank     *Tank
	Input    PlayerInput
}

type TankSnapshot struct {
	ID          string    `json:"id"`
	X           int       `json:"x"`
	Y           int       `json:"y"`
	Dir         Direction `json:"dir"`
	HP          int       `json:"hp"`
	Type        string    `json:"type"`
	DamageBoost bool      `json:"damage_boost,omitempty"`
	ArmorActive bool      `json:"armor_active,omitempty"`
	SpeedBoost  bool      `json:"speed_boost,omitempty"`
}

type BulletSnapshot struct {
	ID  string    `json:"id"`
	X   int       `json:"x"`
	Y   int       `json:"y"`
	Dir Direction `json:"dir"`
}

type TileChange struct {
	X int `json:"x"`
	Y int `json:"y"`
	T int `json:"t"`
}

type PowerUpSnapshot struct {
	ID   string      `json:"id"`
	Type PowerUpType `json:"type"`
	X    int         `json:"x"`
	Y    int         `json:"y"`
}

type StateSnapshot struct {
	Tick        int64             `json:"tick"`
	Tanks       []TankSnapshot    `json:"tanks"`
	Bullets     []BulletSnapshot  `json:"bullets"`
	PowerUps    []PowerUpSnapshot `json:"power_ups,omitempty"`
	TileChanges []TileChange      `json:"tile_changes,omitempty"`
	Kills       []KillEvent       `json:"kills,omitempty"`
}

type Room struct {
	ID         string
	MapCfg     *loader.MapConfig
	Map        *GameMap
	Players    map[string]*Player
	Bullets    map[string]*Bullet
	PowerUps   map[string]*PowerUp
	State      RoomState
	MaxPlayers int
	TickRate   int

	mu                 sync.Mutex
	tickCount          int64
	bulletSeq          int64
	powerUpSeq         int64
	spawnTicker        int
	tankCfgs           map[string]*loader.TankConfig
	initialPlayerCount int

	OnStateUpdate func(snapshot StateSnapshot)
	OnGameOver    func(winnerID string, players []*Player, kills []KillEvent)
	OnKill        func(kill KillEvent)
}

func NewRoom(id string, mapCfg *loader.MapConfig, tankCfgs map[string]*loader.TankConfig, maxPlayers, tickRate int) *Room {
	return &Room{
		ID:         id,
		MapCfg:     mapCfg,
		Map:        NewGameMap(mapCfg),
		Players:    make(map[string]*Player),
		Bullets:    make(map[string]*Bullet),
		PowerUps:   make(map[string]*PowerUp),
		State:      RoomWaiting,
		MaxPlayers: maxPlayers,
		TickRate:   tickRate,
		tankCfgs:   tankCfgs,
	}
}

func (r *Room) AddPlayer(playerID string, userID int64, username, tankType string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.State != RoomWaiting {
		return fmt.Errorf("game already started")
	}
	if len(r.Players) >= r.MaxPlayers {
		return fmt.Errorf("room is full")
	}

	spawnIdx := len(r.Players)
	if spawnIdx >= len(r.Map.Spawns) {
		return fmt.Errorf("no spawn point available")
	}
	spawn := r.Map.Spawns[spawnIdx]

	cfg, ok := r.tankCfgs[tankType]
	if !ok {
		for _, c := range r.tankCfgs {
			cfg = c
			tankType = c.ID
			break
		}
	}

	moveInterval := r.TickRate / cfg.Speed
	if moveInterval < 1 {
		moveInterval = 1
	}
	shootCooldown := cfg.ShootCooldown
	if shootCooldown <= 0 {
		shootCooldown = 10
	}

	r.Players[playerID] = &Player{
		ID:       playerID,
		UserID:   userID,
		Username: username,
		TankType: tankType,
		Tank: &Tank{
			PlayerID:          playerID,
			UserID:            userID,
			X:                 spawn[0],
			Y:                 spawn[1],
			Dir:               DirUp,
			HP:                cfg.HP,
			MaxHP:             cfg.HP,
			TankType:          tankType,
			MoveInterval:      moveInterval,
			ShootCooldownBase: shootCooldown,
			BulletSpeed:       cfg.BulletSpeed,
			BulletDamage:      cfg.BulletDamage,
			Alive:             true,
		},
	}
	return nil
}

func (r *Room) RemovePlayer(playerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.State == RoomPlaying {
		// Mark as dead so winner() counts the disconnect as a death.
		if p, ok := r.Players[playerID]; ok {
			p.Tank.Alive = false
			p.Tank.HP = 0
		}
	} else {
		delete(r.Players, playerID)
	}
}

func (r *Room) SetInput(playerID string, input PlayerInput) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.Players[playerID]; ok {
		p.Input.Move = input.Move
		if input.Shoot {
			p.Input.Shoot = true
		}
	}
}

func (r *Room) PlayerCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Players)
}

func (r *Room) IsFull() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Players) >= r.MaxPlayers
}

func (r *Room) Start() {
	r.mu.Lock()
	r.State = RoomPlaying
	r.initialPlayerCount = len(r.Players)
	r.mu.Unlock()

	ticker := time.NewTicker(time.Second / time.Duration(r.TickRate))
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		if r.State == RoomFinished {
			r.mu.Unlock()
			return
		}

		snapshot, winnerID, kills := r.doTick()

		var players []*Player
		if winnerID != "" {
			r.State = RoomFinished
			for _, p := range r.Players {
				cp := *p
				players = append(players, &cp)
			}
		}
		r.mu.Unlock()

		if r.OnStateUpdate != nil {
			r.OnStateUpdate(snapshot)
		}
		for _, k := range kills {
			if r.OnKill != nil {
				r.OnKill(k)
			}
		}
		if winnerID != "" {
			if r.OnGameOver != nil {
				r.OnGameOver(winnerID, players, kills)
			}
			return
		}
	}
}

// doTick runs one game tick. Must be called with r.mu held.
func (r *Room) doTick() (StateSnapshot, string, []KillEvent) {
	tick := atomic.AddInt64(&r.tickCount, 1)
	var tileChanges []TileChange
	var kills []KillEvent

	// ── Move bullets & check collisions ───────────────────────────────────────
	var deadBullets []string
	for id, b := range r.Bullets {
		b.MoveTicker++
		if b.MoveTicker < b.MoveInterval {
			continue
		}
		b.MoveTicker = 0

		nx, ny := step(b.X, b.Y, b.Dir)

		if r.Map.IsSolid(nx, ny) {
			if r.Map.TileAt(nx, ny) == TileBrick {
				r.Map.DestroyBrick(nx, ny)
				tileChanges = append(tileChanges, TileChange{nx, ny, int(TileEmpty)})
			}
			deadBullets = append(deadBullets, id)
			continue
		}

		hit := false
		for _, p := range r.Players {
			if !p.Tank.Alive || p.ID == b.OwnerPlayerID {
				continue
			}
			if p.Tank.X == nx && p.Tank.Y == ny {
				actualDmg := b.Damage
				if p.Tank.ArmorCharges > 0 {
					absorbed := min(p.Tank.ArmorCharges, actualDmg)
					p.Tank.ArmorCharges -= absorbed
					actualDmg -= absorbed
				}
				p.Tank.HP -= actualDmg
				if p.Tank.HP <= 0 {
					p.Tank.HP = 0
					p.Tank.Alive = false
					killerUserID := int64(0)
					if kp, ok := r.Players[b.OwnerPlayerID]; ok {
						killerUserID = kp.UserID
					}
					kills = append(kills, KillEvent{
						KillerPlayerID: b.OwnerPlayerID,
						VictimPlayerID: p.ID,
						KillerUserID:   killerUserID,
						VictimUserID:   p.UserID,
					})
				}
				deadBullets = append(deadBullets, id)
				hit = true
				break
			}
		}
		if !hit {
			b.X = nx
			b.Y = ny
		}
	}
	for _, id := range deadBullets {
		delete(r.Bullets, id)
	}

	// ── Process player inputs ─────────────────────────────────────────────────
	for _, p := range r.Players {
		if !p.Tank.Alive {
			continue
		}
		t := p.Tank

		// Tick down active buffs
		if t.DamageBoostTicks > 0 {
			t.DamageBoostTicks--
		}
		if t.SpeedBoostTicks > 0 {
			t.SpeedBoostTicks--
		}

		if t.ShootCooldown > 0 {
			t.ShootCooldown--
		}

		if p.Input.Move != "" && p.Input.Move != "none" {
			dir := parseDir(p.Input.Move)
			if dir != t.Dir {
				t.Dir = dir
				t.MoveTicker = 0
			} else {
				effectiveInterval := t.MoveInterval
				if t.SpeedBoostTicks > 0 && effectiveInterval > 1 {
					effectiveInterval = effectiveInterval / 2
				}
				t.MoveTicker++
				if t.MoveTicker >= effectiveInterval {
					t.MoveTicker = 0
					nx, ny := step(t.X, t.Y, t.Dir)
					if r.Map.IsWalkable(nx, ny) && !r.tankAt(nx, ny, p.ID) {
						t.X = nx
						t.Y = ny
					}
				}
			}
		} else {
			t.MoveTicker = 0
		}

		if p.Input.Shoot && t.ShootCooldown == 0 {
			p.Input.Shoot = false
			t.ShootCooldown = t.ShootCooldownBase

			bx, by := step(t.X, t.Y, t.Dir)
			if r.Map.IsSolid(bx, by) {
				if r.Map.TileAt(bx, by) == TileBrick {
					r.Map.DestroyBrick(bx, by)
					tileChanges = append(tileChanges, TileChange{bx, by, int(TileEmpty)})
				}
			} else {
				bulletMoveInterval := r.TickRate / t.BulletSpeed
				if bulletMoveInterval < 1 {
					bulletMoveInterval = 1
				}
				damage := t.BulletDamage
				if t.DamageBoostTicks > 0 {
					damage *= 2
				}
				bid := fmt.Sprintf("b%d", atomic.AddInt64(&r.bulletSeq, 1))
				r.Bullets[bid] = &Bullet{
					ID:            bid,
					OwnerPlayerID: p.ID,
					X:             bx,
					Y:             by,
					Dir:           t.Dir,
					MoveInterval:  bulletMoveInterval,
					Damage:        damage,
				}
			}
		}

		// ── Power-up pickup at current position ───────────────────────────────
		for pid, pu := range r.PowerUps {
			if pu.X == t.X && pu.Y == t.Y {
				r.applyPowerUp(t, pu)
				delete(r.PowerUps, pid)
				break
			}
		}
	}

	// ── Periodic power-up spawn ───────────────────────────────────────────────
	r.spawnTicker++
	if r.spawnTicker >= powerUpSpawnInterval && len(r.PowerUps) < powerUpMaxOnMap {
		r.spawnTicker = 0
		r.trySpawnPowerUp()
	}

	winnerID := r.winner()

	// ── Build snapshot ────────────────────────────────────────────────────────
	snap := StateSnapshot{
		Tick:        tick,
		TileChanges: tileChanges,
		Kills:       kills,
	}
	for _, p := range r.Players {
		snap.Tanks = append(snap.Tanks, TankSnapshot{
			ID:          p.ID,
			X:           p.Tank.X,
			Y:           p.Tank.Y,
			Dir:         p.Tank.Dir,
			HP:          p.Tank.HP,
			Type:        p.Tank.TankType,
			DamageBoost: p.Tank.DamageBoostTicks > 0,
			ArmorActive: p.Tank.ArmorCharges > 0,
			SpeedBoost:  p.Tank.SpeedBoostTicks > 0,
		})
	}
	for _, b := range r.Bullets {
		snap.Bullets = append(snap.Bullets, BulletSnapshot{
			ID:  b.ID,
			X:   b.X,
			Y:   b.Y,
			Dir: b.Dir,
		})
	}
	for _, pu := range r.PowerUps {
		snap.PowerUps = append(snap.PowerUps, PowerUpSnapshot{
			ID:   pu.ID,
			Type: pu.Type,
			X:    pu.X,
			Y:    pu.Y,
		})
	}

	return snap, winnerID, kills
}

func (r *Room) applyPowerUp(t *Tank, pu *PowerUp) {
	switch pu.Type {
	case PowerUpDamage:
		t.DamageBoostTicks = powerUpDamageBoostTicks
	case PowerUpArmor:
		t.ArmorCharges += powerUpArmorCharges
	case PowerUpSpeed:
		t.SpeedBoostTicks = powerUpSpeedBoostTicks
	case PowerUpHeal:
		t.HP = min(t.MaxHP, t.HP+powerUpHealAmount)
	}
}

func (r *Room) trySpawnPowerUp() {
	var candidates [][2]int
	for y := range r.Map.Height {
		for x := range r.Map.Width {
			if !r.Map.IsWalkable(x, y) {
				continue
			}
			occupied := false
			for _, p := range r.Players {
				if p.Tank.X == x && p.Tank.Y == y {
					occupied = true
					break
				}
			}
			if occupied {
				continue
			}
			for _, pu := range r.PowerUps {
				if pu.X == x && pu.Y == y {
					occupied = true
					break
				}
			}
			if !occupied {
				candidates = append(candidates, [2]int{x, y})
			}
		}
	}
	if len(candidates) == 0 {
		return
	}

	pos := candidates[rand.Intn(len(candidates))]
	puType := PowerUpTypes[rand.Intn(len(PowerUpTypes))]
	id := fmt.Sprintf("pu%d", atomic.AddInt64(&r.powerUpSeq, 1))
	r.PowerUps[id] = &PowerUp{ID: id, Type: puType, X: pos[0], Y: pos[1]}
}

func (r *Room) tankAt(x, y int, excludeID string) bool {
	for _, p := range r.Players {
		if p.ID == excludeID || !p.Tank.Alive {
			continue
		}
		if p.Tank.X == x && p.Tank.Y == y {
			return true
		}
	}
	return false
}

func (r *Room) winner() string {
	var alive []*Player
	var dead int
	for _, p := range r.Players {
		if p.Tank.Alive {
			alive = append(alive, p)
		} else {
			dead++
		}
	}
	if dead == 0 {
		return "" // nobody has died yet
	}
	switch len(alive) {
	case 0:
		return "draw"
	case 1:
		return alive[0].ID
	}
	return ""
}

func (r *Room) Snapshot() StateSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := StateSnapshot{Tick: r.tickCount}
	for _, p := range r.Players {
		snap.Tanks = append(snap.Tanks, TankSnapshot{
			ID:   p.ID,
			X:    p.Tank.X,
			Y:    p.Tank.Y,
			Dir:  p.Tank.Dir,
			HP:   p.Tank.HP,
			Type: p.Tank.TankType,
		})
	}
	return snap
}

func step(x, y int, dir Direction) (int, int) {
	switch dir {
	case DirUp:
		return x, y - 1
	case DirDown:
		return x, y + 1
	case DirLeft:
		return x - 1, y
	case DirRight:
		return x + 1, y
	}
	return x, y
}

func parseDir(s string) Direction {
	switch s {
	case "up":
		return DirUp
	case "down":
		return DirDown
	case "left":
		return DirLeft
	case "right":
		return DirRight
	}
	return DirUp
}
