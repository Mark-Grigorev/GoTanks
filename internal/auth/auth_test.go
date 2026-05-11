package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testBotToken  = "test_bot_token_123"
	testJWTSecret = "super-secret-jwt-key-32chars!!"
)

// buildInitData constructs a valid Telegram initData string signed with botToken.
func buildInitData(botToken string, user TelegramUser) string {
	userJSON, _ := json.Marshal(user)
	vals := url.Values{}
	vals.Set("user", string(userJSON))
	vals.Set("auth_date", "1700000000")

	var pairs []string
	for k, v := range vals {
		pairs = append(pairs, k+"="+v[0])
	}
	sort.Strings(pairs)
	dataCheckString := strings.Join(pairs, "\n")

	mac := hmac.New(sha256.New, []byte("WebAppData"))
	mac.Write([]byte(botToken))
	secretKey := mac.Sum(nil)

	mac2 := hmac.New(sha256.New, secretKey)
	mac2.Write([]byte(dataCheckString))
	vals.Set("hash", hex.EncodeToString(mac2.Sum(nil)))

	return vals.Encode()
}

func TestValidateInitData_Valid(t *testing.T) {
	svc := NewService(testBotToken, testJWTSecret, time.Hour)
	user := TelegramUser{ID: 42, FirstName: "Test", Username: "testuser"}

	got, err := svc.ValidateInitData(buildInitData(testBotToken, user))
	require.NoError(t, err)
	assert.Equal(t, int64(42), got.ID)
	assert.Equal(t, "testuser", got.Username)
}

func TestValidateInitData_MissingHash(t *testing.T) {
	svc := NewService(testBotToken, testJWTSecret, time.Hour)
	_, err := svc.ValidateInitData("user=%7B%22id%22%3A1%7D&auth_date=1700000000")
	assert.Error(t, err)
}

func TestValidateInitData_WrongHash(t *testing.T) {
	svc := NewService(testBotToken, testJWTSecret, time.Hour)
	vals, _ := url.ParseQuery(buildInitData(testBotToken, TelegramUser{ID: 1}))
	vals.Set("hash", strings.Repeat("a", 64))
	_, err := svc.ValidateInitData(vals.Encode())
	assert.Error(t, err)
}

func TestValidateInitData_WrongBotToken(t *testing.T) {
	svc := NewService("wrong_token", testJWTSecret, time.Hour)
	_, err := svc.ValidateInitData(buildInitData(testBotToken, TelegramUser{ID: 1}))
	assert.Error(t, err)
}

func TestIssueAndParseToken(t *testing.T) {
	svc := NewService(testBotToken, testJWTSecret, time.Hour)

	tokenStr, err := svc.IssueToken(1, 42, "alice")
	require.NoError(t, err)

	claims, err := svc.ParseToken(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, int64(1), claims.UserID)
	assert.Equal(t, int64(42), claims.TelegramID)
	assert.Equal(t, "alice", claims.Username)
}

func TestParseToken_Expired(t *testing.T) {
	svc := NewService(testBotToken, testJWTSecret, -time.Second)
	tokenStr, err := svc.IssueToken(1, 42, "alice")
	require.NoError(t, err)
	_, err = svc.ParseToken(tokenStr)
	assert.Error(t, err)
}

func TestParseToken_WrongSecret(t *testing.T) {
	svc1 := NewService(testBotToken, testJWTSecret, time.Hour)
	svc2 := NewService(testBotToken, "completely-different-secret!!", time.Hour)
	tokenStr, _ := svc1.IssueToken(1, 42, "alice")
	_, err := svc2.ParseToken(tokenStr)
	assert.Error(t, err)
}

func TestParseToken_Tampered(t *testing.T) {
	svc := NewService(testBotToken, testJWTSecret, time.Hour)
	tokenStr, _ := svc.IssueToken(1, 42, "alice")
	_, err := svc.ParseToken(tokenStr + "x")
	assert.Error(t, err)
}
