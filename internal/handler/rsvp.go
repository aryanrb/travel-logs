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

type RSVPHandler struct {
	svc *service.RSVPService
	log *zap.Logger
}

func NewRSVPHandler() *RSVPHandler {
	return &RSVPHandler{svc: service.NewRSVPService(), log: zap.L().Named("RSVPHandler")}
}

// ─── Organiser (owner only) ──────────────────────────────────────────────────

func (h *RSVPHandler) GetLink(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.svc.GetLink(c.Request().Context(), id, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *RSVPHandler) UpsertLink(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.InviteLinkInput
	if err := bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.UpsertLink(c.Request().Context(), id, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *RSVPHandler) ResetLink(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.svc.ResetLink(c.Request().Context(), id, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *RSVPHandler) ListRSVPs(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.svc.ListRSVPs(c.Request().Context(), id, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *RSVPHandler) Approve(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	rsvpID, err := pathID(c, "rsvpId")
	if err != nil {
		return err
	}
	if err := h.svc.Approve(c.Request().Context(), id, rsvpID, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "approved", http.StatusOK)
}

func (h *RSVPHandler) DeleteRSVP(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	rsvpID, err := pathID(c, "rsvpId")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteRSVP(c.Request().Context(), id, rsvpID, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "removed", http.StatusOK)
}

// ─── Invitee (/join/:token) ──────────────────────────────────────────────────

// Join is auth-optional: the visitor's my_rsvp is included when a token is sent.
func (h *RSVPHandler) Join(c echo.Context) error {
	res, err := h.svc.JoinInfo(c.Request().Context(), c.Param("token"), auth.OptionalUserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *RSVPHandler) Reply(c echo.Context) error {
	var in types.RSVPReplyInput
	if err := bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.Reply(c.Request().Context(), c.Param("token"), auth.UserID(c), in.Status)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}
