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

type TimelineHandler struct {
	svc *service.TimelineService
	log *zap.Logger
}

func NewTimelineHandler() *TimelineHandler {
	return &TimelineHandler{svc: service.NewTimelineService(), log: zap.L().Named("TimelineHandler")}
}

func (h *TimelineHandler) CreateEntry(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.EntryInput
	if err := bind(c, &in); err != nil {
		return err
	}
	e, err := h.svc.CreateEntry(c.Request().Context(), tripID, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, e, http.StatusCreated)
}

func (h *TimelineHandler) ListEntries(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	rows, err := h.svc.ListEntries(c.Request().Context(), tripID, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, rows, http.StatusOK)
}

func (h *TimelineHandler) UpdateEntry(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	eid, err := pathID(c, "eid")
	if err != nil {
		return err
	}
	var in types.EntryInput
	if err := bind(c, &in); err != nil {
		return err
	}
	if err := h.svc.UpdateEntry(c.Request().Context(), tripID, eid, auth.UserID(c), in); err != nil {
		return err
	}
	return responder.JSONResponse(c, "updated", http.StatusOK)
}

func (h *TimelineHandler) DeleteEntry(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	eid, err := pathID(c, "eid")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteEntry(c.Request().Context(), tripID, eid, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "deleted", http.StatusOK)
}

func (h *TimelineHandler) AddPhoto(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.PhotoInput
	if err := bind(c, &in); err != nil {
		return err
	}
	p, err := h.svc.AddPhoto(c.Request().Context(), tripID, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, p, http.StatusCreated)
}

func (h *TimelineHandler) ListPhotos(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	rows, err := h.svc.ListPhotos(c.Request().Context(), tripID, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, rows, http.StatusOK)
}

func (h *TimelineHandler) DeletePhoto(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	pid, err := pathID(c, "pid")
	if err != nil {
		return err
	}
	if err := h.svc.DeletePhoto(c.Request().Context(), tripID, pid, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "deleted", http.StatusOK)
}
