package firefoxkit

import (
	"encoding/json"
	"fmt"
)

// Storage is a thin wrapper over localStorage or sessionStorage. Holds no
// state of its own; every call hits the page via Evaluate.
type Storage struct {
	p    *Page
	name string // "localStorage" | "sessionStorage"
}

// LocalStorage returns a handle to the page's window.localStorage.
func (p *Page) LocalStorage() *Storage { return &Storage{p: p, name: "localStorage"} }

// SessionStorage returns a handle to the page's window.sessionStorage.
func (p *Page) SessionStorage() *Storage { return &Storage{p: p, name: "sessionStorage"} }

// Get returns the value for key, or "" if absent.
func (s *Storage) Get(key string) (string, error) {
	keyJSON, _ := json.Marshal(key)
	expr := fmt.Sprintf(`window.%s.getItem(%s) || ""`, s.name, string(keyJSON))
	var v string
	if err := s.p.Evaluate(expr, &v); err != nil {
		return "", err
	}
	return v, nil
}

// Set writes value at key.
func (s *Storage) Set(key, value string) error {
	keyJSON, _ := json.Marshal(key)
	valJSON, _ := json.Marshal(value)
	expr := fmt.Sprintf(`(function(){ window.%s.setItem(%s, %s); return true; })()`,
		s.name, string(keyJSON), string(valJSON))
	return s.p.Evaluate(expr, nil)
}

// Remove deletes key (no-op if missing).
func (s *Storage) Remove(key string) error {
	keyJSON, _ := json.Marshal(key)
	expr := fmt.Sprintf(`(function(){ window.%s.removeItem(%s); return true; })()`,
		s.name, string(keyJSON))
	return s.p.Evaluate(expr, nil)
}

// Clear empties the storage for the current origin.
func (s *Storage) Clear() error {
	expr := fmt.Sprintf(`(function(){ window.%s.clear(); return true; })()`, s.name)
	return s.p.Evaluate(expr, nil)
}

// Keys returns the list of keys in storage iteration order. Extra FF method
// not present in chromekit — retained for backward compatibility.
func (s *Storage) Keys() ([]string, error) {
	expr := fmt.Sprintf(
		`(function(){ const out = []; for (let i=0; i<window.%s.length; i++) out.push(window.%s.key(i)); return out; })()`,
		s.name, s.name)
	var keys []string
	if err := s.p.Evaluate(expr, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}
