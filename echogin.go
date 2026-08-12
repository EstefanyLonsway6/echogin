package echogin

import (
	"github.com/gin-gonic/gin"
	"github.com/labstack/echo/v4"
)

const middlewareExecutedKey = "echogin.middleware_executed"

func HandleContext(c *gin.Context, e *echo.Echo) {
	if c.GetBool(middlewareExecutedKey) {
		// Middleware already executed, proceed to handler
		return
	}

	// Mark middleware as executed
	c.Set(middlewareExecutedKey, true)

	// Existing logic to handle context
	e.ServeHTTP(c.Writer, c.Request)
}