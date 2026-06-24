package firefoxkit

import (
	"encoding/json"
	"fmt"
)

// SetValue programmatically sets el.value and dispatches input + change events.
// Useful for forms that bind to JS frameworks (React/Vue) which sometimes
// ignore native typing but watch for these synthetic events.
func (p *Page) SetValue(sel, value string) error {
	valJSON, _ := json.Marshal(value)
	expr := fmt.Sprintf(`(function(){
		const el = document.querySelector(%q);
		if (!el) return false;
		// React tracks the previous value on the input to dedupe events;
		// using the prototype setter ensures the new value is observed.
		const proto = Object.getPrototypeOf(el);
		const setter = Object.getOwnPropertyDescriptor(proto, "value");
		if (setter && setter.set) setter.set.call(el, %s);
		else el.value = %s;
		el.dispatchEvent(new Event("input", {bubbles:true}));
		el.dispatchEvent(new Event("change", {bubbles:true}));
		return true;
	})()`, sel, string(valJSON), string(valJSON))
	var ok bool
	if err := p.Evaluate(expr, &ok); err != nil {
		return err
	}
	if !ok {
		return ErrPageNotFound
	}
	return nil
}

// CheckBox sets el.checked = checked on the matched input.
func (p *Page) CheckBox(sel string, checked bool) error {
	expr := fmt.Sprintf(`(function(){
		const el = document.querySelector(%q);
		if (!el) return false;
		el.checked = %v;
		el.dispatchEvent(new Event("input", {bubbles:true}));
		el.dispatchEvent(new Event("change", {bubbles:true}));
		return true;
	})()`, sel, checked)
	var ok bool
	if err := p.Evaluate(expr, &ok); err != nil {
		return err
	}
	if !ok {
		return ErrPageNotFound
	}
	return nil
}
