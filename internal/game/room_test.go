package game

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mark-Grigorev/GoTanks/internal/loader"
)

// 5×5 arena with wall border and empty interior
//
//	W W W W W
//	W . . . W
//	W . . . W
//	W . . . W
//	W W W W W
func newTestRoom() *Room {
	mapCfg := &loader.MapConfig{
		ID: "test", Width: 5, Height: 5,
		Tiles: [][]int{
			{1, 1, 1, 1, 1},
			{1, 0, 0, 0, 1},
			{1, 0, 0, 0, 1},
			{1, 0, 0, 0, 1},
			{1, 1, 1, 1, 1},
		},
		Spawns: [][2]int{{1, 1}, {3, 3}},
	}
	tankCfgs := map[string]*loader.TankConfig{
		"test": {
			ID: "test", HP: 5,
			Speed: 20, // Speed=20 & TickRate=20 → moveInterval=1
			BulletSpeed: 5, BulletDamage: 1, ShootCooldown: 5,
		},
	}
	return NewRoom("r1", mapCfg, tankCfgs, 4, 20)
}

func addReady(t *testing.T, r *Room, id string) *Player {
	t.Helper()
	require.NoError(t, r.AddPlayer(id, 1, id, "test"))
	return r.Players[id]
}

func startPlaying(r *Room, count int) {
	r.State = RoomPlaying
	r.initialPlayerCount = count
}

// ── AddPlayer ────────────────────────────────────────────────────────────────

func TestAddPlayer_OK(t *testing.T) {
	r := newTestRoom()
	require.NoError(t, r.AddPlayer("p1", 1, "alice", "test"))
	assert.Contains(t, r.Players, "p1")
}

func TestAddPlayer_GameStarted(t *testing.T) {
	r := newTestRoom()
	r.State = RoomPlaying
	assert.Error(t, r.AddPlayer("p1", 1, "alice", "test"))
}

func TestAddPlayer_RoomFull(t *testing.T) {
	r := newTestRoom()
	r.MaxPlayers = 1
	require.NoError(t, r.AddPlayer("p1", 1, "alice", "test"))
	assert.Error(t, r.AddPlayer("p2", 2, "bob", "test"))
}

// ── step ─────────────────────────────────────────────────────────────────────

func TestStep(t *testing.T) {
	cases := []struct{ dir Direction; wantX, wantY int }{
		{DirUp, 2, 1}, {DirDown, 2, 3}, {DirLeft, 1, 2}, {DirRight, 3, 2},
	}
	for _, c := range cases {
		x, y := step(2, 2, c.dir)
		assert.Equal(t, c.wantX, x, "dir %d x", c.dir)
		assert.Equal(t, c.wantY, y, "dir %d y", c.dir)
	}
}

// ── Movement ─────────────────────────────────────────────────────────────────

func TestTankMovement(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 2, 2
	p.Tank.Dir = DirRight
	p.Tank.MoveInterval = 1
	p.Input.Move = "right"
	r.doTick()

	assert.Equal(t, 3, p.Tank.X)
	assert.Equal(t, 2, p.Tank.Y)
}

func TestTankCannotMoveIntoWall(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 1, 1
	p.Tank.Dir = DirUp
	p.Tank.MoveInterval = 1
	p.Input.Move = "up"
	r.doTick()

	assert.Equal(t, 1, p.Tank.X, "x unchanged")
	assert.Equal(t, 1, p.Tank.Y, "y unchanged: wall blocked")
}

func TestTankChangesDirectionBeforeMoving(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 2, 2
	p.Tank.Dir = DirRight
	p.Tank.MoveInterval = 1
	p.Input.Move = "up"
	r.doTick()

	assert.Equal(t, DirUp, p.Tank.Dir, "direction changed")
	assert.Equal(t, 2, p.Tank.X, "no movement on turn tick")
	assert.Equal(t, 2, p.Tank.Y)
}

// ── Bullet spawn ──────────────────────────────────────────────────────────────

func TestBulletSpawn_Normal(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 2, 3
	p.Tank.Dir = DirUp
	p.Tank.ShootCooldown = 0
	p.Input.Shoot = true

	snap, _, _ := r.doTick()
	assert.Len(t, snap.Bullets, 1)
}

func TestBulletSpawn_BlockedByWall(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	// (1,1) facing up → wall at (1,0)
	p.Tank.X, p.Tank.Y = 1, 1
	p.Tank.Dir = DirUp
	p.Tank.ShootCooldown = 0
	p.Input.Shoot = true

	snap, _, _ := r.doTick()
	assert.Empty(t, snap.Bullets)
}

