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

type ParticipantHandler struct {
	svc *service.ParticipantService
	log *zap.Logger
}

func NewParticipantHandler() *ParticipantHandler {
	return &ParticipantHandler{svc: service.NewParticipantService(), log: zap.L().Named("ParticipantHandler")}
}

func (h *ParticipantHandler) Invite(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.InviteInput
	if err := bind(c, &in); err != nil {
		return err
	}
	p, err := h.svc.Invite(c.Request().Context(), tripID, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, p, http.StatusCreated)
}

func (h *ParticipantHandler) List(c echo.Context) error {
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

func (h *ParticipantHandler) ChangeRole(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	pid, err := pathID(c, "pid")
	if err != nil {
		return err
	}
	var in types.UpdateRoleInput
	if err := bind(c, &in); err != nil {
		return err
	}
	if err := h.svc.ChangeRole(c.Request().Context(), tripID, pid, auth.UserID(c), in.Role); err != nil {
		return err
	}
	return responder.JSONResponse(c, "updated", http.StatusOK)
}

func (h *ParticipantHandler) Remove(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	pid, err := pathID(c, "pid")
	if err != nil {
		return err
	}
	if err := h.svc.Remove(c.Request().Context(), tripID, pid, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "removed", http.StatusOK)
}

func (h *ParticipantHandler) Accept(c echo.Context) error {
	var in types.AcceptInviteInput
	if err := bind(c, &in); err != nil {
		return err
	}
	p, err := h.svc.Accept(c.Request().Context(), auth.UserID(c), in.Token)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, p, http.StatusOK)
}
