package echogin

import (
	"bufio"
	"context"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/labstack/echo/v4"
)

type contextKey string

const middlewareGuardKey contextKey = "echogin.middleware_guard"

type middlewareGuard struct {
	mu       sync.Mutex
	executed bool
}

func (g *middlewareGuard) markExecuted() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.executed {
		return true
	}
	g.executed = true
	return false
}

type Adapter struct {
	engine *gin.Engine
	echo   *echo.Echo
}

func New(engine *gin.Engine) *Adapter {
	return &Adapter{engine: engine, echo: echo.New()}
}

func (a *Adapter) Wrap(handler echo.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		guard := getOrCreateGuard(c)
		if guard.markExecuted() {
			c.Next()
			return
		}
		handleEcho(a.echo, c, handler)
	}
}

func (a *Adapter) WrapMiddleware(mw echo.MiddlewareFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		guard := getOrCreateGuard(c)
		if guard.markExecuted() {
			c.Next()
			return
		}
		echoMw := mw(func(ec echo.Context) error {
			c.Next()
			return nil
		})
		adaptedCtx := newEchoContext(a.echo, c)
		_ = echoMw(adaptedCtx)
	}
}

func (a *Adapter) HandleContext(c *gin.Context) {
	guard := getOrCreateGuard(c)
	if guard.markExecuted() {
		c.Next()
		return
	}
	a.engine.HandleContext(c)
}

func getOrCreateGuard(c *gin.Context) *middlewareGuard {
	if v, exists := c.Get(string(middlewareGuardKey)); exists {
		if guard, ok := v.(*middlewareGuard); ok {
			return guard
		}
	}
	guard := &middlewareGuard{}
	c.Set(string(middlewareGuardKey), guard)
	return guard
}

func handleEcho(e *echo.Echo, c *gin.Context, handler echo.HandlerFunc) {
	ec := newEchoContext(e, c)
	if err := handler(ec); err != nil {
		_ = c.Error(err)
	}
}

type echoContext struct {
	echo       *echo.Echo
	ginCtx     *gin.Context
	req        *http.Request
	resp       *echoResponse
	echoCtxVal context.Context
	path       string
	paramNames []string
	paramVals  []string
	handler    echo.HandlerFunc
	logger     echo.Logger
}

type echoResponse struct {
	ginCtx    *gin.Context
	committed bool
	status    int
	size      int64
}

func newEchoContext(e *echo.Echo, c *gin.Context) *echoContext {
	return &echoContext{
		echo:   e,
		ginCtx: c,
		req:    c.Request,
		resp: &echoResponse{
			ginCtx: c,
			status: http.StatusOK,
		},
		echoCtxVal: c.Request.Context(),
		path:       c.FullPath(),
	}
}

