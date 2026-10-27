package responder

import "github.com/labstack/echo/v4"

type SuccessResponse struct {
	Success bool `json:"success"`
	Payload any  `json:"payload"`
}

// JSONResponse writes a standard success envelope.
func JSONResponse(c echo.Context, payload any, code int) error {
	return c.JSON(code, SuccessResponse{Success: true, Payload: payload})
}
