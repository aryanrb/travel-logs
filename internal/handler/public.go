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
	p := types.ExploreParams{
		Q:                c.QueryParam("q"),
		TripType:         c.QueryParam("trip_type"),
		Destination:      c.QueryParam("destination"),
		MinDays:          queryInt(c, "min_days"),
		MaxDays:          queryInt(c, "max_days"),
		MinCostPerPerson: queryInt64(c, "min_cost_per_person"),
		MaxCostPerPerson: queryInt64(c, "max_cost_per_person"),
		Currency:         c.QueryParam("currency"),
		Sort:             c.QueryParam("sort"),
		Following:        c.QueryParam("following") == "true",
		Limit:            intOr(c.QueryParam("limit"), 0),
		Cursor:           c.QueryParam("cursor"),
	}
	res, err := h.svc.ListTrips(c.Request().Context(), p, auth.OptionalUserID(c))
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
	res, err := h.svc.GetTrip(c.Request().Context(), id, auth.OptionalUserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *PublicHandler) Like(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Like(c.Request().Context(), id, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "liked", http.StatusOK)
}

func (h *PublicHandler) Unlike(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Unlike(c.Request().Context(), id, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "unliked", http.StatusOK)
}

// Save/Unsave/ListSaved require auth. Save and Unsave are idempotent and 404 when
// the trip isn't public.
func (h *PublicHandler) Save(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Save(c.Request().Context(), id, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "saved", http.StatusOK)
}

func (h *PublicHandler) Unsave(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Unsave(c.Request().Context(), id, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "removed", http.StatusOK)
}

func (h *PublicHandler) ListSaved(c echo.Context) error {
	res, err := h.svc.ListSaved(c.Request().Context(), auth.UserID(c))
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
