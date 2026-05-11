package hub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/Mark-Grigorev/GoTanks/internal/auth"
	"github.com/Mark-Grigorev/GoTanks/internal/game"
	"github.com/Mark-Grigorev/GoTanks/internal/loader"
	"github.com/Mark-Grigorev/GoTanks/internal/store"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type roomEntry struct {
	room         *game.Room
	clients      map[string]*Client // playerID → client
	hostPlayerID string
}

type Hub struct {
	mu         sync.RWMutex
	rooms      map[string]*roomEntry
	clients    map[string]*Client // playerID → client
	loader     *loader.Loader
	store      *store.Store
	auth       *auth.Service
	maxPlayers int
	tickRate   int
}

func New(l *loader.Loader, s *store.Store, a *auth.Service, maxPlayers, tickRate int) *Hub {
	return &Hub{
		rooms:      make(map[string]*roomEntry),
		clients:    make(map[string]*Client),
		loader:     l,
		store:      s,
		auth:       a,
		maxPlayers: maxPlayers,
		tickRate:   tickRate,
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}
	claims, err := h.auth.ParseToken(tokenStr)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	playerID := newID()
	c := newClient(h, conn, claims.UserID, claims.Username, playerID)

	h.mu.Lock()
	h.clients[playerID] = c
	h.mu.Unlock()

	go c.writePump()
	go c.readPump()
}

func (h *Hub) handleClientMsg(c *Client, msg ClientMsg) {
	switch msg.Type {
	case "create_room":
		h.createRoom(c, msg.MapID, msg.TankType)
	case "join_room":
		h.joinRoom(c, msg.RoomID, msg.TankType)
	case "start_game":
		h.forceStart(c)
	case "input":
		h.handleInput(c, msg.Input)
	case "leave_room":
		h.leaveRoom(c)
	default:
		c.Send(ServerMsg{Type: "error", Payload: ErrorPayload{Message: "unknown message type"}})
	}
}

func (h *Hub) createRoom(c *Client, mapID, tankType string) {
	if mapID == "" {
		// use first available map
		for id := range h.loader.Maps {
			mapID = id
			break
		}
	}
	mapCfg, ok := h.loader.Maps[mapID]
	if !ok {
		c.Send(ServerMsg{Type: "error", Payload: ErrorPayload{Message: "unknown map"}})
		return
	}

	roomID := newID()
	room := game.NewRoom(roomID, mapCfg, h.loader.Tanks, h.maxPlayers, h.tickRate)

	if err := room.AddPlayer(c.PlayerID, c.UserID, c.Username, tankType); err != nil {
		c.Send(ServerMsg{Type: "error", Payload: ErrorPayload{Message: err.Error()}})
		return
	}

	entry := &roomEntry{
		room:         room,
		clients:      map[string]*Client{c.PlayerID: c},
		hostPlayerID: c.PlayerID,
	}
	h.setupCallbacks(room, roomID)

	h.mu.Lock()
	h.rooms[roomID] = entry
	h.mu.Unlock()

	c.RoomID = roomID
	c.Send(ServerMsg{Type: "room_created", Payload: RoomCreatedPayload{RoomID: roomID}})
	c.Send(ServerMsg{Type: "room_joined", Payload: RoomJoinedPayload{
		RoomID:      roomID,
		PlayerID:    c.PlayerID,
		PlayerCount: 1,
		MaxPlayers:  h.maxPlayers,
		IsHost:      true,
	}})
}

func (h *Hub) joinRoom(c *Client, roomID, tankType string) {
	h.mu.Lock()
	entry, ok := h.rooms[roomID]
	h.mu.Unlock()

	if !ok {
		c.Send(ServerMsg{Type: "error", Payload: ErrorPayload{Message: "room not found"}})
		return
	}

	if err := entry.room.AddPlayer(c.PlayerID, c.UserID, c.Username, tankType); err != nil {
		c.Send(ServerMsg{Type: "error", Payload: ErrorPayload{Message: err.Error()}})
		return
	}

	h.mu.Lock()
	entry.clients[c.PlayerID] = c
	count := len(entry.clients)
	h.mu.Unlock()

	c.RoomID = roomID
	c.Send(ServerMsg{Type: "room_joined", Payload: RoomJoinedPayload{
		RoomID:      roomID,
		PlayerID:    c.PlayerID,
		PlayerCount: count,
		MaxPlayers:  h.maxPlayers,
	}})

	h.broadcastRoom(roomID, c.PlayerID, ServerMsg{
		Type: "player_joined",
		Payload: PlayerJoinedPayload{
			PlayerID:    c.PlayerID,
			Username:    c.Username,
			PlayerCount: count,
		},
	})

	if entry.room.IsFull() {
		h.startGame(roomID, entry)
	}
}

func (h *Hub) forceStart(c *Client) {
	if c.RoomID == "" {
		return
	}
	h.mu.Lock()
	entry, ok := h.rooms[c.RoomID]
	h.mu.Unlock()
	if !ok || entry.room.State != game.RoomWaiting || entry.room.PlayerCount() == 0 {
		return
	}
	if entry.hostPlayerID != c.PlayerID {
		c.Send(ServerMsg{Type: "error", Payload: ErrorPayload{Message: "only host can start"}})
		return
	}
	h.startGame(c.RoomID, entry)
}

