package firefoxkit

// DialogType enumerates the kinds of dialogs Firefox emits via BiDi.
type DialogType string

const (
	DialogAlert        DialogType = "alert"
	DialogConfirm      DialogType = "confirm"
	DialogPrompt       DialogType = "prompt"
	DialogBeforeUnload DialogType = "beforeunload"
)

// DialogEvent describes a JS dialog the page just opened. Passed to the
// handler registered via Page.OnDialog. Matches chromekit's DialogEvent shape.
type DialogEvent struct {
	Type          DialogType
	Message       string
	DefaultPrompt string // only meaningful for DialogPrompt
}

// DialogAction tells firefoxkit how to respond to the dialog. The zero value
// (Accept=false) dismisses/cancels the dialog. Matches chromekit's DialogAction
// shape exactly.
type DialogAction struct {
	Accept     bool
	PromptText string // only used when Accept=true on a prompt dialog
}
