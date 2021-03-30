package tokay

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	render "github.com/night-codes/tokay-render"
	"github.com/valyala/fasthttp"
)

type (
	// Render is interface for engine.Render
	Render interface {
		JSON(*fasthttp.RequestCtx, int, interface{}) error
		JSONP(*fasthttp.RequestCtx, int, string, interface{}) error
		HTML(*fasthttp.RequestCtx, int, string, interface{}, ...string) error
		XML(*fasthttp.RequestCtx, int, interface{}) error
		JS(*fasthttp.RequestCtx, int, string, interface{}, ...string) error
	}

	// Handler is the function for handling HTTP requests.
	Handler func(*Context)

	// Engine manages routes and dispatches HTTP requests to the handlers of the matching routes.
	Engine struct {
		RouterGroup
		// Default render engine
		Render Render
		// AppEngine usage marker
		AppEngine bool
		// Print debug messages to log
		Debug bool

		DebugFunc func(*Context, time.Duration)
		// fasthhtp server
		Server *fasthttp.Server

		// Enables automatic redirection if the current route can't be matched but a
		// handler for the path with the trailing slash exists.
		// For example if /foo is requested but a route only exists for /foo/, the
		// client is redirected to /foo/ with http status code 301 for GET requests
		// and 307 for all other request methods.
		RedirectTrailingSlash bool

		pool             sync.Pool
		routes           map[string]*Route
		stores           storesMap
		maxParams        int
		notFound         []Handler
		notFoundHandlers []Handler
	}

	// Config is a struct for specifying configuration options for the tokay.Engine object.
	Config struct {
		// Print debug messages to log
		Debug bool
		// DebugFunc is callback function that calls after context
		DebugFunc func(*Context, time.Duration)
		// Extensions to parse template files from. Defaults to [".html"].
		TemplatesExtensions []string
		// Directories to load templates. Default is ["templates"].
		TemplatesDirs []string
		// Left templates delimiter, defaults to {{.
		LeftTemplateDelimiter string
		// Right templates delimiter, defaults to }}.
		RightTemplateDelimiter string
		// Funcs is a slice of FuncMaps to apply to the template upon compilation. This is useful for helper functions. Defaults to [].
		TemplatesFuncs template.FuncMap
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
func New(config ...*Config) *Engine {
	var r *render.Render
	var cfgDebug bool
	var cfgDebugFunc func(*Context, time.Duration)
	rCfg := &render.Config{}
	if len(config) != 0 && config[0] != nil {
		if len(config[0].TemplatesDirs) != 0 {
			rCfg = &render.Config{
				Directories: config[0].TemplatesDirs,
				Extensions:  config[0].TemplatesExtensions,
				Delims: render.Delims{
					Left: config[0].LeftTemplateDelimiter,
				},
				Funcs: config[0].TemplatesFuncs,
			}
		}
		cfgDebug = config[0].Debug
		cfgDebugFunc = config[0].DebugFunc
	}
	r = render.New(rCfg)

	engine := &Engine{
		AppEngine:             AppEngine,
		routes:                make(map[string]*Route),
		stores:                *newStoresMap(),
		Render:                r,
		RedirectTrailingSlash: true,
		Debug:                 cfgDebug,
		DebugFunc:             cfgDebugFunc,
		Server:                &fasthttp.Server{},
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
// It is a shortcut for engine.Server.ListenAndServe(addr, engine.HandleRequest) Note: this method will block the
// calling goroutine indefinitely unless an error happens.
func (engine *Engine) Run(addr string, message ...string) error {
	ec := make(chan error)
	go func() {
		engine.Server.Handler = engine.HandleRequest
		ec <- engine.Server.ListenAndServe(addr)
	}()
	return runmsg(addr, ec, append(message, "HTTP server started at %s")[0])
}

// RunTLS attaches the engine to a fasthttp server and starts listening and
// serving HTTPS (secure) requests. It is a shortcut for
// engine.Server.ListenAndServeTLS(addr, certFile, keyFile)
// Note: this method will block the calling goroutine indefinitely unless an error happens.
func (engine *Engine) RunTLS(addr string, certFile, keyFile string, message ...string) error {
	ec := make(chan error)
	go func() {
		engine.Server.Handler = engine.HandleRequest
		ec <- engine.Server.ListenAndServeTLS(addr, certFile, keyFile)
	}()
	return runmsg(addr, ec, append(message, "HTTPS server started at %s")[0])
}

// RunUnix attaches the engine to a fasthttp server and starts listening and
// serving HTTP requests through the specified unix socket (ie. a file).
// Note: this method will block the calling goroutine indefinitely unless an error happens.
func (engine *Engine) RunUnix(addr string, mode os.FileMode, message ...string) error {
	ec := make(chan error)
	go func() {
		engine.Server.Handler = engine.HandleRequest
		ec <- engine.Server.ListenAndServeUNIX(addr, mode)
	}()
	return runmsg(addr, ec, append(message, "Unix server started at %s")[0])
}

// HandleRequest handles the HTTP request.
func (engine *Engine) HandleRequest(ctx *fasthttp.RequestCtx) {
	start := time.Now()
	c := engine.pool.Get().(*Context)
	c.init(ctx)
	c.handlers, c.pnames = engine.find(string(ctx.Method()), string(ctx.Path()), c.pvalues)
	fin := func() {
		c.Next()
		engine.pool.Put(c)
		engine.debug(fmt.Sprintf("%-21s | %d | %9v | %-7s %-25s ", time.Now().Format("2006/01/02 - 15:04:05"), c.Response.StatusCode(), time.Since(start), string(ctx.Method()), string(ctx.Path())))
		if engine.DebugFunc != nil {
			engine.DebugFunc(c, time.Since(start))
		}
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

func (engine *Engine) find(method, path string, pvalues []string) (handlers []Handler, pnames []string) {
	var hh interface{}
	if store := engine.stores.Get(method); store != nil {
		if hh, pnames = store.Get(path, pvalues); hh != nil {
			return hh.([]Handler), pnames
		}
	}

	return engine.notFoundHandlers, pnames
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
		debug.Println(text...)
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
	if c.GetHeader("Redirect-Trailing-Slash") != "" {
		return false
	}
	path := c.Path()
	statusCode := 301 // Permanent redirect, request with GET method
	if c.Method() != "GET" {
		statusCode = 307
	}

	if length := len(path); length > 1 && path[length-1] == '/' {
		path = path[:length-1]
	} else {
		path = path + "/"
	}

	methods := c.Engine().findAllowedMethods(path)
	if len(methods) == 0 {
		return false
	}
	c.Redirect(statusCode, path)
	return true
}
