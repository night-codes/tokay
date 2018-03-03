package tokay

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"runtime"
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
		stores                storesMap
		maxParams             int
		notFound              []Handler
		notFoundHandlers      []Handler
		Debug                 bool
		RedirectTrailingSlash bool
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
		stores:                *newStoresMap(),
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
	ws := false
	start := time.Now()
	c := engine.pool.Get().(*Context)
	c.init(ctx)
	c.handlers, c.pnames, ws = engine.find(string(ctx.Method()), string(ctx.Path()), c.pvalues)
	fin := func() {
		c.Next()
		engine.pool.Put(c)
		engine.debug(fmt.Sprintf("%-21s | %d | %9v | %-7s %-25s ", time.Now().Format("2006/01/02 - 15:04:05"), c.Response.StatusCode(), time.Since(start), string(ctx.Method()), string(ctx.Path())))
	}
	if ws {
		c.Websocket(fin)
		return
	}
	fin()
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
	for _, h := range handlers {
		engine.debug(fmt.Sprintf("%-7s %-25s -->", method, path), runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name())
	}
	store := engine.stores.Get(method)
	if store == nil {
		store = newStore()
		engine.stores.Set(method, store)
	}
	if n := store.Add(path, handlers); n > engine.maxParams {
		engine.maxParams = n
	}
}

func (engine *Engine) find(method, path string, pvalues []string) (handlers []Handler, pnames []string, ws bool) {
	var hh interface{}
	if store := engine.stores.Get(method); store != nil {
		if hh, pnames = store.Get(path, pvalues); hh != nil {
			return hh.([]Handler), pnames, false
		}
	}
	if method == "GET" {
		if store := engine.stores.Get("WEBSOCKET"); store != nil {
			if hh, pnames = store.Get(path, pvalues); hh != nil {
				return hh.([]Handler), pnames, true
			}
		}
	}

	return engine.notFoundHandlers, pnames, false
}

func (engine *Engine) findAllowedMethods(path string) map[string]bool {
	methods := make(map[string]bool)
	pvalues := make([]string, engine.maxParams)
	engine.stores.Range(func(m string, store routeStore) {
		if handlers, _ := store.Get(path, pvalues); handlers != nil {
			methods[m] = true
		}
	})
	return methods
}

func (engine *Engine) debug(text ...interface{}) {
	if engine.Debug {
		Debug.Println(text...)
	}
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
		c.Redirect(statusCode, path+"/")
		return true
	}
	if path[len(path)-1] == '/' && hasdot {
		c.Redirect(statusCode, path[:len(path)-1])
		return true
	}

	return false
}
