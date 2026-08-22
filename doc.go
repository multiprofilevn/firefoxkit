// Package firefoxkit wraps Firefox automation via the W3C WebDriver BiDi
// protocol with a Puppeteer-like API. Mirrors github.com/multiprofilevn/chromekit:
//
//	b, _ := firefoxkit.Launch(`C:\Program Files\Mozilla Firefox\firefox.exe`)
//	defer b.Close()
//	p := b.Current()
//	p.Navigate("https://example.com", 30*time.Second)
//
// Firefox 129+ required (CDP support was removed; BiDi is the replacement).
// Native input backend (winapi) is reused 1:1 from chromekit; only the
// window class and content offset differ.
package firefoxkit
