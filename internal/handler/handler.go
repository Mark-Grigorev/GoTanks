package handler

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Mark-Grigorev/GoTanks/internal/auth"
	"github.com/Mark-Grigorev/GoTanks/internal/hub"
	"github.com/Mark-Grigorev/GoTanks/internal/loader"
	"github.com/Mark-Grigorev/GoTanks/internal/store"
)

type Handler struct {
	auth   *auth.Service
	store  *store.Store
	loader *loader.Loader
	hub    *hub.Hub
	web    fs.FS
}

func New(a *auth.Service, s *store.Store, l *loader.Loader, h *hub.Hub, web fs.FS) *Handler {
	return &Handler{auth: a, store: s, loader: l, hub: h, web: web}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(chimw.Logger)

	r.Get("/health", h.Health)
	r.Handle("/*", http.FileServer(http.FS(h.web)))

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth", h.Auth)
		r.Get("/tanks", h.Tanks)
		r.Get("/maps", h.Maps)
		r.Get("/rooms", h.RoomList)
		r.Get("/leaderboard", h.Leaderboard)

		r.Group(func(r chi.Router) {
			r.Use(h.JWTMiddleware)
			r.Get("/stats/me", h.MyStats)
		})
	})

	// WebSocket (token via query param)
	r.Get("/ws", h.hub.ServeWS)

	return r
}

// Health GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Status string `json:"status"`
		DB     string `json:"db"`
	}

	if err := h.store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, response{Status: "degraded", DB: "unreachable"})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "ok", DB: "ok"})
}

// Auth POST /api/auth
func (h *Handler) Auth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InitData string `json:"init_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	tgUser, err := h.auth.ValidateInitData(body.InitData)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid init_data")
		return
	}

	username := tgUser.Username
	if username == "" {
		username = tgUser.FirstName
	}

	user, err := h.store.UpsertUser(r.Context(), tgUser.ID, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	token, err := h.auth.IssueToken(user.ID, user.TelegramID, user.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// Tanks GET /api/tanks
func (h *Handler) Tanks(w http.ResponseWriter, r *http.Request) {
	tanks := make([]*loader.TankConfig, 0, len(h.loader.Tanks))
	for _, t := range h.loader.Tanks {
		tanks = append(tanks, t)
	}
	writeJSON(w, http.StatusOK, tanks)
}

// Maps GET /api/maps
func (h *Handler) Maps(w http.ResponseWriter, r *http.Request) {
	type mapInfo struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	maps := make([]mapInfo, 0, len(h.loader.Maps))
	for _, m := range h.loader.Maps {
		maps = append(maps, mapInfo{ID: m.ID, Name: m.Name, Width: m.Width, Height: m.Height})
	}
	writeJSON(w, http.StatusOK, maps)
}

// RoomList GET /api/rooms
func (h *Handler) RoomList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.hub.Rooms())
}

// Leaderboard GET /api/leaderboard
func (h *Handler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	entries, err := h.store.Leaderboard(r.Context(), 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// MyStats GET /api/stats/me  (requires JWT)
func (h *Handler) MyStats(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r.Context())
	stats, err := h.store.GetStats(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