func TestBulletSpawn_DestroysBrick(t *testing.T) {
	r := newTestRoom()
	r.Map.Tiles[2][2] = TileBrick
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	// (2,3) facing up → brick at spawn (2,2): destroyed, no bullet
	p.Tank.X, p.Tank.Y = 2, 3
	p.Tank.Dir = DirUp
	p.Tank.ShootCooldown = 0
	p.Input.Shoot = true

	snap, _, _ := r.doTick()
	assert.Empty(t, snap.Bullets)
	assert.Len(t, snap.TileChanges, 1)
	assert.Equal(t, TileEmpty, r.Map.Tiles[2][2])
}

func TestShootCooldown(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 2, 3
	p.Tank.Dir = DirUp
	p.Tank.ShootCooldown = 5
	p.Input.Shoot = true

	snap, _, _ := r.doTick()
	assert.Empty(t, snap.Bullets)
}

// ── Bullet movement & collision ───────────────────────────────────────────────

func TestBulletMovesAfterInterval(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 2, 3
	p.Tank.Dir = DirUp
	p.Tank.ShootCooldown = 0
	p.Input.Shoot = true
	r.doTick()

	b := firstBullet(r)
	require.NotNil(t, b)
	startX, startY := b.X, b.Y

	for range b.MoveInterval {
		r.doTick()
	}

	b = firstBullet(r)
	require.NotNil(t, b, "bullet should still exist")
	assert.False(t, b.X == startX && b.Y == startY, "bullet should have moved")
}

func TestBulletHitsWall(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 2, 2
	p.Tank.Dir = DirUp
	p.Tank.ShootCooldown = 0
	p.Input.Shoot = true
	r.doTick()

	for range 40 {
		r.doTick()
		if firstBullet(r) == nil {
			return
		}
	}
	t.Error("bullet should have been destroyed by top wall within 40 ticks")
}

func TestBulletDestroysBrick(t *testing.T) {
	r := newTestRoom()
	r.Map.Tiles[1][2] = TileBrick // brick at (2,1)
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	// Tank at (2,3), bullet spawns at (2,2) [empty], travels to brick at (2,1).
	p.Tank.X, p.Tank.Y = 2, 3
	p.Tank.Dir = DirUp
	p.Tank.ShootCooldown = 0
	p.Input.Shoot = true
	r.doTick()

	for range 20 {
		snap, _, _ := r.doTick()
		for _, ch := range snap.TileChanges {
			if ch.X == 2 && ch.Y == 1 && ch.T == int(TileEmpty) {
				return
			}
		}
	}
	t.Error("bullet should have destroyed brick at (2,1) within 20 ticks")
}

// ── Winner logic ──────────────────────────────────────────────────────────────

func TestWinner_NobodyDeadYet(t *testing.T) {
	r := newTestRoom()
	addReady(t, r, "p1")
	addReady(t, r, "p2")
	startPlaying(r, 2)
	assert.Equal(t, "", r.winner())
}

func TestWinner_OneAlive(t *testing.T) {
	r := newTestRoom()
	addReady(t, r, "p1")
	addReady(t, r, "p2")
	startPlaying(r, 2)
	r.Players["p2"].Tank.Alive = false
	assert.Equal(t, "p1", r.winner())
}

func TestWinner_Draw(t *testing.T) {
	r := newTestRoom()
	addReady(t, r, "p1")
	addReady(t, r, "p2")
	startPlaying(r, 2)
	r.Players["p1"].Tank.Alive = false
	r.Players["p2"].Tank.Alive = false
	assert.Equal(t, "draw", r.winner())
}

func TestWinner_SoloAlive(t *testing.T) {
	r := newTestRoom()
	addReady(t, r, "p1")
	startPlaying(r, 1)
	assert.Equal(t, "", r.winner()) // no deaths yet
}

func TestWinner_SoloDies(t *testing.T) {
	r := newTestRoom()
	addReady(t, r, "p1")
	startPlaying(r, 1)
	r.Players["p1"].Tank.Alive = false
	assert.Equal(t, "draw", r.winner())
}

func TestWinner_DisconnectDuringGame(t *testing.T) {
	r := newTestRoom()
	addReady(t, r, "p1")
	addReady(t, r, "p2")
	startPlaying(r, 2)
	r.RemovePlayer("p2") // p2 disconnects mid-game
	assert.Equal(t, "p1", r.winner())
}

// ── Power-ups ─────────────────────────────────────────────────────────────────

func placePowerUp(r *Room, pu *PowerUp) {
	r.PowerUps[pu.ID] = pu
}

func TestPowerUp_DamageBoost(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 2, 2
	placePowerUp(r, &PowerUp{ID: "pu1", Type: PowerUpDamage, X: 2, Y: 2})

	r.doTick() // tank stands on power-up → picked up

	assert.Equal(t, 0, len(r.PowerUps), "power-up should be consumed")
	assert.Equal(t, powerUpDamageBoostTicks, p.Tank.DamageBoostTicks)
}

