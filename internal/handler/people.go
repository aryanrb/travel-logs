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

type PeopleHandler struct {
	svc *service.SocialService
	log *zap.Logger
}

func NewPeopleHandler() *PeopleHandler {
	return &PeopleHandler{svc: service.NewSocialService(), log: zap.L().Named("PeopleHandler")}
}

// Profile is auth-optional: relationship fields are included when signed in.
func (h *PeopleHandler) Profile(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.svc.Profile(c.Request().Context(), id, auth.OptionalUserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *PeopleHandler) Follow(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Follow(c.Request().Context(), auth.UserID(c), id); err != nil {
		return err
	}
	return responder.JSONResponse(c, "following", http.StatusOK)
}

func (h *PeopleHandler) Unfollow(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Unfollow(c.Request().Context(), auth.UserID(c), id); err != nil {
		return err
	}
	return responder.JSONResponse(c, "unfollowed", http.StatusOK)
}

func (h *PeopleHandler) Friend(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Friend(c.Request().Context(), auth.UserID(c), id); err != nil {
		return err
	}
	return responder.JSONResponse(c, "ok", http.StatusOK)
}

func (h *PeopleHandler) Unfriend(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Unfriend(c.Request().Context(), auth.UserID(c), id); err != nil {
		return err
	}
	return responder.JSONResponse(c, "ok", http.StatusOK)
}

func (h *PeopleHandler) Friends(c echo.Context) error {
	res, err := h.svc.Friends(c.Request().Context(), auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *PeopleHandler) Block(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Block(c.Request().Context(), auth.UserID(c), id); err != nil {
		return err
	}
	return responder.JSONResponse(c, "blocked", http.StatusOK)
}

func (h *PeopleHandler) Unblock(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Unblock(c.Request().Context(), auth.UserID(c), id); err != nil {
		return err
	}
	return responder.JSONResponse(c, "unblocked", http.StatusOK)
}

func (h *PeopleHandler) Blocked(c echo.Context) error {
	res, err := h.svc.Blocked(c.Request().Context(), auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *PeopleHandler) Report(c echo.Context) error {
	var in types.ReportInput
	if err := bind(c, &in); err != nil {
		return err
	}
	if err := h.svc.Report(c.Request().Context(), auth.UserID(c), in); err != nil {
		return err
	}
	return responder.JSONResponse(c, "reported", http.StatusCreated)
}