// --- echo.Context implementation ---
func (e *echoContext) Request() *http.Request                { return e.req }
func (e *echoContext) SetRequest(r *http.Request)             { e.req = r }
func (e *echoContext) SetResponse(r *echo.Response)           {}
func (e *echoContext) Response() *echo.Response               { return echo.NewResponse(e.resp, e.echo) }
func (e *echoContext) IsTLS() bool                            { return e.req.TLS != nil }
func (e *echoContext) IsWebSocket() bool                      { return false }
func (e *echoContext) Scheme() string                         { if e.req.TLS != nil { return "https" }; return "http" }
func (e *echoContext) RealIP() string                         { return e.ginCtx.ClientIP() }
func (e *echoContext) Path() string                           { return e.path }
func (e *echoContext) SetPath(p string)                       { e.path = p }
func (e *echoContext) Param(name string) string               { return e.ginCtx.Param(name) }
func (e *echoContext) ParamNames() []string                   { return e.paramNames }
func (e *echoContext) SetParamNames(names ...string)          { e.paramNames = names }
func (e *echoContext) ParamValues() []string                  { return e.paramVals }
func (e *echoContext) SetParamValues(values ...string)        { e.paramVals = values }
func (e *echoContext) QueryParam(name string) string          { return e.ginCtx.Query(name) }
func (e *echoContext) QueryParams() url.Values                { return e.req.URL.Query() }
func (e *echoContext) QueryString() string                    { return e.req.URL.RawQuery }
func (e *echoContext) FormValue(name string) string           { return e.ginCtx.PostForm(name) }
func (e *echoContext) FormParams() (url.Values, error)        { return e.req.PostForm, nil }
func (e *echoContext) FormFile(name string) (*multipart.FileHeader, error) { return e.ginCtx.FormFile(name) }
func (e *echoContext) MultipartForm() (*multipart.Form, error)          { return e.ginCtx.MultipartForm() }
func (e *echoContext) Cookie(name string) (*http.Cookie, error)         { return e.req.Cookie(name) }
func (e *echoContext) SetCookie(cookie *http.Cookie)                    { http.SetCookie(e.ginCtx.Writer, cookie) }
func (e *echoContext) Cookies() []*http.Cookie                          { return e.req.Cookies() }
func (e *echoContext) Get(key string) interface{}                       { v, _ := e.ginCtx.Get(key); return v }
func (e *echoContext) Set(key string, val interface{})                  { e.ginCtx.Set(key, val) }
func (e *echoContext) Bind(i interface{}) error                         { return e.ginCtx.ShouldBind(i) }
func (e *echoContext) Validate(i interface{}) error                     { return e.ginCtx.ShouldBind(i) }
func (e *echoContext) Render(code int, name string, data interface{}) error { return nil }
func (e *echoContext) HTML(code int, html string) error                 { e.ginCtx.Data(code, "text/html; charset=utf-8", []byte(html)); return nil }
func (e *echoContext) HTMLBlob(code int, b []byte) error                { e.ginCtx.Data(code, "text/html; charset=utf-8", b); return nil }
func (e *echoContext) String(code int, s string) error                  { e.ginCtx.String(code, s); return nil }
func (e *echoContext) JSON(code int, i interface{}) error               { e.ginCtx.JSON(code, i); return nil }
func (e *echoContext) JSONPretty(code int, i interface{}, indent string) error { return e.JSON(code, i) }
func (e *echoContext) JSONBlob(code int, b []byte) error                { e.ginCtx.Data(code, "application/json", b); return nil }
func (e *echoContext) JSONP(code int, callback string, i interface{}) error { e.ginCtx.JSONP(code, i); return nil }
func (e *echoContext) JSONPBlob(code int, callback string, b []byte) error { return nil }
func (e *echoContext) XML(code int, i interface{}) error                { e.ginCtx.XML(code, i); return nil }
func (e *echoContext) XMLPretty(code int, i interface{}, indent string) error { return e.XML(code, i) }
func (e *echoContext) XMLBlob(code int, b []byte) error                 { return e.XML(code, b) }
func (e *echoContext) Blob(code int, contentType string, b []byte) error { e.ginCtx.Data(code, contentType, b); return nil }
func (e *echoContext) Stream(code int, contentType string, r io.Reader) error { return nil }
func (e *echoContext) File(file string) error                           { e.ginCtx.File(file); return nil }
func (e *echoContext) Attachment(file string, name string) error        { e.ginCtx.FileAttachment(file, name); return nil }
func (e *echoContext) Inline(file string, name string) error            { return e.File(file) }
func (e *echoContext) NoContent(code int) error                         { e.ginCtx.Status(code); return nil }
func (e *echoContext) Redirect(code int, url string) error              { e.ginCtx.Redirect(code, url); return nil }
func (e *echoContext) Error(err error)                                  { _ = e.ginCtx.Error(err) }
func (e *echoContext) Handler() echo.HandlerFunc                        { return e.handler }
func (e *echoContext) SetHandler(h echo.HandlerFunc)                    { e.handler = h }
func (e *echoContext) Logger() echo.Logger                              {
	if e.logger != nil { return e.logger }
	return e.echo.Logger
}
func (e *echoContext) SetLogger(l echo.Logger)                          { e.logger = l }
func (e *echoContext) Echo() *echo.Echo                                 { return e.echo }
func (e *echoContext) Reset(r *http.Request, w http.ResponseWriter)     {}

// --- echoResponse ---
func (er *echoResponse) WriteHeader(code int) {
	if er.committed { return }
	er.status = code
	er.ginCtx.Writer.WriteHeader(code)
	er.committed = true
}
func (er *echoResponse) Write(b []byte) (int, error) {
	n, err := er.ginCtx.Writer.Write(b)
	er.size += int64(n)
	return n, err
}
func (er *echoResponse) Header() http.Header                    { return er.ginCtx.Writer.Header() }
func (er *echoResponse) WriteHeaderNow()                        {}
func (er *echoResponse) Flush()                                 { er.ginCtx.Writer.Flush() }
func (er *echoResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, nil }
func (er *echoResponse) Unwrap() http.ResponseWriter            { return er.ginCtx.Writer }
func (er *echoResponse) Status() int                            { return er.status }
func (er *echoResponse) Size() int64                            { return er.size }
func (er *echoResponse) Committed() bool                        { return er.committed }
func (er *echoResponse) Before(fn func())                       {}
func (er *echoResponse) After(fn func())                        {}
