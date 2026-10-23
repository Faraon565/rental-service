package http

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const userIDContextKey contextKey = "user_id"

func (h *handler) JWTMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := getUserIDFromRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		next(w, r.WithContext(ctx))
	}
}

func getUserIDFromRequest(r *http.Request) (int64, error) {
	authorizationHeader := r.Header.Get("Authorization")
	if authorizationHeader == "" {
		return 0, errors.New("authorization header is required")
	}

	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authorizationHeader, bearerPrefix) {
		return 0, errors.New("authorization header must be Bearer token")
	}

	tokenString := strings.TrimSpace(strings.TrimPrefix(authorizationHeader, bearerPrefix))
	if tokenString == "" {
		return 0, errors.New("token is required")
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return []byte(getJWTSecret()), nil
	})
	if err != nil || !token.Valid {
		return 0, errors.New("invalid token")
	}

	userIDValue, ok := claims["user_id"]
	if !ok {
		return 0, errors.New("user_id not found in token")
	}

	userID, ok := userIDValue.(float64)
	if !ok || userID <= 0 {
		return 0, errors.New("invalid user_id in token")
	}

	return int64(userID), nil
}

func getUserIDFromContext(ctx context.Context) (int64, error) {
	userID, ok := ctx.Value(userIDContextKey).(int64)
	if !ok || userID <= 0 {
		return 0, errors.New("user_id not found in request context")
	}

	return userID, nil
}

func getJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "dev-secret-key"
	}

	return secret
}
