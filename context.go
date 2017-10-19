package tokay

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SerializeFunc serializes the given data of arbitrary type into a byte array.
type SerializeFunc func(data interface{}) ([]byte, error)

// Context represents the contextual data and environment while processing an incoming HTTP request.
type Context struct {
	*fasthttp.RequestCtx
	Serialize SerializeFunc // the function serializing the given data of arbitrary type into a byte array.

	engine   *Engine
	aborted  bool
	pnames   []string  // list of route parameter names
	pvalues  []string  // list of parameter values corresponding to pnames
	data     dataMap   // data items managed by Get and Set
	index    int       // the index of the currently executing handler in handlers
	handlers []Handler // the handlers associated with the current route
}

// Engine returns the Engine that is handling the incoming HTTP request.
func (c *Context) Engine() *Engine {
	return c.engine
}

// SetContentType sets response Content-Type.
func (c *Context) SetContentType(contentType string) {
	c.RequestCtx.SetContentType(contentType)
}

// SetStatusCode sets response status code.
func (c *Context) SetStatusCode(statusCode int) {
	c.RequestCtx.SetStatusCode(statusCode)
}

// SetCookie adds a Set-Cookie header to the ResponseWriter's headers.
// The provided cookie must have a valid Name.
// Paramethers `path` and `domain` can be empty strings
// Set expiration time to CookieExpireDelete for expiring (deleting) the cookie on the client.
// By default cookie lifetime is limited by browser session.
func (c *Context) SetCookie(name, value string, path, domain string, secure, httpOnly bool, expire ...time.Time) {
	if path == "" {
		path = "/"
	}

	cookie := fasthttp.AcquireCookie()
	cookie.SetKey(name)
	cookie.SetValue(url.QueryEscape(value))
	cookie.SetPath(path)
	cookie.SetSecure(secure)
	cookie.SetHTTPOnly(httpOnly)

	if len(expire) == 1 {
		cookie.SetExpire(expire[0])
	}

	if domain != "" {
		cookie.SetDomain(domain)
	}

	c.Response.Header.SetCookie(cookie)
}

// ClientIP returns the real client IP. It parses X-Real-IP and X-Forwarded-For in order to
// work properly with reverse-proxies such us: nginx or haproxy. Use X-Forwarded-For before
// X-Real-Ip as nginx uses X-Real-Ip with the proxy's IP.
func (c *Context) ClientIP() string {
	if c.engine.AppEngine {
		if addr := c.GetHeader("X-Appengine-Remote-Addr"); addr != "" {
			return addr
		}
	}

	clientIP := c.GetHeader("X-Forwarded-For")
	if index := strings.IndexByte(clientIP, ','); index >= 0 {
		clientIP = clientIP[0:index]
	}
	clientIP = strings.TrimSpace(clientIP)
	if len(clientIP) > 0 {
		return clientIP
	}
	clientIP = strings.TrimSpace(c.GetHeader("X-Real-Ip"))
	if len(clientIP) > 0 {
		return clientIP
	}

	if ip := c.RemoteIP().String(); len(ip) > 0 {
		return ip
	}

	return ""
}

// Redirect returns a HTTP redirect to the specific location.
func (c *Context) Redirect(statusCode int, uri string) {
	c.RequestCtx.Redirect(uri, statusCode)
}

// Param returns the named parameter value that is found in the URL path matching the current route.
// If the named parameter cannot be found, an empty string will be returned.
func (c *Context) Param(name string) string {
	for i, n := range c.pnames {
		if n == name {
			return c.pvalues[i]
		}
	}
	return ""
}

// Get returns the named data item previously registered with the context by calling Set.
// If the named data item cannot be found, nil will be returned.
func (c *Context) Get(name string) (value interface{}) {
	return c.data.Get(name)
}

// GetHeader returns value from request headers.
func (c *Context) GetHeader(key string) string {
	return string(c.Request.Header.Peek(key))
}

// Header is a intelligent shortcut for c.Response.Header.Set(key, value).
// It writes a header in the response. If value == "", this method removes the header
// `c.Response.Header.Del(key)`
func (c *Context) Header(key, value string) {
	if len(value) == 0 {
		c.Response.Header.Del(key)
	} else {
		c.Response.Header.Set(key, value)
	}
}

