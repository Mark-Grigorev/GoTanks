package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	"github.com/Mark-Grigorev/GoTanks/internal/store"
)

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

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return exitConfigError
	}

	authSvc, err := newAuthService(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth: %v\n", err)
		return exitAuthError
	}

	ctx := context.Background()

	if err := store.Migrate(cfg.DBConnString); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		return exitDBMigrate
	}

	db, err := store.New(ctx, cfg.DBConnString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db connect: %v\n", err)
		return exitDBConnect
	}
	defer db.Close()

	l, err := loader.Load("tanks", "maps")
	if err != nil {
		fmt.Fprintf(os.Stderr, "loader: %v\n", err)
		return exitConfigError
	}
	log.Printf("loaded %d tanks, %d maps", len(l.Tanks), len(l.Maps))

	h := hub.New(l, db, authSvc, cfg.MaxPlayers, cfg.TickRate)
	hand := handler.New(authSvc, db, l, h)

	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: hand.Router(),
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("server listening on :%s", cfg.AppPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown: %v\n", err)
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
