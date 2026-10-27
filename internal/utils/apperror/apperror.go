package apperror

import (
	"errors"
	"net/http"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type FailedResponse struct {
	Success bool `json:"success"`
	Message any  `json:"message"`
}

// Error is a typed service-layer error carrying an HTTP status + message.
type Error struct {
	status int
	msg    string
}

func (e *Error) Error() string { return e.msg }
func (e *Error) Status() int   { return e.status }

func NotFound(msg string) *Error            { return &Error{http.StatusNotFound, msg} }
func Conflict(msg string) *Error            { return &Error{http.StatusConflict, msg} }
func BadRequest(msg string) *Error          { return &Error{http.StatusBadRequest, msg} }
func UnprocessableEntity(msg string) *Error { return &Error{http.StatusUnprocessableEntity, msg} }
func Unauthorized(msg string) *Error        { return &Error{http.StatusUnauthorized, msg} }
func Forbidden(msg string) *Error           { return &Error{http.StatusForbidden, msg} }
func Internal(msg string) *Error            { return &Error{http.StatusInternalServerError, msg} }

// FromDBOrNil passes nil through, else maps the error via FromDB. Handy for
// :exec queries where a nil error means success.
func FromDBOrNil(err error) error {
	if err == nil {
		return nil
	}
	return FromDB(err)
}

// FromDB maps common pgx/pg errors to typed app errors.
func FromDB(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return NotFound("Record not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return Conflict("Duplicate record")
	}
	return Internal(err.Error())
}

// CustomErrorHandler is echo's centralized error responder.
func CustomErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	status, msg := http.StatusInternalServerError, "internal server error"

	var appErr *Error
	var httpErr *echo.HTTPError
	switch {
	case errors.As(err, &appErr):
		status, msg = appErr.Status(), appErr.Error()
	case errors.As(err, &httpErr):
		status = httpErr.Code
		if m, ok := httpErr.Message.(string); ok {
			msg = m
		}
	}

	zap.L().Error(err.Error(),
		zap.Int("status", status),
		zap.String("method", c.Request().Method),
		zap.String("uri", c.Request().URL.RequestURI()))

	_ = c.JSON(status, FailedResponse{Success: false, Message: msg})
}
