package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"travel-service/internal/auth"
	service "travel-service/internal/services"
	"travel-service/internal/utils/responder"
)

type SummaryHandler struct {
	svc *service.SummaryService
	log *zap.Logger
}

func NewSummaryHandler() *SummaryHandler {
	return &SummaryHandler{svc: service.NewSummaryService(), log: zap.L().Named("SummaryHandler")}
}

func (h *SummaryHandler) Get(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.svc.Get(c.Request().Context(), tripID, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}
