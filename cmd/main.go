package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Mark-Grigorev/GoTanks/internal/auth"
	"github.com/Mark-Grigorev/GoTanks/internal/config"
	"github.com/Mark-Grigorev/GoTanks/internal/handler"
	"github.com/Mark-Grigorev/GoTanks/internal/hub"
	"github.com/Mark-Grigorev/GoTanks/internal/loader"
	"github.com/Mark-Grigorev/GoTanks/internal/logger"
	"github.com/Mark-Grigorev/GoTanks/internal/store"
)

//go:embed web
var webFiles embed.FS

const (
	exitOK             = 0
	exitConfigError    = 2
	exitAuthError      = 3
	exitDBConnect      = 4
	exitDBMigrate      = 5
	exitServerShutdown = 6
)

func main() {
	os.Exit(run())
}

func run() int {
	_ = godotenv.Load()

	log := logger.New(true) // hardcode - DEGUG=true

	cfg, err := config.Load()
	if err != nil {
		log.Errorf("config: %v", err)
		return exitConfigError
	}

	authSvc, err := newAuthService(cfg)
	if err != nil {
		log.Errorf("auth: %v", err)
		return exitAuthError
	}

	ctx := context.Background()

	if err := store.Migrate(cfg.DBConnString); err != nil {
		log.Errorf("migrate: %v", err)
		return exitDBMigrate
	}

	db, err := store.New(ctx, cfg.DBConnString)
	if err != nil {
		log.Errorf("db connect: %v", err)
		return exitDBConnect
	}
	defer db.Close()

	l, err := loader.Load()
	if err != nil {
		log.Errorf("loader: %v", err)
		return exitConfigError
	}
	log.Infof("loaded %d tanks, %d maps", len(l.Tanks), len(l.Maps))

	webRoot, _ := fs.Sub(webFiles, "web")

	h := hub.New(l, db, authSvc, cfg.MaxPlayers, cfg.TickRate)
	hand := handler.New(authSvc, db, l, h, webRoot)

	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: hand.Router(),
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Infof("server listening on :%s", cfg.AppPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("server: %v", err)
		}
	}()

	<-quit
	log.Infof("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Errorf("shutdown: %v", err)
		return exitServerShutdown
	}
	return exitOK
}

func newAuthService(cfg *config.Config) (*auth.Service, error) {
	if len(cfg.JWTSecret) < 16 {
		return nil, fmt.Errorf("JWT_SECRET too short (min 16 chars)")
	}
	return auth.NewService(cfg.BotToken, cfg.JWTSecret, cfg.JWTDuration), nil
}
