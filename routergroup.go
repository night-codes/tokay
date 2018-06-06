package tokay

import (
	"bytes"

	"github.com/valyala/fasthttp"
)

// RouterGroup represents a group of routes that share the same path prefix.
type RouterGroup struct {
	path     string
	engine   *Engine
	handlers []Handler
}

// newRouteGroup creates a new RouterGroup with the given path, engine, and handlers.
func newRouteGroup(path string, engine *Engine, handlers []Handler) *RouterGroup {
	return &RouterGroup{
		path:     path,
		engine:   engine,
		handlers: handlers,
	}
}

// Path returns RouterGroup fullpath
func (r *RouterGroup) Path() (path string) {
	return r.path
}

// GET adds a GET route to the engine with the given route path and handlers.
func (r *RouterGroup) GET(path string, handlers ...Handler) *Route {
	return newRoute(path, r).GET(handlers...)
}

// POST adds a POST route to the engine with the given route path and handlers.
func (r *RouterGroup) POST(path string, handlers ...Handler) *Route {
	return newRoute(path, r).POST(handlers...)
}

// PUT adds a PUT route to the engine with the given route path and handlers.
func (r *RouterGroup) PUT(path string, handlers ...Handler) *Route {
	return newRoute(path, r).PUT(handlers...)
}

// PATCH adds a PATCH route to the engine with the given route path and handlers.
func (r *RouterGroup) PATCH(path string, handlers ...Handler) *Route {
	return newRoute(path, r).PATCH(handlers...)
}

// DELETE adds a DELETE route to the engine with the given route path and handlers.
func (r *RouterGroup) DELETE(path string, handlers ...Handler) *Route {
	return newRoute(path, r).DELETE(handlers...)
}

// CONNECT adds a CONNECT route to the engine with the given route path and handlers.
func (r *RouterGroup) CONNECT(path string, handlers ...Handler) *Route {
	return newRoute(path, r).CONNECT(handlers...)
}

// HEAD adds a HEAD route to the engine with the given route path and handlers.
func (r *RouterGroup) HEAD(path string, handlers ...Handler) *Route {
	return newRoute(path, r).HEAD(handlers...)
}

// OPTIONS adds an OPTIONS route to the engine with the given route path and handlers.
func (r *RouterGroup) OPTIONS(path string, handlers ...Handler) *Route {
	return newRoute(path, r).OPTIONS(handlers...)
}

// TRACE adds a TRACE route to the engine with the given route path and handlers.
func (r *RouterGroup) TRACE(path string, handlers ...Handler) *Route {
	return newRoute(path, r).TRACE(handlers...)
}

// Any adds a route with the given route, handlers, and the HTTP methods as listed in routing.Methods.
func (r *RouterGroup) Any(path string, handlers ...Handler) *Route {
	route := newRoute(path, r)
	for _, method := range Methods {
		route.add(method, handlers)
	}
	return route
}

// To adds a route to the engine with the given HTTP methods, route path, and handlers.
// Multiple HTTP methods should be separated by commas (without any surrounding spaces).
func (r *RouterGroup) To(methods, path string, handlers ...Handler) *Route {
	return newRoute(path, r).To(methods, handlers...)
}

// Group creates a RouterGroup with the given route path and handlers.
// The new group will combine the existing path with the new one.
// If no handler is provided, the new group will inherit the handlers registered
// with the current group.
func (r *RouterGroup) Group(path string, handlers ...Handler) *RouterGroup {
	if len(handlers) == 0 {
		handlers = make([]Handler, len(r.handlers))
		copy(handlers, r.handlers)
	}
	if path == "" || path[0] != '/' {
		path = "/" + path
	}
	return newRouteGroup(r.path+path, r.engine, handlers)
}

// Use registers one or multiple handlers to the current route group.
// These handlers will be shared by all routes belong to this group and its subgroups.
func (r *RouterGroup) Use(handlers ...Handler) {
	r.handlers = append(r.handlers, handlers...)
}

// Static serves files from the given file system root.
// Where:
// 'path' - relative path from current engine path on site (must be without trailing slash),
// 'root' - directory that contains served files. For example:
//     engine.Static("/static", "/var/www")
func (r *RouterGroup) Static(path, root string, compress ...bool) *Route {
	if len(compress) == 0 {
		compress = append(compress, true)
	}
	if path == "" || path[len(path)-1] != '/' {
		path += "/"
	}

	group := r.Group(path)
	handler := (&fasthttp.FS{
		Root:     root,
		Compress: compress[0],
		PathRewrite: func(ctx *fasthttp.RequestCtx) []byte {
			return append([]byte{'/'}, bytes.TrimPrefix(ctx.Request.RequestURI(), []byte(group.path))...)
		},
	}).NewRequestHandler()

	return newRoute("*", group).To("GET,HEAD", func(c *Context) {
		handler(c.RequestCtx)
	})
}
