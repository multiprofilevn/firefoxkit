package firefoxkit

import "fmt"

// SetSelectValue assigns value to the <select> at selector and dispatches
// input + change events. Chromekit API parity alias for SelectByValue.
func (p *Page) SetSelectValue(selector, value string) error {
	return p.SelectByValue(selector, value)
}

// SelectByValue picks the option matching value in the <select> at sel.
// Fires both `input` and `change` events.
func (p *Page) SelectByValue(sel, value string) error {
	fnBody := fmt.Sprintf(`(el) => {
		const value = %q;
		const opt = Array.from(el.options).find(o => o.value === value);
		if (!opt) return false;
		el.value = value;
		el.dispatchEvent(new Event("input", {bubbles:true}));
		el.dispatchEvent(new Event("change", {bubbles:true}));
		return true;
	}`, value)
	var ok bool
	if err := p.EvalSelector(sel, fnBody, &ok); err != nil {
		return err
	}
	if !ok {
		return ErrPageNotFound
	}
	return nil
}

// SelectByIndex picks options[index].
func (p *Page) SelectByIndex(sel string, index int) error {
	fnBody := fmt.Sprintf(`(el) => {
		const i = %d;
		if (i < 0 || i >= el.options.length) return false;
		el.selectedIndex = i;
		el.dispatchEvent(new Event("input", {bubbles:true}));
		el.dispatchEvent(new Event("change", {bubbles:true}));
		return true;
	}`, index)
	var ok bool
	if err := p.EvalSelector(sel, fnBody, &ok); err != nil {
		return err
	}
	if !ok {
		return ErrPageNotFound
	}
	return nil
}

// SelectByText picks the first option whose visible label matches text.
func (p *Page) SelectByText(sel, text string) error {
	fnBody := fmt.Sprintf(`(el) => {
		const text = %q;
		const opt = Array.from(el.options).find(o => (o.textContent || "").trim() === text);
		if (!opt) return false;
		el.value = opt.value;
		el.dispatchEvent(new Event("input", {bubbles:true}));
		el.dispatchEvent(new Event("change", {bubbles:true}));
		return true;
	}`, text)
	var ok bool
	if err := p.EvalSelector(sel, fnBody, &ok); err != nil {
		return err
	}
	if !ok {
		return ErrPageNotFound
	}
	return nil
}
