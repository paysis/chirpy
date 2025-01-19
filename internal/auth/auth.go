package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	pw, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(pw), err
}

func CheckPasswordHash(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresInArgs ...time.Duration) (string, error) {
	var expiresIn time.Duration
	if len(expiresInArgs) == 0 {
		expiresIn = 1 * time.Hour
	} else {
		expiresIn = expiresInArgs[0]
	}

	claims := jwt.RegisteredClaims{
		Issuer:    "Chirpy",
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   userID.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(tokenSecret))
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(tokenSecret), nil
	})

	if err != nil {
		return uuid.UUID{}, err
	}

	claims := token.Claims
	sid, err := claims.GetSubject()

	if err != nil {
		return uuid.UUID{}, err
	}

	uid, err := uuid.Parse(sid)

	if err != nil {
		return uuid.UUID{}, err
	}

	return uid, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	h := headers.Get("Authorization")
	if h == "" {
		return "", fmt.Errorf("no authorization header found")
	}

	parts := strings.Split(h, " ")
	if len(parts) != 2 {
		return "", fmt.Errorf("authorization header value is invalid")
	}

	if parts[0] != "Bearer" {
		return "", fmt.Errorf("only support bearer tokens")
	}

	return parts[1], nil
}

func MakeRefreshToken() (string, error) {
	buf := make([]byte, 32)
	_, err := rand.Read(buf)

	if err != nil {
		return "", err
	}

	bufStr := hex.EncodeToString(buf)

	return bufStr, nil
}

func GetAPIKey(headers http.Header) (string, error) {
	h := headers.Get("Authorization")
	if h == "" {
		return "", fmt.Errorf("no authorization header found")
	}

	parts := strings.Split(h, " ")
	if len(parts) != 2 {
		return "", fmt.Errorf("authorization header value is invalid")
	}

	if parts[0] != "ApiKey" {
		return "", fmt.Errorf("only support bearer tokens")
	}

	return parts[1], nil
}
