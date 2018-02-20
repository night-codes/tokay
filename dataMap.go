package tokay

import (
	"sync"
)

type dataMap struct {
	sync.RWMutex
	M map[string]interface{}
}

func newDataMap() *dataMap {
	return &dataMap{M: make(map[string]interface{})}
}

func (m *dataMap) Copy() (c map[string]interface{}) {
	c = make(map[string]interface{}, len(m.M))

	m.RLock()
	for k, v := range m.M {
		c[k] = v
	}
	m.RUnlock()

	return c
}

func (m *dataMap) Set(key string, val interface{}) {
	m.Lock()
	m.M[key] = val
	m.Unlock()
}

func (m *dataMap) Clear() {
	m.Lock()
	m.M = make(map[string]interface{})
	m.Unlock()
}

func (m *dataMap) Range(fn func(key string, value interface{})) {
	m.Lock()
	for key, value := range m.M {
		fn(key, value)
	}
	m.Unlock()
}

func (m *dataMap) Replace(newMap map[string]interface{}) {
	m.Lock()
	m.M = newMap
	m.Unlock()
}

func (m *dataMap) Delete(key string) {
	m.Lock()
	delete(m.M, key)
	m.Unlock()
}

func (m *dataMap) Get(key string) interface{} {
	m.RLock()
	v := m.M[key]
	m.RUnlock()

	return v
}

func (m *dataMap) Len() int {
	m.RLock()
	n := len(m.M)
	m.RUnlock()

	return n
}

func (m *dataMap) GetEx(key string) (interface{}, bool) {
	m.RLock()
	v, exists := m.M[key]
	m.RUnlock()
	return v, exists
}
