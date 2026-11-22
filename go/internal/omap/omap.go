// Package omap provides an insertion-ordered map and Python-compatible JSON
// output, so that results keep the key order of the Python implementation.
package omap

// Map is a string-keyed map that remembers insertion order (like a Python dict).
type Map struct {
	keys []string
	vals map[string]any
}

// New builds a Map from key, value pairs: New("a", 1, "b", 2).
func New(kv ...any) *Map {
	m := &Map{vals: map[string]any{}}
	for i := 0; i+1 < len(kv); i += 2 {
		m.Set(kv[i].(string), kv[i+1])
	}
	return m
}

// Set stores a value; an existing key keeps its position.
func (m *Map) Set(key string, value any) {
	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = value
}

// Get returns the value and whether the key exists.
func (m *Map) Get(key string) (any, bool) {
	v, ok := m.vals[key]
	return v, ok
}

// Has reports whether the key exists.
func (m *Map) Has(key string) bool {
	_, ok := m.vals[key]
	return ok
}

// Delete removes a key.
func (m *Map) Delete(key string) {
	if _, ok := m.vals[key]; !ok {
		return
	}
	delete(m.vals, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i:i], m.keys[i+1:]...)
			break
		}
	}
}

// Keys returns the keys in insertion order (a copy).
func (m *Map) Keys() []string { return append([]string(nil), m.keys...) }

// Len is the number of keys.
func (m *Map) Len() int { return len(m.keys) }

// Copy returns a shallow copy.
func (m *Map) Copy() *Map {
	c := &Map{vals: make(map[string]any, len(m.keys))}
	for _, k := range m.keys {
		c.Set(k, m.vals[k])
	}
	return c
}

// List is a growable list that can be stored in a Map before it is filled.
type List struct{ Items []any }

// Append adds an item.
func (l *List) Append(v any) { l.Items = append(l.Items, v) }

// Plain converts Map and List (recursively) to map[string]any and []any.
func Plain(v any) any {
	switch t := v.(type) {
	case *Map:
		out := make(map[string]any, t.Len())
		for _, k := range t.keys {
			out[k] = Plain(t.vals[k])
		}
		return out
	case *List:
		return Plain(t.Items)
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = Plain(x)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = Plain(x)
		}
		return out
	}
	return v
}
