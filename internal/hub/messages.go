package hub

import "github.com/Mark-Grigorev/GoTanks/internal/game"

// ClientMsg — сообщение от клиента к серверу.
type ClientMsg struct {
	Type     string          `json:"type"`
	MapID    string          `json:"map_id,omitempty"`
	RoomID   string          `json:"room_id,omitempty"`
	TankType string          `json:"tank_type,omitempty"`
	Input    game.PlayerInput `json:"input,omitempty"`
}

// ServerMsg — сообщение от сервера к клиенту.
type ServerMsg struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

// Payload types

type RoomCreatedPayload struct {
	RoomID string `json:"room_id"`
}

type RoomJoinedPayload struct {
	RoomID      string `json:"room_id"`
	PlayerID    string `json:"player_id"`
	PlayerCount int    `json:"player_count"`
	MaxPlayers  int    `json:"max_players"`
}

type PlayerJoinedPayload struct {
	PlayerID    string `json:"player_id"`
	Username    string `json:"username"`
	PlayerCount int    `json:"player_count"`
}

type PlayerLeftPayload struct {
	PlayerID    string `json:"player_id"`
	PlayerCount int    `json:"player_count"`
}

type GameStartPayload struct {
	Map     MapPayload      `json:"map"`
	Players []PlayerPayload `json:"players"`
}

type MapPayload struct {
	ID     string    `json:"id"`
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Tiles  [][]int   `json:"tiles"`
}

type PlayerPayload struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	TankType string `json:"tank_type"`
}

type GameOverPayload struct {
	WinnerID string `json:"winner_id"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

type RoomListEntry struct {
	ID          string `json:"id"`
	MapID       string `json:"map_id"`
	PlayerCount int    `json:"player_count"`
	MaxPlayers  int    `json:"max_players"`
}
