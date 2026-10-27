package middlewares

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// ContextTimeout caps each request's context at 20s.
func ContextTimeout(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 20*time.Second)
		defer cancel()
		c.SetRequest(c.Request().WithContext(ctx))

		err := next(c)
		if ctx.Err() == context.DeadlineExceeded {
			zap.L().Error(c.Path(), zap.Error(ctx.Err()))
			return echo.NewHTTPError(http.StatusRequestTimeout, "Timeout Exceeded")
		}
		return err
	}
}
