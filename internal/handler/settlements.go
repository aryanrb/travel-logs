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

type SettlementHandler struct {
	svc *service.SettlementService
	log *zap.Logger
}

func NewSettlementHandler() *SettlementHandler {
	return &SettlementHandler{svc: service.NewSettlementService(), log: zap.L().Named("SettlementHandler")}
}

func (h *SettlementHandler) Balances(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.svc.Balances(c.Request().Context(), tripID, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *SettlementHandler) List(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	rows, err := h.svc.List(c.Request().Context(), tripID, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, rows, http.StatusOK)
}

func (h *SettlementHandler) Create(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.SettlementInput
	if err := bind(c, &in); err != nil {
		return err
	}
	st, err := h.svc.Create(c.Request().Context(), tripID, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, st, http.StatusCreated)
}

func (h *SettlementHandler) Confirm(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	sid, err := pathID(c, "sid")
	if err != nil {
		return err
	}
	if err := h.svc.Confirm(c.Request().Context(), tripID, sid, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "confirmed", http.StatusOK)
}

func (h *SettlementHandler) Delete(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	sid, err := pathID(c, "sid")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), tripID, sid, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "deleted", http.StatusOK)
}
