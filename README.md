![Tokay](https://cdn.rawgit.com/night-codes/9160052050046367b8cd329c0250531d/raw/72544777a52ff1e8d60cdbcd7c504c440544b9a3/tokey.svg)


[![GoDoc](https://godoc.org/github.com/night-codes/tokay?status.png)](https://godoc.org/github.com/night-codes/tokay)
[![Go Report](http://goreportcard.com/badge/night-codes/tokay)](http://goreportcard.com/report/night-codes/tokay)

## Description

**Tokay** is small and fast web framework written in Go (Golang) for the high-performance [fasthttp](https://github.com/valyala/fasthttp) server.
The package has the following features:

* middleware pipeline architecture, similar to that of the [Express framework](http://expressjs.com).
* extremely fast request routing with zero dynamic memory allocation
* modular code organization through route grouping
* flexible URL path matching, supporting URL parameters and regular expressions
* URL creation according to the predefined routes
* HTML, XML, JSON etc. rendering based on package [night-codes/tokay-render](https://github.com/night-codes/tokay-render)

## Requirements

Go 1.8 or above.

## Installation

Run the following command to install the package:

```
go get github.com/night-codes/tokay
```

## Getting Started

Create a `server.go` file with the following content:

```go
package main

import (
	"github.com/night-codes/tokay"
)

func main() {
	router := tokay.New()
	
	router.GET("/", func(c *tokay.Context) {
		c.String(200, "Hello, world!")
	})
	
	panic(router.Run(":8080"))
}
```

Now run the following command to start the Web server:

```
go run server.go
```

You should be able to access URLs such as `http://localhost:8080`.


### Routes

Routing works by building a routing table in a router and then dispatching HTTP requests to the matching handlers 
found in the routing table. An intuitive illustration of a routing table is as follows:


Routes              |  Handlers
--------------------|-----------------
`GET /users`        |  m1, m2, h1, ...
`POST /users`       |  m1, m2, h2, ...
`PUT /users/<id>`   |  m1, m2, h3, ...
`DELETE /users/<id>`|  m1, m2, h4, ...


For an incoming request `GET /users`, the first route would match and the handlers m1, m2, and h1 would be executed.
If the request is `PUT /users/123`, the third route would match and the corresponding handlers would be executed.
Note that the token `<id>` can match any number of non-slash characters and the matching part can be accessed as 
a path parameter value in the handlers.

**If an incoming request matches multiple routes in the table, the route added first to the table will take precedence.
All other matching routes will be ignored.**

The actual implementation of the routing table uses a variant of the radix tree data structure, which makes the routing
process as fast as working with a hash table, thanks to the inspiration from [httprouter](https://github.com/julienschmidt/httprouter).

To add a new route and its handlers to the routing table, call the `To` method like the following:
  
```go
router := tokay.New()
router.To("GET", "/users", m1, m2, h1)
router.To("POST", "/users", m1, m2, h2)
```

You can also use shortcut methods, such as `Get`, `Post`, `Put`, etc., which are named after the HTTP method names:
 
```go
router.GET("/users", m1, m2, h1)
router.POST("/users", m1, m2, h2)
```

If you have multiple routes with the same URL path but different HTTP methods, like the above example, you can 
chain them together as follows,

```go
router.GET("/users", m1, m2, h1).POST(m1, m2, h2)
```

If you want to use the same set of handlers to handle the same URL path but different HTTP methods, you can take
the following shortcut:

```go
router.To("GET,POST", "/users", m1, m2, h)
```

A route may contain parameter tokens which are in the format of `<name:pattern>`, where `name` stands for the parameter
name, and `pattern` is a regular expression which the parameter value should match. A token `<name>` is equivalent
to `<name:[^/]*>`, i.e., it matches any number of non-slash characters. At the end of a route, an asterisk character
can be used to match any number of arbitrary characters. Below are some examples:

* `/users/<username>`: matches `/users/admin`
* `/users/accnt-<id:\d+>`: matches `/users/accnt-123`, but not `/users/accnt-admin`
* `/users/<username>/*`: matches `/users/admin/profile/address`

When a URL path matches a route, the matching parameters on the URL path can be accessed via `Context.Param()`:

```go
router := tokay.New()

router.GET("/users/<username>", func (c *tokay.Context) {
	fmt.Fprintf(c, "Name: %v", c.Param("username"))
})
```


### Route Groups

Route group is a way of grouping together the routes which have the same route prefix. The routes in a group also
share the same handlers that are registered with the group via its `Use` method. For example,

```go
router := tokay.New()
api := router.Group("/api")
api.Use(m1, m2)
api.GET("/users", h1).POST(h2)
api.PUT("/users/<id>", h3).DELETE(h4)
```

The above `/api` route group establishes the following routing table:


Routes                  |  Handlers
------------------------|-------------
`GET /api/users`        |  m1, m2, h1, ...
`POST /api/users`       |  m1, m2, h2, ...
`PUT /api/users/<id>`   |  m1, m2, h3, ...
`DELETE /api/users/<id>`|  m1, m2, h4, ...


As you can see, all these routes have the same route prefix `/api` and the handlers `m1` and `m2`. In other similar
routing frameworks, the handlers registered with a route group are also called *middlewares*.

Route groups can be nested. That is, a route group can create a child group by calling the `Group()` method. The router
serves as the top level route group. A child group inherits the handlers registered with its parent group. For example, 

```go
router := tokay.New()
router.Use(m1)

api := router.Group("/api")
api.Use(m2)

users := api.Group("/users")
users.Use(m3)
users.PUT("/<id>", h1)
```

Because the router serves as the parent of the `api` group which is the parent of the `users` group, 
the `PUT /api/users/<id>` route is associated with the handlers `m1`, `m2`, `m3`, and `h1`.


### Router

Router manages the routing table and dispatches incoming requests to appropriate handlers. A router instance is created
by calling the `tokay.New()` method.

To hook up router with fasthttp, use the following code:

```go
router := tokay.New()
panic(router.Run(":8080"))
```


### Handlers

A handler is a function with the signature `func(*tokay.Context)`. A handler is executed by the router if
the incoming request URL path matches the route that the handler is associated with. Through the `tokay.Context` 
parameter, you can access the request information in handlers.

A route may be associated with multiple handlers. These handlers will be executed in the order that they are registered
to the route. The execution sequence can be terminated in the middle using one of the following method. 
A handler calls `Context.Abort()`: the router will simply skip the rest of the handlers. 
 
A handler can call `Context.Next()` to explicitly execute the rest of the unexecuted handlers and take actions after
they finish execution. For example, a response compression handler may start the output buffer, call `Context.Next()`,
and then compress and send the output to response.


### Context

For each incoming request, a `tokay.Context` object is passed through the relevant handlers. Because `tokay.Context`
embeds `fasthttp.RequestCtx`, you can access all properties and methods provided by the latter.
 
Additionally, the `Context.Param()` method allows handlers to access the URL path parameters that match the current route.
Using `Context.Get()` and `Context.Set()`, handlers can share data between each other. For example, an authentication
handler can store the authenticated user identity by calling `Context.Set()`, and other handlers can retrieve back
the identity information by calling `Context.Get()`.

Context also provides a handy `WriteData()` method that can be used to write data of arbitrary type to the response.
The `WriteData()` method can also be overridden (by replacement) to achieve more versatile response data writing. 


### Error Handling

When an incoming request has no matching route, the router will call the handlers registered via the `Router.NotFound()`
method. All the handlers registered via `Router.Use()` will also be called in advance. By default, the following two
handlers are registered with `Router.NotFound()`:

* `tokay.MethodNotAllowedHandler`: a handler that sends an `Allow` HTTP header indicating the allowed HTTP methods for a requested URL
* `tokay.NotFoundHandler`: a handler triggering 404 HTTP error
