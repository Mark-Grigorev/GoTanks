package store

import "time"

type User struct {
	ID         int64
	TelegramID int64
	Username   string
	CreatedAt  time.Time
}

type UserStats struct {
	UserID int64
	Wins   int
	Losses int
	Kills  int
	Deaths int
}

type Match struct {
	ID         int64
	MapID      string
	StartedAt  time.Time
	FinishedAt *time.Time
}

type MatchPlayer struct {
	MatchID  int64
	UserID   int64
	TankType string
	Result   string // "win" | "loss"
}

type LeaderboardEntry struct {
	User  User
	Stats UserStats
}
