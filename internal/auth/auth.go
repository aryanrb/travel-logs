// Package auth handles password hashing, JWT issue/verify, and pulling the
// authenticated user id out of the request context.
package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"travel-service/config"
	"travel-service/internal/utils/apperror"
)

const ctxUserKey = "userID"
const tokenTTL = 7 * 24 * time.Hour

// HashPassword returns a bcrypt hash for storage.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether pw matches the stored hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// GenerateToken signs a JWT whose subject is the user id.
func GenerateToken(userID uuid.UUID) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.Env.JWTSecret))
}

// parseToken validates the signature and returns the user id.
func parseToken(raw string) (uuid.UUID, error) {
	tok, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(config.Env.JWTSecret), nil
	})
	if err != nil || !tok.Valid {
		return uuid.Nil, apperror.Unauthorized("invalid or expired token")
	}
	claims := tok.Claims.(*jwt.RegisteredClaims)
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, apperror.Unauthorized("invalid token subject")
	}
	return id, nil
}

// Middleware rejects requests without a valid bearer token and stores the
// user id in the echo context for handlers to read via UserID.
func Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		h := c.Request().Header.Get("Authorization")
		const p = "Bearer "
		if len(h) <= len(p) || h[:len(p)] != p {
			return apperror.Unauthorized("missing bearer token")
		}
		id, err := parseToken(h[len(p):])
		if err != nil {
			return err
		}
		c.Set(ctxUserKey, id)
		return next(c)
	}
}

// UserID returns the authenticated user id. Only valid behind Middleware.
func UserID(c echo.Context) uuid.UUID {
	id, _ := c.Get(ctxUserKey).(uuid.UUID)
	return id
}

// OptionalUserID returns the caller's id when a valid bearer token is present, or
// nil otherwise. For auth-optional routes (public discovery, profiles, comments)
// that are not behind Middleware but include the visitor's own state when signed in.
func OptionalUserID(c echo.Context) *uuid.UUID {
	h := c.Request().Header.Get("Authorization")
	const p = "Bearer "
	if len(h) <= len(p) || h[:len(p)] != p {
		return nil
	}
	id, err := parseToken(h[len(p):])
	if err != nil {
		return nil
	}
	return &id
}
