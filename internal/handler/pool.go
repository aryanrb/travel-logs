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

type PoolHandler struct {
	svc *service.PoolService
	log *zap.Logger
}

func NewPoolHandler() *PoolHandler {
	return &PoolHandler{svc: service.NewPoolService(), log: zap.L().Named("PoolHandler")}
}

func (h *PoolHandler) Get(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.svc.Get(c.Request().Context(), id, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *PoolHandler) Set(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.PoolInput
	if err := bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.Set(c.Request().Context(), id, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *PoolHandler) Delete(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), id, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "disabled", http.StatusOK)
}
