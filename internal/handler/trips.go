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

type TripHandler struct {
	svc *service.TripService
	log *zap.Logger
}

func NewTripHandler() *TripHandler {
	return &TripHandler{svc: service.NewTripService(), log: zap.L().Named("TripHandler")}
}

func (h *TripHandler) Create(c echo.Context) error {
	var in types.TripInput
	if err := bind(c, &in); err != nil {
		return err
	}
	trip, err := h.svc.Create(c.Request().Context(), auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, trip, http.StatusCreated)
}

func (h *TripHandler) List(c echo.Context) error {
	res, err := h.svc.List(c.Request().Context(), auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *TripHandler) Get(c echo.Context) error {
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

func (h *TripHandler) Update(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.TripInput
	if err := bind(c, &in); err != nil {
		return err
	}
	trip, err := h.svc.Update(c.Request().Context(), id, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, trip, http.StatusOK)
}

func (h *TripHandler) SetBudget(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.BudgetInput
	if err := bind(c, &in); err != nil {
		return err
	}
	trip, err := h.svc.SetBudget(c.Request().Context(), id, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, trip, http.StatusOK)
}

func (h *TripHandler) Delete(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), id, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "deleted", http.StatusOK)
}

func (h *TripHandler) SetPublic(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.SetPublicInput
	if err := bind(c, &in); err != nil {
		return err
	}
	if err := h.svc.SetPublic(c.Request().Context(), id, auth.UserID(c), in.IsPublic, in.ShowCostsPublic); err != nil {
		return err
	}
	return responder.JSONResponse(c, "updated", http.StatusOK)
}

func (h *TripHandler) Archive(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Archive(c.Request().Context(), id, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "archived", http.StatusOK)
}

func (h *TripHandler) AddDestination(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.DestinationInput
	if err := bind(c, &in); err != nil {
		return err
	}
	d, err := h.svc.AddDestination(c.Request().Context(), id, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, d, http.StatusCreated)
}

func (h *TripHandler) DeleteDestination(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	destID, err := pathID(c, "destID")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteDestination(c.Request().Context(), id, destID, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "deleted", http.StatusOK)
}
