package firefoxkit

import (
	"context"
	"encoding/json"
	"fmt"
)

// StorageAPI groups web-storage namespaces.
type StorageAPI struct{ p *Page }

// Storage returns the web-storage namespace.
func (p *Page) Storage() *StorageAPI { return &StorageAPI{p: p} }

// Local returns the localStorage namespace.
func (a *StorageAPI) Local() *WebStorage {
	return &WebStorage{p: a.p, name: "localStorage"}
}

// Session returns the sessionStorage namespace.
func (a *StorageAPI) Session() *WebStorage {
	return &WebStorage{p: a.p, name: "sessionStorage"}
}

// WebStorage wraps either localStorage or sessionStorage. All ops execute
// via script.evaluate against the page's realm — content-script isolation
// not currently supported.
type WebStorage struct {
	p    *Page
	name string // "localStorage" | "sessionStorage"
}

// Get returns the value for key, or "" if absent.
func (w *WebStorage) Get(ctx context.Context, key string) (string, error) {
	keyJSON, _ := json.Marshal(key)
	expr := fmt.Sprintf(`window.%s.getItem(%s) || ""`, w.name, string(keyJSON))
	var v string
	if err := w.p.Evaluate(ctx, expr, &v); err != nil {
		return "", err
	}
	return v, nil
}

// Set writes value at key.
func (w *WebStorage) Set(ctx context.Context, key, value string) error {
	keyJSON, _ := json.Marshal(key)
	valJSON, _ := json.Marshal(value)
	expr := fmt.Sprintf(`(function(){ window.%s.setItem(%s, %s); return true; })()`,
		w.name, string(keyJSON), string(valJSON))
	return w.p.Evaluate(ctx, expr, nil)
}

// Remove deletes key.
func (w *WebStorage) Remove(ctx context.Context, key string) error {
	keyJSON, _ := json.Marshal(key)
	expr := fmt.Sprintf(`(function(){ window.%s.removeItem(%s); return true; })()`,
		w.name, string(keyJSON))
	return w.p.Evaluate(ctx, expr, nil)
}

// Clear empties the storage.
func (w *WebStorage) Clear(ctx context.Context) error {
	expr := fmt.Sprintf(`(function(){ window.%s.clear(); return true; })()`, w.name)
	return w.p.Evaluate(ctx, expr, nil)
}

// Keys returns the list of keys in deterministic order (Storage iteration order).
func (w *WebStorage) Keys(ctx context.Context) ([]string, error) {
	expr := fmt.Sprintf(`(function(){ const out = []; for (let i=0; i<window.%s.length; i++) out.push(window.%s.key(i)); return out; })()`,
		w.name, w.name)
	var keys []string
	if err := w.p.Evaluate(ctx, expr, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}
