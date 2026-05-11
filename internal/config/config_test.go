package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DB_CONN_STRING", "postgres://localhost/test")
	t.Setenv("BOT_TOKEN", "testtoken123")
	t.Setenv("JWT_SECRET", "secret-at-least-16-chars")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.AppPort)
	assert.Equal(t, 20, cfg.TickRate)
	assert.Equal(t, 4, cfg.MaxPlayers)
	assert.Equal(t, "postgres://localhost/test", cfg.DBConnString)
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("DB_CONN_STRING", "postgres://db/gotanks")
	t.Setenv("BOT_TOKEN", "mybot")
	t.Setenv("JWT_SECRET", "my-jwt-secret-16ch")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("TICK_RATE", "30")
	t.Setenv("MAX_PLAYERS", "8")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "9090", cfg.AppPort)
	assert.Equal(t, 30, cfg.TickRate)
	assert.Equal(t, 8, cfg.MaxPlayers)
}

func TestLoad_MissingRequired(t *testing.T) {
	for _, key := range []string{"DB_CONN_STRING", "BOT_TOKEN", "JWT_SECRET"} {
		old, existed := os.LookupEnv(key)
		os.Unsetenv(key)
		if existed {
			t.Cleanup(func() { os.Setenv(key, old) })
		}
	}

	_, err := Load()
	assert.Error(t, err)
}
