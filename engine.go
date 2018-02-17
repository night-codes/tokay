package tokay

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/night-codes/tokay-render"
	"github.com/valyala/fasthttp"
)

type (
	// Handler is the function for handling HTTP requests.
	Handler func(*Context)

	// Engine manages routes and dispatches HTTP requests to the handlers of the matching routes.
	Engine struct {
		RouterGroup
		Render                *render.Render
		AppEngine             bool
		pool                  sync.Pool
		routes                map[string]*Route
		stores                map[string]routeStore
		maxParams             int
		notFound              []Handler
		notFoundHandlers      []Handler
		RedirectTrailingSlash bool
	}

	// routeStore stores route paths and the corresponding handlers.
	routeStore interface {
		Add(key string, data interface{}) int
		Get(key string, pvalues []string) (data interface{}, pnames []string)
		String() string
	}
)

var (
	// AppEngine usage marker
	AppEngine bool

	// Methods lists all supported HTTP methods by Engine.
	Methods = []string{
		"HEAD",
		"GET",
		"POST",
		"CONNECT",
		"DELETE",
		"OPTIONS",
		"PATCH",
		"PUT",
		"TRACE",
	}
)

// New creates a new Engine object.
func New() *Engine {
	engine := &Engine{
		AppEngine:             AppEngine,
		routes:                make(map[string]*Route),
		stores:                make(map[string]routeStore),
		Render:                render.New(),
		RedirectTrailingSlash: true,
	}
	engine.RouterGroup = *newRouteGroup("", engine, make([]Handler, 0))
	engine.NotFound(MethodNotAllowedHandler, NotFoundHandler)
	engine.pool.New = func() interface{} {
		return &Context{
			pvalues: make([]string, engine.maxParams),
			engine:  engine,
		}
	}
	return engine
}

func runmsg(addr string, ec chan error, message string) (err error) {
	if message != "" {
		select {
		case err = <-ec:
			return
		case _ = <-time.Tick(time.Second / 4):
			if strings.Contains(message, "%s") {
				fmt.Printf(message+"\n", addr)
			} else {
				fmt.Println(message)
			}
		}
	}
	err = <-ec
	return
}

// Run attaches the engine to a fasthttp server and starts listening and serving HTTP requests.
// It is a shortcut for fasthttp.ListenAndServe(addr, engine.HandleRequest) Note: this method will block the
// calling goroutine indefinitely unless an error happens.
func (engine *Engine) Run(addr string, message ...string) error {
	ec := make(chan error)
	go func() {
		ec <- fasthttp.ListenAndServe(addr, engine.HandleRequest)
	}()
	return runmsg(addr, ec, append(message, "HTTP server started at %s")[0])
}

// RunTLS attaches the engine to a fasthttp server and starts listening and
// serving HTTPS (secure) requests. It is a shortcut for
// fasthttp.ListenAndServeTLS(addr, certFile, keyFile, engine.HandleRequest)
// Note: this method will block the calling goroutine indefinitely unless an error happens.
func (engine *Engine) RunTLS(addr string, certFile, keyFile string, message ...string) error {
	ec := make(chan error)
	go func() {
		ec <- fasthttp.ListenAndServeTLS(addr, certFile, keyFile, engine.HandleRequest)
	}()
	return runmsg(addr, ec, append(message, "HTTPS server started at %s")[0])
}

// RunUnix attaches the engine to a fasthttp server and starts listening and
// serving HTTP requests through the specified unix socket (ie. a file).
// Note: this method will block the calling goroutine indefinitely unless an error happens.
func (engine *Engine) RunUnix(addr string, mode os.FileMode, message ...string) error {
	ec := make(chan error)
	go func() {
		ec <- fasthttp.ListenAndServeUNIX(addr, mode, engine.HandleRequest)
	}()
	return runmsg(addr, ec, append(message, "Unix server started at %s")[0])
}

// HandleRequest handles the HTTP request.
func (engine *Engine) HandleRequest(ctx *fasthttp.RequestCtx) {
	c := engine.pool.Get().(*Context)
	c.init(ctx)
	c.handlers, c.pnames = engine.find(string(ctx.Method()), string(ctx.Path()), c.pvalues)
	c.Next()
	engine.pool.Put(c)
}

// Route returns the named route.
// Nil is returned if the named route cannot be found.
func (engine *Engine) Route(name string) *Route {
	return engine.routes[name]
}

// Use appends the specified handlers to the engine and shares them with all routes.
func (engine *Engine) Use(handlers ...Handler) {
	engine.RouterGroup.Use(handlers...)
	engine.notFoundHandlers = combineHandlers(engine.handlers, engine.notFound)
}

// NotFound specifies the handlers that should be invoked when the engine cannot find any route matching a request.
// Note that the handlers registered via Use will be invoked first in this case.
func (engine *Engine) NotFound(handlers ...Handler) {
	engine.notFound = handlers
	engine.notFoundHandlers = combineHandlers(engine.handlers, engine.notFound)
}

// handleError is the error handler for handling any unhandled errors.
func (engine *Engine) handleError(c *Context, err error) {
	c.Error(err.Error(), http.StatusInternalServerError)
}

func (engine *Engine) add(method, path string, handlers []Handler) {
	store := engine.stores[method]
	if store == nil {
		store = newStore()
		engine.stores[method] = store
	}
	if n := store.Add(path, handlers); n > engine.maxParams {
		engine.maxParams = n
	}
}

func (engine *Engine) find(method, path string, pvalues []string) (handlers []Handler, pnames []string) {
	var hh interface{}
	if store := engine.stores[method]; store != nil {
		hh, pnames = store.Get(path, pvalues)
	}
	if hh != nil {
		return hh.([]Handler), pnames
	}
	return engine.notFoundHandlers, pnames
}

func (engine *Engine) findAllowedMethods(path string) map[string]bool {
	methods := make(map[string]bool)
	pvalues := make([]string, engine.maxParams)
	for m, store := range engine.stores {
		if handlers, _ := store.Get(path, pvalues); handlers != nil {
			methods[m] = true
		}
	}
	return methods
}

// NotFoundHandler returns a 404 HTTP error indicating a request has no matching route.
func NotFoundHandler(c *Context) {
	if c.engine.RedirectTrailingSlash && redirectTrailingSlash(c) {
		return
	}
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

func redirectTrailingSlash(c *Context) bool {
	path := c.Path()
	fmt.Println(1, path)
	statusCode := 301 // Permanent redirect, request with GET method
	if c.Method() != "GET" {
		statusCode = 307
	}
	if len(path) == 0 {
		c.Redirect(statusCode, "/")
		return true
	}

	pathSpl := strings.Split(path, "/")
	d := 1
	if path[len(path)-1] == '/' && len(pathSpl) > 1 {
		d = 2
	}
	hasdot := strings.Index(pathSpl[len(pathSpl)-d], ".") != -1

	if path[len(path)-1] != '/' && !hasdot {
		path = path + "/"
		c.Redirect(statusCode, path)
		return true
	}
	if path[len(path)-1] == '/' && hasdot {
		path = path[:len(path)-1]
		c.Redirect(statusCode, path)
		return true
	}

	return false
}