// GetEx returns the named data item and info about data item exists.
func (c *Context) GetEx(name string) (value interface{}, ok bool) {
	return c.data.GetEx(name)
}

// Set stores the named data item in the context so that it can be retrieved later.
func (c *Context) Set(name string, value interface{}) {
	c.data.Set(name, value)
}

// Unset the named data item in the context.
func (c *Context) Unset(name string) {
	c.data.Delete(name)
}

// Next calls the rest of the handlers associated with the current route.
// If any of these handlers returns an error, Next will return the error and skip the following handlers.
// Next is normally used when a handler needs to do some postprocessing after the rest of the handlers
// are executed.
func (c *Context) Next() {
	c.index++
	for n := len(c.handlers); c.index < n; c.index++ {
		c.handlers[c.index](c)
	}
}

// Abort skips the rest of the handlers associated with the current route.
// Abort is normally used when a handler handles the request normally and wants to skip the rest of the handlers.
// If a handler wants to indicate an error condition, it should simply return the error without calling Abort.
func (c *Context) Abort() {
	c.aborted = true
	c.index = len(c.handlers)
}

// AbortWithStatus calls `Abort()` and writes the headers with the specified status code.
// For example, a failed attempt to authenticate a request could use:
//     context.AbortWithStatus(401).
func (c *Context) AbortWithStatus(statusCode int) {
	c.SetStatusCode(statusCode)
	c.Abort()
}

// AbortWithError calls `AbortWithStatus()` and `Error()` internally.
func (c *Context) AbortWithError(statusCode int, err error) {
	if err != nil {
		c.Error(err.Error(), statusCode)
	} else {
		c.Error(http.StatusText(statusCode), statusCode)
	}
	c.Abort()
}

// IsAborted returns true if the current context was aborted.
func (c *Context) IsAborted() bool {
	return c.aborted
}

// URL creates a URL using the named route and the parameter values.
// The parameters should be given in the sequence of name1, value1, name2, value2, and so on.
// If a parameter in the route is not provided a value, the parameter token will remain in the resulting URL.
// Parameter values will be properly URL encoded.
// The method returns an empty string if the URL creation fails.
func (c *Context) URL(route string, pairs ...interface{}) string {
	if r := c.engine.routes[route]; r != nil {
		return r.URL(pairs...)
	}
	return ""
}

// WriteData writes the given data of arbitrary type to the response.
// The method calls the Serialize() method to convert the data into a byte array and then writes
// the byte array to the response.
func (c *Context) WriteData(data interface{}) (err error) {
	var bytes []byte
	if bytes, err = c.Serialize(data); err == nil {
		_, err = c.Write(bytes)
	}
	return
}

// init sets the request and response of the context and resets all other properties.
func (c *Context) init(ctx *fasthttp.RequestCtx) {
	c.RequestCtx = ctx
	c.data = dataMap{M: make(map[string]interface{})}
	c.index = -1
	c.Serialize = Serialize
}

// Cookie returns the named cookie provided in the request or
// ErrNoCookie if not found. And return the named cookie is unescaped.
// If multiple cookies match the given name, only one cookie will
// be returned.
func (c *Context) Cookie(name string) string {
	val, _ := url.QueryUnescape(string(c.Request.Header.Cookie(name)))
	return val
}

// Serialize converts the given data into a byte array.
// If the data is neither a byte array nor a string, it will call fmt.Sprint to convert it into a string.
func Serialize(data interface{}) (bytes []byte, err error) {
	switch data.(type) {
	case []byte:
		return data.([]byte), nil
	case string:
		return []byte(data.(string)), nil
	default:
		if data != nil {
			return []byte(fmt.Sprint(data)), nil
		}
	}
	return nil, nil
}

// JSON serializes the given struct as JSON into the response body.
// It also sets the Content-Type as "application/json".
func (c *Context) JSON(statusCode int, obj interface{}) {
	c.engine.Render.JSON(c.RequestCtx, statusCode, obj)
}

