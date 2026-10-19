package service

import (
	"context"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"time"
)

type AuthService struct {
	Store  domain.Store
	Secret []byte
	TTL    time.Duration
}

func (s AuthService) Register(ctx context.Context, username, email, password string) (domain.User, string, error) {
	username = strings.TrimSpace(username)
	email = strings.ToLower(strings.TrimSpace(email))
	if len(username) < 2 || !strings.Contains(email, "@") || len(password) < 8 {
		return domain.User{}, "", fmt.Errorf("username, email, or password is invalid")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, "", err
	}
	u, err := s.Store.CreateUser(ctx, domain.User{ID: uuid.NewString(), Username: username, Email: email, PasswordHash: string(hash)})
	if err != nil {
		return domain.User{}, "", err
	}
	token, err := s.Token(u)
	return u, token, err
}
func (s AuthService) Login(ctx context.Context, login, password string) (domain.User, string, error) {
	u, err := s.Store.UserByLogin(ctx, strings.TrimSpace(login))
	if err != nil || u.Disabled || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return domain.User{}, "", fmt.Errorf("invalid credentials")
	}
	token, err := s.Token(u)
	return u, token, err
}
func (s AuthService) Token(u domain.User) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{Subject: u.ID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(s.TTL))}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.Secret)
}
func (s AuthService) Parse(token string) (string, error) {
	claims := jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.Secret, nil
	})
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return "", fmt.Errorf("invalid token")
	}
	return claims.Subject, nil
}
