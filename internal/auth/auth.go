package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Service struct {
	botToken  string
	jwtSecret []byte
	duration  time.Duration
}

type Claims struct {
	UserID     int64  `json:"user_id"`
	TelegramID int64  `json:"telegram_id"`
	Username   string `json:"username"`
	jwt.RegisteredClaims
}

type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

func NewService(botToken, jwtSecret string, duration time.Duration) *Service {
	return &Service{
		botToken:  botToken,
		jwtSecret: []byte(jwtSecret),
		duration:  duration,
	}
}

// ValidateInitData validates Telegram initData HMAC and extracts the user.
func (s *Service) ValidateInitData(initData string) (*TelegramUser, error) {
	vals, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("parse initData: %w", err)
	}

	receivedHash := vals.Get("hash")
	if receivedHash == "" {
		return nil, errors.New("missing hash")
	}

	var pairs []string
	for k, v := range vals {
		if k == "hash" {
			continue
		}
		pairs = append(pairs, k+"="+v[0])
	}
	sort.Strings(pairs)
	dataCheckString := strings.Join(pairs, "\n")

	// secret_key = HMAC-SHA256("WebAppData", bot_token)
	mac := hmac.New(sha256.New, []byte("WebAppData"))
	mac.Write([]byte(s.botToken))
	secretKey := mac.Sum(nil)

	mac2 := hmac.New(sha256.New, secretKey)
	mac2.Write([]byte(dataCheckString))
	expectedHash := hex.EncodeToString(mac2.Sum(nil))

	if !hmac.Equal([]byte(receivedHash), []byte(expectedHash)) {
		return nil, errors.New("invalid hash")
	}

	userJSON := vals.Get("user")
	if userJSON == "" {
		return nil, errors.New("missing user field in initData")
	}
	var user TelegramUser
	if err := json.Unmarshal([]byte(userJSON), &user); err != nil {
		return nil, fmt.Errorf("parse user: %w", err)
	}
	return &user, nil
}

func (s *Service) IssueToken(userID, telegramID int64, username string) (string, error) {
	claims := Claims{
		UserID:     userID,
		TelegramID: telegramID,
		Username:   username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
