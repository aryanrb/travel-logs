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

type CommentHandler struct {
	svc *service.CommentService
	log *zap.Logger
}

func NewCommentHandler() *CommentHandler {
	return &CommentHandler{svc: service.NewCommentService(), log: zap.L().Named("CommentHandler")}
}

// List is auth-optional: a signed-in viewer's blocks are applied.
func (h *CommentHandler) List(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.svc.List(c.Request().Context(), id, c.QueryParam("cursor"), auth.OptionalUserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *CommentHandler) Create(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.CommentInput
	if err := bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.Create(c.Request().Context(), id, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusCreated)
}

func (h *CommentHandler) Update(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.UpdateCommentInput
	if err := bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.Update(c.Request().Context(), id, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *CommentHandler) Delete(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), id, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "deleted", http.StatusOK)
}
