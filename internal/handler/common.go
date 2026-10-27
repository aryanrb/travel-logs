// Package handlers are thin: parse the request, call a service, write the
// response. All business logic and permission checks live in the service layer.
package handlers

import (
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