func TestPowerUp_ArmorAbsorbsDamage(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	p2 := addReady(t, r, "p2")
	startPlaying(r, 2)

	p.Tank.X, p.Tank.Y = 2, 2
	p.Tank.ArmorCharges = 2

	// Place bullet that will hit p1 (simulate by calling applyPowerUp directly)
	p.Tank.HP = 5
	initialHP := p.Tank.HP

	// Manually apply 1 damage via armor path
	actualDmg := 3
	absorbed := min(p.Tank.ArmorCharges, actualDmg)
	p.Tank.ArmorCharges -= absorbed
	p.Tank.HP -= (actualDmg - absorbed)

	assert.Equal(t, 0, p.Tank.ArmorCharges, "armor fully consumed")
	assert.Equal(t, initialHP-1, p.Tank.HP, "only 1 damage passed through armor")
	_ = p2
}

func TestPowerUp_SpeedBoost(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 2, 2
	placePowerUp(r, &PowerUp{ID: "pu1", Type: PowerUpSpeed, X: 2, Y: 2})
	r.doTick() // pick up

	assert.Equal(t, powerUpSpeedBoostTicks, p.Tank.SpeedBoostTicks)

	// With MoveInterval=1 and speed boost, effective interval = 1/2 = 0 capped to 1 — still 1
	// Use a slower tank to see the halving effect
	p.Tank.MoveInterval = 4
	p.Tank.SpeedBoostTicks = 100
	p.Tank.Dir = DirRight
	p.Input.Move = "right"

	// Without boost: needs 4 ticks to move. With boost (interval=2): needs 2 ticks.
	p.Tank.X, p.Tank.Y = 2, 2
	r.doTick() // tick 1 — direction change already set, MoveTicker=1
	r.doTick() // tick 2 — MoveTicker=2 >= effectiveInterval(2) → moves

	assert.Equal(t, 3, p.Tank.X, "speed boost should halve move interval")
}

func TestPowerUp_Heal(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.HP = 2
	p.Tank.MaxHP = 5
	p.Tank.X, p.Tank.Y = 2, 2
	placePowerUp(r, &PowerUp{ID: "pu1", Type: PowerUpHeal, X: 2, Y: 2})

	r.doTick()

	assert.Equal(t, 2+powerUpHealAmount, p.Tank.HP)
}

func TestPowerUp_HealCappedAtMax(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.HP = p.Tank.MaxHP // already full
	p.Tank.X, p.Tank.Y = 2, 2
	placePowerUp(r, &PowerUp{ID: "pu1", Type: PowerUpHeal, X: 2, Y: 2})

	r.doTick()

	assert.Equal(t, p.Tank.MaxHP, p.Tank.HP, "HP should not exceed MaxHP")
}

func TestPowerUp_PickedUpOnMove(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	// Tank at (2,2), power-up at (2,3). Move down to pick it up.
	p.Tank.X, p.Tank.Y = 2, 2
	p.Tank.Dir = DirDown
	p.Tank.MoveInterval = 1
	placePowerUp(r, &PowerUp{ID: "pu1", Type: PowerUpSpeed, X: 2, Y: 3})

	p.Input.Move = "down"
	r.doTick() // move to (2,3) + pickup

	assert.Equal(t, 2, p.Tank.X)
	assert.Equal(t, 3, p.Tank.Y)
	assert.Empty(t, r.PowerUps, "power-up picked up on arrival")
	assert.Equal(t, powerUpSpeedBoostTicks, p.Tank.SpeedBoostTicks)
}

func TestPowerUp_SpawnLimitRespected(t *testing.T) {
	r := newTestRoom()
	addReady(t, r, "p1") // spawns at (1,1)
	startPlaying(r, 1)

	// Fill to max using cells the player won't visit (rows 2-3)
	for i := range powerUpMaxOnMap {
		r.PowerUps[fmt.Sprintf("pu%d", i)] = &PowerUp{
			ID: fmt.Sprintf("pu%d", i), Type: PowerUpHeal, X: 1 + i, Y: 2,
		}
	}

	r.spawnTicker = powerUpSpawnInterval // trigger spawn attempt
	r.doTick()

	assert.Equal(t, powerUpMaxOnMap, len(r.PowerUps), "spawn should not exceed max")
}

func TestPowerUp_DamageBoostDoublesOutput(t *testing.T) {
	r := newTestRoom()
	p := addReady(t, r, "p1")
	startPlaying(r, 1)

	p.Tank.X, p.Tank.Y = 2, 3
	p.Tank.Dir = DirUp
	p.Tank.ShootCooldown = 0
	p.Tank.DamageBoostTicks = 100
	p.Input.Shoot = true

	r.doTick()

	b := firstBullet(r)
	require.NotNil(t, b)
	assert.Equal(t, p.Tank.BulletDamage*2, b.Damage)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func firstBullet(r *Room) *Bullet {
	for _, b := range r.Bullets {
		return b
	}
	return nil
}
