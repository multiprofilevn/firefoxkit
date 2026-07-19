package firefoxkit

// FirstIframeRectPierce finds the first <iframe> reachable by walking OPEN shadow
// roots (el.shadowRoot) and returns its viewport rectangle. found=false when no
// iframe is reachable.
//
// LIMITATION (parity gap vs chromekit): WebDriver BiDi / Firefox cannot pierce
// CLOSED shadow roots — el.shadowRoot is null for them — so an iframe nested in a
// closed shadow root is unreachable and yields found=false. chromekit's CDP
// dom.GetDocument().WithPierce(true) does traverse closed roots. Callers needing
// closed-shadow support on Firefox must fall back (e.g. a solver extension).
func (p *Page) FirstIframeRectPierce() (BoundingBox, bool, error) {
	const js = `(() => {
		function walk(root) {
			const all = root.querySelectorAll ? root.querySelectorAll('*') : [];
			for (const el of all) {
				if (el.tagName === 'IFRAME') return el;
				if (el.shadowRoot) { const f = walk(el.shadowRoot); if (f) return f; }
			}
			return null;
		}
		const iframe = walk(document);
		if (!iframe) return { found: false, x: 0, y: 0, width: 0, height: 0 };
		const r = iframe.getBoundingClientRect();
		return { found: true, x: r.left, y: r.top, width: r.width, height: r.height };
	})()`
	var out struct {
		Found  bool    `json:"found"`
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := p.Evaluate(js, &out); err != nil {
		return BoundingBox{}, false, err
	}
	if !out.Found {
		return BoundingBox{}, false, nil
	}
	return BoundingBox{X: out.X, Y: out.Y, Width: out.Width, Height: out.Height}, true, nil
}
