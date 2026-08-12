package echogin

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/labstack/echo/v4"
)

func TestPreventDuplicateMiddlewareExecution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	adapter := New(engine)

	var middlewareCount int32

	engine.Use(func(c *gin.Context) {
		atomic.AddInt32(&middlewareCount, 1)
		c.Next()
	})

	engine.GET("/test", adapter.Wrap(func(ec echo.Context) error {
		// Get the underlying gin.Context via Get/Set bridge and trigger re-entry
		ginCtxValue := ec.Get("__gin_ctx__")
		if ginCtx, ok := ginCtxValue.(*gin.Context); ok {
			adapter.HandleContext(ginCtx)
		}
		return nil
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if middlewareCount != 1 {
		t.Errorf("middleware executed %d times, expected exactly 1", middlewareCount)
	}
}

func TestMiddlewareGuardMarkExecuted(t *testing.T) {
	guard := &middlewareGuard{}

	if guard.markExecuted() {
		t.Error("first markExecuted() should return false")
	}
	if !guard.markExecuted() {
		t.Error("second markExecuted() should return true (already executed)")
	}
}

func TestGetOrCreateGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	guard1 := getOrCreateGuard(c)
	if guard1 == nil {
		t.Fatal("guard should not be nil")
	}

	guard2 := getOrCreateGuard(c)
	if guard1 != guard2 {
		t.Error("guard should return the same instance on subsequent calls")
	}
}

func TestWrapMiddlewareGuardOnReEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	adapter := New(engine)

	var execCount int32

	engine.Use(adapter.WrapMiddleware(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			atomic.AddInt32(&execCount, 1)
			return next(c)
		}
	}))

	engine.GET("/reentry", func(c *gin.Context) {
		adapter.HandleContext(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/reentry", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if execCount != 1 {
		t.Errorf("middleware executed %d times, expected exactly 1 (guard prevented re-entry)", execCount)
	}
}
