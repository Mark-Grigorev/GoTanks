package game

type Direction int

const (
	DirUp    Direction = 0
	DirRight Direction = 1
	DirDown  Direction = 2
	DirLeft  Direction = 3
)

type Tank struct {
	PlayerID          string
	UserID            int64
	X, Y              int
	Dir               Direction
	HP                int
	MaxHP             int
	TankType          string
	MoveInterval      int // ticks between moves
	MoveTicker        int
	ShootCooldown     int // ticks until next shot allowed
	ShootCooldownBase int // configured cooldown per tank type
	BulletSpeed       int // tiles per second
	BulletDamage      int
	Alive             bool
}

type Bullet struct {
	ID           string
	OwnerPlayerID string
	X, Y         int
	Dir          Direction
	MoveInterval int // ticks between moves
	MoveTicker   int
	Damage       int
}

type KillEvent struct {
	KillerPlayerID string
	VictimPlayerID string
	KillerUserID   int64
	VictimUserID   int64
}
