package handlers

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"travel-service/internal/auth"
	service "travel-service/internal/services"
	"travel-service/internal/utils/apperror"
	"travel-service/internal/utils/responder"
)

type EventHandler struct {
	svc *service.EventService
	log *zap.Logger
}

func NewEventHandler() *EventHandler {
	return &EventHandler{svc: service.NewEventService(), log: zap.L().Named("EventHandler")}
}

// Activity returns a trip's newest events, paged by the optional `before` cursor.
func (h *EventHandler) Activity(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var before *time.Time
	if raw := c.QueryParam("before"); raw != "" {
		t, perr := time.Parse(time.RFC3339, raw)
		if perr != nil {
			return apperror.BadRequest("before must be an RFC3339 timestamp")
		}
		before = &t
	}
	res, err := h.svc.List(c.Request().Context(), tripID, auth.UserID(c), before)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}
