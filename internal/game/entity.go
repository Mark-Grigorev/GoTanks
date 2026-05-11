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
	MoveInterval      int
	MoveTicker        int
	ShootCooldown     int
	ShootCooldownBase int
	BulletSpeed       int
	BulletDamage      int
	Alive             bool

	// Active power-up effects
	DamageBoostTicks int // ticks remaining; bullet damage ×2
	ArmorCharges     int // absorbs this many damage points
	SpeedBoostTicks  int // ticks remaining; move interval halved
}

type Bullet struct {
	ID            string
	OwnerPlayerID string
	X, Y          int
	Dir           Direction
	MoveInterval  int
	MoveTicker    int
	Damage        int
}

type KillEvent struct {
	KillerPlayerID string `json:"killer_id"`
	VictimPlayerID string `json:"victim_id"`
	KillerUserID   int64  `json:"killer_user_id"`
	VictimUserID   int64  `json:"victim_user_id"`
}

// ── Power-ups ──────────────────────────────────────────────────────────────

type PowerUpType string

const (
	PowerUpDamage PowerUpType = "damage" // bullet damage ×2 for 15 s
	PowerUpArmor  PowerUpType = "armor"  // absorbs 3 damage points
	PowerUpSpeed  PowerUpType = "speed"  // move speed ×2 for 10 s
	PowerUpHeal   PowerUpType = "heal"   // restores 2 HP
)

var PowerUpTypes = []PowerUpType{PowerUpDamage, PowerUpArmor, PowerUpSpeed, PowerUpHeal}

type PowerUp struct {
	ID   string
	Type PowerUpType
	X, Y int
}
