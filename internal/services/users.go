package services

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"travel-service/internal/auth"
	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
)

type UserService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewUserService() *UserService {
	return &UserService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("UserService")}
}

func publicUser(u models.User) types.PublicUser {
	return types.PublicUser{ID: u.ID, Name: u.Name, Email: u.Email, PhotoURL: u.PhotoUrl}
}

func (s *UserService) Register(ctx context.Context, in types.RegisterInput) (*types.AuthResponse, error) {
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if in.Email == "" || !strings.Contains(in.Email, "@") {
		return nil, apperror.UnprocessableEntity("a valid email is required")
	}
	if len(in.Password) < 8 {
		return nil, apperror.UnprocessableEntity("password must be at least 8 characters")
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, apperror.UnprocessableEntity("name is required")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, apperror.Internal("could not hash password")
	}
	u, err := s.q.InsertUser(ctx, models.InsertUserParams{Email: in.Email, PasswordHash: hash, Name: in.Name})
	if err != nil {
		return nil, apperror.FromDB(err) // unique violation -> Conflict
	}
	return s.authResponse(u)
}

func (s *UserService) Login(ctx context.Context, in types.LoginInput) (*types.AuthResponse, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))
	u, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.Unauthorized("invalid email or password")
		}
		return nil, apperror.FromDB(err)
	}
	if !auth.CheckPassword(u.PasswordHash, in.Password) {
		return nil, apperror.Unauthorized("invalid email or password")
	}
	return s.authResponse(u)
}

func (s *UserService) authResponse(u models.User) (*types.AuthResponse, error) {
	token, err := auth.GenerateToken(u.ID)
	if err != nil {
		return nil, apperror.Internal("could not issue token")
	}
	return &types.AuthResponse{Token: token, User: publicUser(u)}, nil
}

func (s *UserService) Me(ctx context.Context, userID uuid.UUID) (*types.PublicUser, error) {
	u, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	pu := publicUser(u)
	return &pu, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, userID uuid.UUID, in types.UpdateProfileInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return apperror.UnprocessableEntity("name is required")
	}
	return apperror.FromDBOrNil(s.q.UpdateUserProfile(ctx, models.UpdateUserProfileParams{
		Name: in.Name, PhotoUrl: in.PhotoURL, ID: userID,
	}))
}
