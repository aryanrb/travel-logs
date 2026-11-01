// Package handlers are thin: parse the request, call a service, write the
// response. All business logic and permission checks live in the service layer.
package handlers

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"travel-service/internal/utils/apperror"
)

// pathID parses a UUID path parameter or returns a 400.
func pathID(c echo.Context, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		return uuid.Nil, apperror.BadRequest("invalid " + name)
	}
	return id, nil
}

// bind decodes the JSON body or returns a 400.
func bind(c echo.Context, dst any) error {
	if err := c.Bind(dst); err != nil {
		return apperror.BadRequest("invalid request body")
	}
	return nil
}

// queryInt returns an optional int query param, or nil when absent/unparseable.
func queryInt(c echo.Context, name string) *int {
	s := c.QueryParam(name)
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

// queryInt64 returns an optional int64 query param, or nil when absent/unparseable.
func queryInt64(c echo.Context, name string) *int64 {
	s := c.QueryParam(name)
	if s == "" {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

// intOr parses s as an int, falling back to def.
func intOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
