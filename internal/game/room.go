package game

import (
	"fmt"
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

type PlayerInput struct {
	Move  string `json:"move"`  // "up"|"down"|"left"|"right"|"none"
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
	ID   string    `json:"id"`
	X    int       `json:"x"`
	Y    int       `json:"y"`
	Dir  Direction `json:"dir"`
	HP   int       `json:"hp"`
	Type string    `json:"type"`
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

type StateSnapshot struct {
	Tick        int64            `json:"tick"`
	Tanks       []TankSnapshot   `json:"tanks"`
	Bullets     []BulletSnapshot `json:"bullets"`
	TileChanges []TileChange     `json:"tile_changes,omitempty"`
	Kills       []KillEvent      `json:"kills,omitempty"`
}

type Room struct {
	ID         string
	MapCfg     *loader.MapConfig
	Map        *GameMap
	Players    map[string]*Player
	Bullets    map[string]*Bullet
	State      RoomState
	MaxPlayers int
	TickRate   int

	mu                 sync.Mutex
	tickCount          int64
	bulletSeq          int64
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
	delete(r.Players, playerID)
}

func (r *Room) SetInput(playerID string, input PlayerInput) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.Players[playerID]; ok {
		p.Input.Move = input.Move
		if input.Shoot {
			p.Input.Shoot = true // only game loop clears this after firing
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

	// Move bullets and check collisions
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
				p.Tank.HP -= b.Damage
				if p.Tank.HP <= 0 {
					p.Tank.HP = 0
					p.Tank.Alive = false
					// find killer user ID
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

	// Process player inputs
	for _, p := range r.Players {
		if !p.Tank.Alive {
			continue
		}
		t := p.Tank

		if t.ShootCooldown > 0 {
			t.ShootCooldown--
		}

		if p.Input.Move != "" && p.Input.Move != "none" {
			dir := parseDir(p.Input.Move)
			if dir != t.Dir {
				t.Dir = dir
				t.MoveTicker = 0
			} else {
				t.MoveTicker++
				if t.MoveTicker >= t.MoveInterval {
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
			bulletMoveInterval := r.TickRate / t.BulletSpeed
			if bulletMoveInterval < 1 {
				bulletMoveInterval = 1
			}
			bid := fmt.Sprintf("b%d", atomic.AddInt64(&r.bulletSeq, 1))
			r.Bullets[bid] = &Bullet{
				ID:            bid,
				OwnerPlayerID: p.ID,
				X:             bx,
				Y:             by,
				Dir:           t.Dir,
				MoveInterval:  bulletMoveInterval,
				Damage:        t.BulletDamage,
			}
		}
	}

	winnerID := r.winner()

	snap := StateSnapshot{
		Tick:        tick,
		TileChanges: tileChanges,
		Kills:       kills,
	}
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
	for _, b := range r.Bullets {
		snap.Bullets = append(snap.Bullets, BulletSnapshot{
			ID:  b.ID,
			X:   b.X,
			Y:   b.Y,
			Dir: b.Dir,
		})
	}

	return snap, winnerID, kills
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
	if r.initialPlayerCount <= 1 {
		return "" // solo mode — game never ends automatically
	}
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
		return "" // никто ещё не умер — рано подводить итог
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
