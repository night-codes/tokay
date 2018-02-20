package tokay

import (
	"sync"
)

type (
	// routeStore stores route paths and the corresponding handlers.
	routeStore interface {
		Add(key string, data interface{}) int
		Get(key string, pvalues []string) (data interface{}, pnames []string)
		String() string
	}

	storesMap struct {
		sync.RWMutex
		M map[string]routeStore
	}
)

func newStoresMap() *storesMap {
	return &storesMap{M: make(map[string]routeStore)}
}

func (m *storesMap) Set(key string, val routeStore) {
	m.Lock()
	m.M[key] = val
	m.Unlock()
}

func (m *storesMap) Range(fn func(key string, value routeStore)) {
	m.Lock()
	for key, value := range m.M {
		fn(key, value)
	}
	m.Unlock()
}

func (m *storesMap) Get(key string) routeStore {
	m.RLock()
	v := m.M[key]
	m.RUnlock()

	return v
}
