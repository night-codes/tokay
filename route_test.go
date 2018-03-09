package tokay

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockStore struct {
	*store
	data map[string]interface{}
}

func newMockStore() *mockStore {
	return &mockStore{newStore(), make(map[string]interface{})}
}

func (s *mockStore) Add(key string, data interface{}) int {
	for _, handler := range data.([]Handler) {
		handler(nil)
	}
	return s.store.Add(key, data)
}

func TestRouteNew(t *testing.T) {
	router := New()
	group := newRouteGroup("/admin", router, nil)

	r1 := newRoute("/users", group)
	assert.Equal(t, "/admin/users", r1.name, "route.name =")
	assert.Equal(t, "/admin/users", r1.path, "route.path =")
	assert.Equal(t, "/admin/users", r1.template, "route.template =")
	_, exists := router.routes[r1.name]
	assert.True(t, exists, "router.routes[name] is ")

	r2 := newRoute("/users/<id:\\d+>/*", group)
	assert.Equal(t, "/admin/users/<id:\\d+>/*", r2.name, "route.name =")
	assert.Equal(t, "/admin/users/<id:\\d+>/<:.*>", r2.path, "route.path =")
	assert.Equal(t, "/admin/users/<id>/<>", r2.template, "route.template =")
	_, exists = router.routes[r2.name]
	assert.True(t, exists, "router.routes[name] is ")
}

func TestRouteName(t *testing.T) {
	router := New()
	group := newRouteGroup("/admin", router, nil)

	r1 := newRoute("/users", group)
	assert.Equal(t, "/admin/users", r1.name, "route.name =")
	r1.Name("user")
	assert.Equal(t, "user", r1.name, "route.name =")
	_, exists := router.routes[r1.name]
	assert.True(t, exists, "router.routes[name] is ")
}

func TestRouteURL(t *testing.T) {
	router := New()
	group := newRouteGroup("/admin", router, nil)
	r := newRoute("/users/<id:\\d+>/<action>/*", group)
	assert.Equal(t, "/admin/users/123/address/<>", r.URL("id", 123, "action", "address"), "Route.URL@1 =")
	assert.Equal(t, "/admin/users/123/<action>/<>", r.URL("id", 123), "Route.URL@2 =")
	assert.Equal(t, "/admin/users/123//<>", r.URL("id", 123, "action"), "Route.URL@3 =")
	assert.Equal(t, "/admin/users/123/profile/", r.URL("id", 123, "action", "profile", ""), "Route.URL@4 =")
	assert.Equal(t, "/admin/users/123/profile/xyz%2Fabc", r.URL("id", 123, "action", "profile", "", "xyz/abc"), "Route.URL@5 =")
	assert.Equal(t, "/admin/users/123/a%2C%3C%3E%3F%23/<>", r.URL("id", 123, "action", "a,<>?#"), "Route.URL@6 =")
}

func newHandler(tag string, buf *bytes.Buffer) Handler {
	return func(*Context) {
		fmt.Fprintf(buf, tag)
	}
}

func TestRouteAdd(t *testing.T) {
	store := newMockStore()
	router := New()
	router.stores.Set("GET", store)
	assert.Equal(t, 0, store.count, "router.stores.Set(GET).count =")

	var buf bytes.Buffer

	group := newRouteGroup("/admin", router, []Handler{newHandler("1.", &buf), newHandler("2.", &buf)})
	newRoute("/users", group).GET(newHandler("3.", &buf), newHandler("4.", &buf))
	assert.Equal(t, "1.2.3.4.", buf.String(), "buf@1 =")

	buf.Reset()
	group = newRouteGroup("/admin", router, []Handler{})
	newRoute("/users", group).GET(newHandler("3.", &buf), newHandler("4.", &buf))
	assert.Equal(t, "3.4.", buf.String(), "buf@2 =")

	buf.Reset()
	group = newRouteGroup("/admin", router, []Handler{newHandler("1.", &buf), newHandler("2.", &buf)})
	newRoute("/users", group).GET()
	assert.Equal(t, "1.2.", buf.String(), "buf@3 =")
}

func TestRouteMethods(t *testing.T) {
	router := New()
	for _, method := range Methods {
		store := newMockStore()
		router.stores.Set(method, store)
		assert.Equal(t, 0, store.count, "router.stores.Set("+method+", store).count =")
	}
	group := newRouteGroup("/admin", router, nil)

	newRoute("/users", group).GET()
	assert.Equal(t, 1, router.stores.Get("GET").(*mockStore).count, "router.stores.Get(GET).count =")
	newRoute("/users", group).POST()
	assert.Equal(t, 1, router.stores.Get("POST").(*mockStore).count, "router.stores.Get(POST).count =")
	newRoute("/users", group).PATCH()
	assert.Equal(t, 1, router.stores.Get("PATCH").(*mockStore).count, "router.stores.Get(PATCH).count =")
	newRoute("/users", group).PUT()
	assert.Equal(t, 1, router.stores.Get("PUT").(*mockStore).count, "router.stores.Get(PUT).count =")
	newRoute("/users", group).DELETE()
	assert.Equal(t, 1, router.stores.Get("DELETE").(*mockStore).count, "router.stores.Get(DELETE).count =")
	newRoute("/users", group).CONNECT()
	assert.Equal(t, 1, router.stores.Get("CONNECT").(*mockStore).count, "router.stores.Get(CONNECT).count =")
	newRoute("/users", group).HEAD()
	assert.Equal(t, 1, router.stores.Get("HEAD").(*mockStore).count, "router.stores.Get(HEAD).count =")
	newRoute("/users", group).OPTIONS()
	assert.Equal(t, 1, router.stores.Get("OPTIONS").(*mockStore).count, "router.stores.Get(OPTIONS).count =")
	newRoute("/users", group).TRACE()
	assert.Equal(t, 1, router.stores.Get("TRACE").(*mockStore).count, "router.stores.Get(TRACE).count =")

	newRoute("/posts", group).To("GET,POST")
	assert.Equal(t, 2, router.stores.Get("GET").(*mockStore).count, "router.stores.Get(GET).count =")
	assert.Equal(t, 2, router.stores.Get("POST").(*mockStore).count, "router.stores.Get(POST).count =")
	assert.Equal(t, 1, router.stores.Get("PUT").(*mockStore).count, "router.stores.Get(PUT).count =")
}

func TestBuildURLTemplate(t *testing.T) {
	tests := []struct {
		path, expected string
	}{
		{"", ""},
		{"/users", "/users"},
		{"<id>", "<id>"},
		{"<id", "<id"},
		{"/users/<id>", "/users/<id>"},
		{"/users/<id:\\d+>", "/users/<id>"},
		{"/users/<:\\d+>", "/users/<>"},
		{"/users/<id>/xyz", "/users/<id>/xyz"},
		{"/users/<id:\\d+>/xyz", "/users/<id>/xyz"},
		{"/users/<id:\\d+>/<test>", "/users/<id>/<test>"},
		{"/users/<id:\\d+>/<test>/", "/users/<id>/<test>/"},
		{"/users/<id:\\d+><test>", "/users/<id><test>"},
		{"/users/<id:\\d+><test>/", "/users/<id><test>/"},
	}
	for _, test := range tests {
		actual := buildURLTemplate(test.path)
		assert.Equal(t, test.expected, actual, "buildURLTemplate("+test.path+") =")
	}
}
