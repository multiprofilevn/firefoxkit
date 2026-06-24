package firefoxkit

import "github.com/tuwibu/firefoxkit/input"

// keyEntry maps a chromekit-compatible key name to the BiDi/W3C key value and
// the VK code needed for KeyCombo (modifier path). Named keys without a rune
// carry vk only; printable single-character keys carry only the key string.
type keyEntry struct {
	// w3cKey is the WebDriver BiDi key value sent in keyDown/keyUp actions.
	// For named keys this is a W3C Unicode code-point string (e.g. "").
	// For printable chars this is just the character itself.
	w3cKey string
	// vk is the Win32 virtual-key code for KeyCombo dispatch.
	vk uint16
	// isPrintable signals the key produces visible text (Press uses KeyChar).
	isPrintable bool
}

// keyNameMap is the canonical map of key-name strings to their dispatch
// descriptors. Names match chromekit's keyMap exactly so callers can share
// key-name strings across both toolkits.
//
// Sources:
//   - chromekit page_keyboard_api.go keyMap
//   - W3C WebDriver BiDi key values (same Unicode PUA code-points as
//     WebDriver classic / Selenium)
var keyNameMap = map[string]keyEntry{
	// Navigation / editing keys (chromekit canonical set)
	"Enter":      {w3cKey: input.KeyEnter, vk: input.VKReturn},
	"Tab":        {w3cKey: input.KeyTab, vk: input.VKTab},
	"Escape":     {w3cKey: input.KeyEscape, vk: input.VKEscape},
	"Esc":        {w3cKey: input.KeyEscape, vk: input.VKEscape}, // alias
	"Backspace":  {w3cKey: input.KeyBackspace, vk: input.VKBackspace},
	"Delete":     {w3cKey: input.KeyDelete, vk: input.VKDelete},
	"ArrowUp":    {w3cKey: input.KeyArrowUp, vk: input.VKUp},
	"ArrowDown":  {w3cKey: input.KeyArrowDown, vk: input.VKDown},
	"ArrowLeft":  {w3cKey: input.KeyArrowLeft, vk: input.VKLeft},
	"ArrowRight": {w3cKey: input.KeyArrowRight, vk: input.VKRight},
	"Home":       {w3cKey: input.KeyHome, vk: input.VKHome},
	"End":        {w3cKey: input.KeyEnd, vk: input.VKEnd},
	"PageUp":     {w3cKey: input.KeyPageUp, vk: input.VKPageUp},
	"PageDown":   {w3cKey: input.KeyPageDown, vk: input.VKPageDown},
	"Space":      {w3cKey: " ", vk: input.VKSpace, isPrintable: true},

	// Modifier keys (send as keyDown/keyUp without producing text)
	"Control": {w3cKey: input.KeyControl, vk: input.VKControl},
	"Shift":   {w3cKey: input.KeyShift, vk: input.VKShift},
	"Alt":     {w3cKey: input.KeyAlt, vk: input.VKMenu},
	"Meta":    {w3cKey: input.KeyMeta, vk: input.VKWin},

	// Function keys
	"F1":  {w3cKey: input.KeyF1, vk: input.VKF1},
	"F2":  {w3cKey: input.KeyF2, vk: input.VKF2},
	"F3":  {w3cKey: input.KeyF3, vk: input.VKF3},
	"F4":  {w3cKey: input.KeyF4, vk: input.VKF4},
	"F5":  {w3cKey: input.KeyF5, vk: input.VKF5},
	"F6":  {w3cKey: input.KeyF6, vk: input.VKF6},
	"F7":  {w3cKey: input.KeyF7, vk: input.VKF7},
	"F8":  {w3cKey: input.KeyF8, vk: input.VKF8},
	"F9":  {w3cKey: input.KeyF9, vk: input.VKF9},
	"F10": {w3cKey: input.KeyF10, vk: input.VKF10},
	"F11": {w3cKey: input.KeyF11, vk: input.VKF11},
	"F12": {w3cKey: input.KeyF12, vk: input.VKF12},
}

// resolveKey returns the keyEntry for a key name string. For single printable
// characters not found in keyNameMap, it synthesises an entry from the
// character itself so any Unicode rune can be typed.
//
// Returns (entry, true) on success; (zero, false) when the name has no mapping
// and is not a single-rune string.
func resolveKey(key string) (keyEntry, bool) {
	if e, ok := keyNameMap[key]; ok {
		return e, true
	}
	// Single printable character fallback — matches chromekit's dispatchKey
	// `WithKey(key)` path for arbitrary characters.
	runes := []rune(key)
	if len(runes) == 1 {
		return keyEntry{w3cKey: key, isPrintable: true}, true
	}
	return keyEntry{}, false
}
