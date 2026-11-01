package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"travel-service/internal/auth"
	service "travel-service/internal/services"
	"travel-service/internal/types"
	"travel-service/internal/utils/responder"
)

type NotificationHandler struct {
	svc *service.NotificationService
	log *zap.Logger
}

func NewNotificationHandler() *NotificationHandler {
	return &NotificationHandler{svc: service.NewNotificationService(), log: zap.L().Named("NotificationHandler")}
}

func (h *NotificationHandler) List(c echo.Context) error {
	res, err := h.svc.List(c.Request().Context(), auth.UserID(c), c.QueryParam("cursor"))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *NotificationHandler) MarkRead(c echo.Context) error {
	var in types.MarkReadInput
	if err := bind(c, &in); err != nil {
		return err
	}
	if err := h.svc.MarkRead(c.Request().Context(), auth.UserID(c), in.IDs); err != nil {
		return err
	}
	return responder.JSONResponse(c, "ok", http.StatusOK)
}
