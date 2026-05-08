package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	AppPort      string        `envconfig:"APP_PORT"      default:"8080"`
	AppEnv       string        `envconfig:"APP_ENV"       default:"local"`
	DBConnString string        `envconfig:"DB_CONN_STRING" required:"true"`
	BotToken     string        `envconfig:"BOT_TOKEN"     required:"true"`
	JWTSecret    string        `envconfig:"JWT_SECRET"    required:"true"`
	JWTDuration  time.Duration `envconfig:"JWT_DURATION"  default:"24h"`
	TickRate     int           `envconfig:"TICK_RATE"     default:"20"`
	MaxPlayers   int           `envconfig:"MAX_PLAYERS"   default:"4"`
}

func Load() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, err
	}
	return &c, nil
}
