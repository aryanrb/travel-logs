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

type ExpenseHandler struct {
	svc *service.ExpenseService
	log *zap.Logger
}

func NewExpenseHandler() *ExpenseHandler {
	return &ExpenseHandler{svc: service.NewExpenseService(), log: zap.L().Named("ExpenseHandler")}
}

func (h *ExpenseHandler) Create(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in types.ExpenseInput
	if err := bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.Create(c.Request().Context(), tripID, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusCreated)
}

func (h *ExpenseHandler) List(c echo.Context) error {
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

func (h *ExpenseHandler) Get(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	eid, err := pathID(c, "eid")
	if err != nil {
		return err
	}
	res, err := h.svc.Get(c.Request().Context(), tripID, eid, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *ExpenseHandler) Update(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	eid, err := pathID(c, "eid")
	if err != nil {
		return err
	}
	var in types.ExpenseInput
	if err := bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.Update(c.Request().Context(), tripID, eid, auth.UserID(c), in)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, res, http.StatusOK)
}

func (h *ExpenseHandler) Delete(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	eid, err := pathID(c, "eid")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), tripID, eid, auth.UserID(c)); err != nil {
		return err
	}
	return responder.JSONResponse(c, "deleted", http.StatusOK)
}

func (h *ExpenseHandler) ListExternalPayers(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	rows, err := h.svc.ListExternalPayers(c.Request().Context(), tripID, auth.UserID(c))
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, rows, http.StatusOK)
}

func (h *ExpenseHandler) CreateExternalPayer(c echo.Context) error {
	tripID, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in struct {
		Name    string  `json:"name"`
		Contact *string `json:"contact"`
	}
	if err := bind(c, &in); err != nil {
		return err
	}
	p, err := h.svc.CreateExternalPayer(c.Request().Context(), tripID, auth.UserID(c), in.Name, in.Contact)
	if err != nil {
		return err
	}
	return responder.JSONResponse(c, p, http.StatusCreated)
}
