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

type TransportHandler struct {
	svc *service.TransportService
	log *zap.Logger
}

func NewTransportHandler() *TransportHandler {
	return &TransportHandler{svc: service.NewTransportService(), log: zap.L().Named("TransportHandler")}
}

func (h *TransportHandler) Create(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.TransportInput
	if err := bind(c, &in); err != nil {
		return err
	}
	t, err := h.svc.Create(c.Request().Context(), tripID, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, t, http.StatusCreated)
}

func (h *TransportHandler) List(c echo.Context) error {
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

func (h *TransportHandler) Update(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	tid, err := pathID(c, "tid")
	if err != nil {
		return err
	}
	var in types.TransportInput
	if err := bind(c, &in); err != nil {
		return err
	}
	if err := h.svc.Update(c.Request().Context(), tripID, tid, auth.UserID(c), in); err != nil {
		return err
	}
	return responder.JSONResponse(c, "updated", http.StatusOK)
}

func (h *TransportHandler) Delete(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	tid, err := pathID(c, "tid")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), tripID, tid, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "deleted", http.StatusOK)
}
