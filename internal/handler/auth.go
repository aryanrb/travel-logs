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

type AuthHandler struct {
	svc *service.UserService
	log *zap.Logger
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{svc: service.NewUserService(), log: zap.L().Named("AuthHandler")}
}

func (h *AuthHandler) Register(c echo.Context) error {
	var in types.RegisterInput
	if err := bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.Register(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusCreated)
}

func (h *AuthHandler) Login(c echo.Context) error {
	var in types.LoginInput
	if err := bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.Login(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *AuthHandler) Me(c echo.Context) error {
	res, err := h.svc.Me(c.Request().Context(), auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *AuthHandler) UpdateProfile(c echo.Context) error {
	var in types.UpdateProfileInput
	if err := bind(c, &in); err != nil {
		return err
	}
	if err := h.svc.UpdateProfile(c.Request().Context(), auth.UserID(c), in); err != nil {
		return err
	}
	return responder.JSONResponse(c, "updated", http.StatusOK)
}