func (h *Hub) handleInput(c *Client, input game.PlayerInput) {
	if c.RoomID == "" {
		return
	}
	h.mu.RLock()
	entry, ok := h.rooms[c.RoomID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	entry.room.SetInput(c.PlayerID, input)
}

func (h *Hub) leaveRoom(c *Client) {
	if c.RoomID == "" {
		return
	}
	h.removeFromRoom(c)
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c.PlayerID)
	h.mu.Unlock()

	if c.RoomID != "" {
		h.removeFromRoom(c)
	}
	close(c.send)
}

func (h *Hub) removeFromRoom(c *Client) {
	roomID := c.RoomID
	c.RoomID = ""

	h.mu.Lock()
	entry, ok := h.rooms[roomID]
	if ok {
		delete(entry.clients, c.PlayerID)
	}
	h.mu.Unlock()

	if !ok {
		return
	}
	entry.room.RemovePlayer(c.PlayerID)

	count := entry.room.PlayerCount()
	h.broadcastRoom(roomID, "", ServerMsg{
		Type: "player_left",
		Payload: PlayerLeftPayload{
			PlayerID:    c.PlayerID,
			PlayerCount: count,
		},
	})

	if count == 0 {
		h.mu.Lock()
		delete(h.rooms, roomID)
		h.mu.Unlock()
	}
}

func (h *Hub) setupCallbacks(room *game.Room, roomID string) {
	room.OnStateUpdate = func(snap game.StateSnapshot) {
		msg := ServerMsg{Type: "state", Payload: snap}
		h.broadcastRoomMsg(roomID, msg)
	}
	room.OnKill = func(kill game.KillEvent) {
		go func() {
			if err := h.store.RecordKill(context.Background(), kill.KillerUserID, kill.VictimUserID); err != nil {
				log.Printf("record kill: %v", err)
			}
		}()
	}
	room.OnGameOver = func(winnerID string, players []*game.Player, _ []game.KillEvent) {
		h.broadcastRoomMsg(roomID, ServerMsg{
			Type:    "game_over",
			Payload: GameOverPayload{WinnerID: winnerID},
		})
		go h.finishMatch(winnerID, players)
	}
}

func (h *Hub) startGame(roomID string, entry *roomEntry) {
	mapCfg := entry.room.MapCfg

	tiles := make([][]int, len(mapCfg.Tiles))
	for i, row := range mapCfg.Tiles {
		tiles[i] = make([]int, len(row))
		copy(tiles[i], row)
	}

	var players []PlayerPayload
	h.mu.RLock()
	for _, c := range entry.clients {
		players = append(players, PlayerPayload{
			ID:       c.PlayerID,
			Username: c.Username,
		})
	}
	h.mu.RUnlock()

	h.broadcastRoomMsg(roomID, ServerMsg{
		Type: "game_start",
		Payload: GameStartPayload{
			Map: MapPayload{
				ID:     mapCfg.ID,
				Width:  mapCfg.Width,
				Height: mapCfg.Height,
				Tiles:  tiles,
			},
			Players: players,
		},
	})

	go entry.room.Start()
}

func (h *Hub) finishMatch(winnerID string, players []*game.Player) {
	match, err := h.store.CreateMatch(context.Background(), "")
	if err != nil {
		log.Printf("create match: %v", err)
		return
	}

	var matchPlayers []store.MatchPlayer
	for _, p := range players {
		result := "loss"
		if p.ID == winnerID {
			result = "win"
		}
		matchPlayers = append(matchPlayers, store.MatchPlayer{
			UserID:   p.UserID,
			TankType: p.TankType,
			Result:   result,
		})
	}
	if err := h.store.FinishMatch(context.Background(), match.ID, matchPlayers); err != nil {
		log.Printf("finish match: %v", err)
	}
}

func (h *Hub) broadcastRoom(roomID, excludePlayerID string, msg ServerMsg) {
	data, err := marshalMsg(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	entry, ok := h.rooms[roomID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	for pid, c := range entry.clients {
		if pid == excludePlayerID {
			continue
		}
		select {
		case c.send <- data:
		default:
		}
	}
}

func (h *Hub) broadcastRoomMsg(roomID string, msg ServerMsg) {
	h.broadcastRoom(roomID, "", msg)
}

func (h *Hub) Rooms() []RoomListEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]RoomListEntry, 0, len(h.rooms))
	for id, e := range h.rooms {
		if e.room.State == game.RoomWaiting {
			out = append(out, RoomListEntry{
				ID:          id,
				MapID:       e.room.MapCfg.ID,
				PlayerCount: len(e.clients),
				MaxPlayers:  h.maxPlayers,
			})
		}
	}
	return out
}

func newID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func marshalMsg(msg ServerMsg) ([]byte, error) {
	return json.Marshal(msg)
}
