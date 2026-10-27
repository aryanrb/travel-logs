// Package db initializes and holds the PostgreSQL connection pool.
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"travel-service/config"
)

// Client is the shared pool; services call models.New(db.Client).
var Client *pgxpool.Pool

// MustConnectPG opens the pool or terminates the process on failure.
func MustConnectPG(ctx context.Context, logger *zap.Logger) *pgxpool.Pool {
	cfg, err := pgxpool.ParseConfig(config.Env.PostgresqlUrl)
	if err != nil {
		logger.Fatal("bad db connection string", zap.Error(err))
	}
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 20 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		logger.Fatal("db connection error", zap.Error(err))
	}
	Client = pool
	return pool
}