// JSONP marshals the given interface object and writes the JSON response.
func (c *Context) JSONP(statusCode int, callbackName string, obj interface{}) {
	c.engine.Render.JSONP(c.RequestCtx, statusCode, callbackName, obj)
}

// HTML renders the HTTP template specified by its file name.
// It also updates the HTTP code and sets the Content-Type as "text/html".
func (c *Context) HTML(statusCode int, name string, obj interface{}) {
	c.engine.Render.HTML(c.RequestCtx, statusCode, name, obj)
}

// XML serializes the given struct as XML into the response body.
// It also sets the Content-Type as "application/xml".
func (c *Context) XML(statusCode int, obj interface{}) {
	c.engine.Render.XML(c.RequestCtx, statusCode, obj)
}

// String writes the given string into the response body.
func (c *Context) String(statusCode int, format string, values ...interface{}) {
	c.SetStatusCode(statusCode)
	if len(values) > 0 {
		fmt.Fprintf(c, format, values[0])
	} else {
		fmt.Fprintf(c, format)
	}
}

// Data writes some data into the body stream and updates the HTTP code.
func (c *Context) Data(statusCode int, contentType string, data []byte) {
	c.SetStatusCode(statusCode)
	c.SetContentType(contentType)
	c.Write(data)
}

// Body returns request body
// The returned body is valid until the request modification.
func (c *Context) Body() []byte {
	return c.Request.Body()
}

// ContentType returns the Content-Type header of the request.
func (c *Context) ContentType() string {
	return filterFlags(c.GetHeader("Content-Type"))
}

// PostForm returns the specified key from a POST urlencoded form or
// multipart form when it exists, otherwise it returns an empty string "".
func (c *Context) PostForm(key string) string {
	return string(c.PostArgs().Peek(key))
}

// PostFormArray returns a slice of strings for a given form key. The length
// of the slice depends on the number of params with the given key.
func (c *Context) PostFormArray(key string) []string {
	var ret []string
	retBytes := c.PostArgs().PeekMulti(key)
	for k := range retBytes {
		ret = append(ret, string(retBytes[k]))
	}
	return ret
}

// PostFormEx is like PostForm(key). It returns the specified key from a POST
// urlencoded form or multipart form when it exists `(value, true)` (even when
// the value is an empty string), otherwise it returns ("", false).
func (c *Context) PostFormEx(key string) (string, bool) {
	args := c.PostArgs()
	return string(args.Peek(key)), args.Has(key)
}

// PostFormArrayEx returns a slice of strings for a given form key and
// a boolean value whether at least one value exists for the given key.
func (c *Context) PostFormArrayEx(key string) ([]string, bool) {
	var ret []string
	args := c.PostArgs()
	if args.Has(key) {
		retBytes := args.PeekMulti(key)
		for k := range retBytes {
			ret = append(ret, string(retBytes[k]))
		}
		return ret, true
	}
	return ret, false
}

// Query returns the keyed url query value if it exists, otherwise it
// returns an empty string "".
func (c *Context) Query(key string) string {
	return string(c.QueryArgs().Peek(key))
}

// QueryArray returns a slice of strings for a given query key.
// The length of the slice depends on the number of params with the given key.
func (c *Context) QueryArray(key string) []string {
	var ret []string
	retBytes := c.QueryArgs().PeekMulti(key)
	for k := range retBytes {
		ret = append(ret, string(retBytes[k]))
	}
	return ret
}

// QueryEx is like Query(), it returns the keyed url query value if it exists `(value, true)`
// (even when the value is an empty string), otherwise it returns `("", false)`.
func (c *Context) QueryEx(key string) (string, bool) {
	args := c.QueryArgs()
	return string(args.Peek(key)), args.Has(key)
}

// QueryArrayEx returns a slice of strings for a given query key, plus a boolean value
// whether at least one value exists for the given key.
func (c *Context) QueryArrayEx(key string) ([]string, bool) {
	var ret []string
	args := c.QueryArgs()
	if args.Has(key) {
		retBytes := args.PeekMulti(key)
		for k := range retBytes {
			ret = append(ret, string(retBytes[k]))
		}
		return ret, true
	}
	return ret, false
}
