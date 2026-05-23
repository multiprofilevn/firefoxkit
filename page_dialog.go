package firefoxkit

import (
	"context"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// DialogType enumerates the kinds of dialogs Firefox emits via BiDi.
type DialogType string

const (
	DialogAlert        DialogType = "alert"
	DialogConfirm      DialogType = "confirm"
	DialogPrompt       DialogType = "prompt"
	DialogBeforeUnload DialogType = "beforeunload"
)

// Dialog represents an open user prompt. The handler must call Accept or
// Dismiss to release it; otherwise the page hangs until the BiDi timeout.
type Dialog struct {
	page         *Page
	contextID    string
	Type         DialogType
	Message      string
	DefaultValue string
}

// Accept confirms the dialog. text is filled into prompt() inputs; ignored
// for other dialog types.
func (d *Dialog) Accept(ctx context.Context, text string) error {
	accept := true
	return d.page.browser.client.HandleUserPrompt(ctx, bidi.HandleUserPromptParams{
		Context:  d.contextID,
		Accept:   &accept,
		UserText: text,
	})
}

// Dismiss cancels the dialog.
func (d *Dialog) Dismiss(ctx context.Context) error {
	dismiss := false
	return d.page.browser.client.HandleUserPrompt(ctx, bidi.HandleUserPromptParams{
		Context: d.contextID,
		Accept:  &dismiss,
	})
}
