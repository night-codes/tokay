package tokay

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/night-codes/tokay-render"
	"github.com/valyala/fasthttp"
)

type (
	// Handler is the function for handling HTTP requests.
	Handler func(*Context)

	// Engine manages routes and dispatches HTTP requests to the handlers of the matching routes.
	Engine struct {
		RouterGroup
		Render           *render.Render
		pool             sync.Pool
		routes           map[string]*Route
		stores           map[string]routeStore
		maxParams        int
		notFound         []Handler
		notFoundHandlers []Handler
	}

	// routeStore stores route paths and the corresponding handlers.
	routeStore interface {
		Add(key string, data interface{}) int
		Get(key string, pvalues []string) (data interface{}, pnames []string)
		String() string
	}
)

// Methods lists all supported HTTP methods by Engine.
var Methods = []string{
	"CONNECT",
	"DELETE",
	"GET",
	"HEAD",
	"OPTIONS",
	"PATCH",
	"POST",
	"PUT",
	"TRACE",
}

// New creates a new Engine object.
func New() *Engine {
	r := &Engine{
		routes: make(map[string]*Route),
		stores: make(map[string]routeStore),
	}
	r.Render = render.New()
	r.RouterGroup = *newRouteGroup("", r, make([]Handler, 0))
	r.NotFound(MethodNotAllowedHandler, NotFoundHandler)
	r.pool.New = func() interface{} {
		return &Context{
			pvalues: make([]string, r.maxParams),
			engine:  r,
		}
	}
	return r
}

// Run attaches the engine to a fasthttp server and starts listening and serving HTTP requests.
// It is a shortcut for fasthttp.ListenAndServe(addr, engine.HandleRequest) Note: this method will block the
// calling goroutine indefinitely unless an error happens.
func (r *Engine) Run(addr string) (err error) {
	return fasthttp.ListenAndServe(addr, r.HandleRequest)
}

// RunTLS attaches the engine to a fasthttp server and starts listening and
// serving HTTPS (secure) requests. It is a shortcut for
// fasthttp.ListenAndServeTLS(addr, certFile, keyFile, engine.HandleRequest)
// Note: this method will block the calling goroutine indefinitely unless an error happens.
func (r *Engine) RunTLS(addr string, certFile, keyFile string) (err error) {
	return fasthttp.ListenAndServeTLS(addr, certFile, keyFile, r.HandleRequest)
}

// RunUnix attaches the engine to a fasthttp server and starts listening and
// serving HTTP requests through the specified unix socket (ie. a file).
// Note: this method will block the calling goroutine indefinitely unless an error happens.
func (r *Engine) RunUnix(addr string, mode os.FileMode) (err error) {
	return fasthttp.ListenAndServeUNIX(addr, mode, r.HandleRequest)
}

// HandleRequest handles the HTTP request.
func (r *Engine) HandleRequest(ctx *fasthttp.RequestCtx) {
	c := r.pool.Get().(*Context)
	c.init(ctx)
	c.handlers, c.pnames = r.find(string(ctx.Method()), string(ctx.Path()), c.pvalues)
	c.Next()
	r.pool.Put(c)
}

// Route returns the named route.
// Nil is returned if the named route cannot be found.
func (r *Engine) Route(name string) *Route {
	return r.routes[name]
}

// Use appends the specified handlers to the engine and shares them with all routes.
func (r *Engine) Use(handlers ...Handler) {
	r.RouterGroup.Use(handlers...)
	r.notFoundHandlers = combineHandlers(r.handlers, r.notFound)
}

// NotFound specifies the handlers that should be invoked when the engine cannot find any route matching a request.
// Note that the handlers registered via Use will be invoked first in this case.
func (r *Engine) NotFound(handlers ...Handler) {
	r.notFound = handlers
	r.notFoundHandlers = combineHandlers(r.handlers, r.notFound)
}

// handleError is the error handler for handling any unhandled errors.
func (r *Engine) handleError(c *Context, err error) {
	c.Error(err.Error(), http.StatusInternalServerError)
}

func (r *Engine) add(method, path string, handlers []Handler) {
	store := r.stores[method]
	if store == nil {
		store = newStore()
		r.stores[method] = store
	}
	if n := store.Add(path, handlers); n > r.maxParams {
		r.maxParams = n
	}
}

func (r *Engine) find(method, path string, pvalues []string) (handlers []Handler, pnames []string) {
	var hh interface{}
	if store := r.stores[method]; store != nil {
		hh, pnames = store.Get(path, pvalues)
	}
	if hh != nil {
		return hh.([]Handler), pnames
	}
	return r.notFoundHandlers, pnames
}

func (r *Engine) findAllowedMethods(path string) map[string]bool {
	methods := make(map[string]bool)
	pvalues := make([]string, r.maxParams)
	for m, store := range r.stores {
		if handlers, _ := store.Get(path, pvalues); handlers != nil {
			methods[m] = true
		}
	}
	return methods
}

// NotFoundHandler returns a 404 HTTP error indicating a request has no matching route.
func NotFoundHandler(c *Context) {
	c.String(http.StatusNotFound, http.StatusText(http.StatusNotFound))
}

// MethodNotAllowedHandler handles the situation when a request has matching route without matching HTTP method.
// In this case, the handler will respond with an Allow HTTP header listing the allowed HTTP methods.
// Otherwise, the handler will do nothing and let the next handler (usually a NotFoundHandler) to handle the problem.
func MethodNotAllowedHandler(c *Context) {
	methods := c.Engine().findAllowedMethods(string(c.Path()))
	if len(methods) == 0 {
		return
	}
	methods["OPTIONS"] = true
	ms := make([]string, len(methods))
	i := 0
	for method := range methods {
		ms[i] = method
		i++
	}
	sort.Strings(ms)
	c.Response.Header.Set("Allow", strings.Join(ms, ", "))
	if string(c.Method()) != "OPTIONS" {
		c.Response.SetStatusCode(http.StatusMethodNotAllowed)
	}
	c.Abort()
	return
}
