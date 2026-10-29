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

type PublicHandler struct {
	svc *service.PublicService
	log *zap.Logger
}

func NewPublicHandler() *PublicHandler {
	return &PublicHandler{svc: service.NewPublicService(), log: zap.L().Named("PublicHandler")}
}

// List and Get require no auth; any bearer token is simply ignored (these routes
// are not behind the auth middleware).

func (h *PublicHandler) List(c echo.Context) error {
	res, err := h.svc.ListTrips(c.Request().Context(), c.QueryParam("q"), c.QueryParam("trip_type"))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *PublicHandler) Get(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.svc.GetTrip(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

// Copy requires auth — the new trip is owned by the caller.
func (h *PublicHandler) Copy(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.CopyTripInput
	if err := bind(c, &in); err != nil {
		return err
	}
	trip, err := h.svc.Copy(c.Request().Context(), id, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, trip, http.StatusCreated)
}
