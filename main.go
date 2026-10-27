package main

import (
	"context"
	"embed"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"travel-service/config"
	db "travel-service/internal/database"
	"travel-service/internal/middlewares"
	v1 "travel-service/internal/routes/v1"
	"travel-service/internal/utils/apperror"
)

//go:embed migrations/*.sql
var migrationsFiles embed.FS

func init() {
	zap.ReplaceGlobals(zap.Must(zap.NewProduction()))
	if err := godotenv.Load(); err != nil {
		zap.L().Info("no .env file; reading from runtime env")
	}
	config.InitConfig()
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Migrations are applied via `make migrate` (tern). Files embedded for reference.
	_ = migrationsFiles

	dbpool := db.MustConnectPG(ctx, zap.L())
	defer dbpool.Close()

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = apperror.CustomErrorHandler

	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.RequestID())
	e.Use(middleware.Secure())
	e.Use(middleware.Recover())
	e.Use(middlewares.ContextTimeout)
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI: true, LogStatus: true, LogMethod: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			zap.L().Info("request",
				zap.String("uri", v.URI), zap.Int("status", v.Status), zap.String("method", v.Method))
			return nil
		},
	}))

	e.GET("/", func(c echo.Context) error { return c.JSON(200, "Up and Running") })
	v1.Load(e.Group("/v1"))
	e.RouteNotFound("/*", func(c echo.Context) error { return c.JSON(http.StatusNotFound, "Route Not Found") })

	go func() {
		if err := e.Start(":" + config.Env.Port); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("server stopped", zap.Error(err))
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		zap.L().Fatal("shutdown error", zap.Error(err))
	}
}
