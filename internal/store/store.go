package store

import (
	"context"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
)

//go:embed migrations
var migrationsFS embed.FS

type Store struct {
	db *pgxpool.Pool
}

func New(ctx context.Context, connStr string) (*Store, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{db: pool}, nil
}

func (s *Store) Close() {
	s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}

func Migrate(connStr string) error {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("iofs: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, connStr)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func (s *Store) UpsertUser(ctx context.Context, telegramID int64, username string) (*User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		INSERT INTO users (telegram_id, username)
		VALUES ($1, $2)
		ON CONFLICT (telegram_id) DO UPDATE SET username = EXCLUDED.username
		RETURNING id, telegram_id, username, created_at
	`, telegramID, username).Scan(&u.ID, &u.TelegramID, &u.Username, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO user_stats (user_id) VALUES ($1) ON CONFLICT DO NOTHING
	`, u.ID)
	if err != nil {
		return nil, fmt.Errorf("init stats: %w", err)
	}
	return &u, nil
}

func (s *Store) GetStats(ctx context.Context, userID int64) (*UserStats, error) {
	var st UserStats
	err := s.db.QueryRow(ctx, `
		SELECT user_id, wins, losses, kills, deaths FROM user_stats WHERE user_id = $1
	`, userID).Scan(&st.UserID, &st.Wins, &st.Losses, &st.Kills, &st.Deaths)
	if err == pgx.ErrNoRows {
		return &UserStats{UserID: userID}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}
	return &st, nil
}

func (s *Store) Leaderboard(ctx context.Context, limit int) ([]LeaderboardEntry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.telegram_id, u.username, u.created_at,
		       s.wins, s.losses, s.kills, s.deaths
		FROM users u
		JOIN user_stats s ON s.user_id = u.id
		ORDER BY s.wins DESC, s.kills DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: %w", err)
	}
	defer rows.Close()

	var result []LeaderboardEntry
	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(
			&e.User.ID, &e.User.TelegramID, &e.User.Username, &e.User.CreatedAt,
			&e.Stats.Wins, &e.Stats.Losses, &e.Stats.Kills, &e.Stats.Deaths,
		); err != nil {
			return nil, err
		}
		e.Stats.UserID = e.User.ID
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *Store) CreateMatch(ctx context.Context, mapID string) (*Match, error) {
	var m Match
	err := s.db.QueryRow(ctx, `
		INSERT INTO matches (map_id) VALUES ($1)
		RETURNING id, map_id, started_at, finished_at
	`, mapID).Scan(&m.ID, &m.MapID, &m.StartedAt, &m.FinishedAt)
	if err != nil {
		return nil, fmt.Errorf("create match: %w", err)
	}
	return &m, nil
}

func (s *Store) FinishMatch(ctx context.Context, matchID int64, players []MatchPlayer) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `UPDATE matches SET finished_at = NOW() WHERE id = $1`, matchID)
	if err != nil {
		return fmt.Errorf("finish match: %w", err)
	}

	for _, p := range players {
		_, err = tx.Exec(ctx, `
			INSERT INTO match_players (match_id, user_id, tank_type, result) VALUES ($1, $2, $3, $4)
		`, matchID, p.UserID, p.TankType, p.Result)
		if err != nil {
			return fmt.Errorf("insert match player: %w", err)
		}

		if p.Result == "win" {
			_, err = tx.Exec(ctx, `
				INSERT INTO user_stats (user_id, wins) VALUES ($1, 1)
				ON CONFLICT (user_id) DO UPDATE SET wins = user_stats.wins + 1
			`, p.UserID)
		} else {
			_, err = tx.Exec(ctx, `
				INSERT INTO user_stats (user_id, losses) VALUES ($1, 1)
				ON CONFLICT (user_id) DO UPDATE SET losses = user_stats.losses + 1
			`, p.UserID)
		}
		if err != nil {
			return fmt.Errorf("update stats: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordKill(ctx context.Context, killerID, victimID int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO user_stats (user_id, kills) VALUES ($1, 1)
		ON CONFLICT (user_id) DO UPDATE SET kills = user_stats.kills + 1
	`, killerID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO user_stats (user_id, deaths) VALUES ($1, 1)
		ON CONFLICT (user_id) DO UPDATE SET deaths = user_stats.deaths + 1
	`, victimID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
