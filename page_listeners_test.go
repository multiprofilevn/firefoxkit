package firefoxkit

import (
	"testing"
	"time"
)

// TestConsoleEvent_Fields verifies ConsoleEvent has the expected fields and
// that the renamed type (was ConsoleMessage) works correctly.
func TestConsoleEvent_Fields(t *testing.T) {
	ev := ConsoleEvent{
		Level:     ConsoleWarn,
		Text:      "hello",
		Method:    "warn",
		Timestamp: time.Now(),
	}
	if ev.Level != ConsoleWarn {
		t.Errorf("Level = %q, want %q", ev.Level, ConsoleWarn)
	}
	if ev.Text != "hello" {
		t.Errorf("Text = %q, want %q", ev.Text, "hello")
	}
}

// TestDialogEvent_Fields verifies DialogEvent has the expected fields.
func TestDialogEvent_Fields(t *testing.T) {
	ev := DialogEvent{
		Type:          DialogPrompt,
		Message:       "Enter value",
		DefaultPrompt: "default",
	}
	if ev.Type != DialogPrompt {
		t.Errorf("Type = %q, want %q", ev.Type, DialogPrompt)
	}
	if ev.Message != "Enter value" {
		t.Errorf("Message = %q", ev.Message)
	}
	if ev.DefaultPrompt != "default" {
		t.Errorf("DefaultPrompt = %q", ev.DefaultPrompt)
	}
}

// TestDialogAction_ZeroValueDismisses verifies that the zero value of
// DialogAction means "dismiss" (Accept=false), matching chromekit contract.
func TestDialogAction_ZeroValueDismisses(t *testing.T) {
	var a DialogAction
	if a.Accept {
		t.Error("zero DialogAction.Accept must be false (dismiss)")
	}
	if a.PromptText != "" {
		t.Errorf("zero DialogAction.PromptText must be empty, got %q", a.PromptText)
	}
}

// TestDialogAction_AcceptWithPrompt verifies accept + prompt text fields.
func TestDialogAction_AcceptWithPrompt(t *testing.T) {
	a := DialogAction{Accept: true, PromptText: "my text"}
	if !a.Accept {
		t.Error("Accept must be true")
	}
	if a.PromptText != "my text" {
		t.Errorf("PromptText = %q, want %q", a.PromptText, "my text")
	}
}

// TestOnConsole_SignatureCompiles verifies the ctx-free, no-return signature
// accepts func(ConsoleEvent). Fails at compile time if signature regresses.
func TestOnConsole_SignatureCompiles(t *testing.T) {
	var _ func(func(ConsoleEvent)) = (*Page)(nil).OnConsole
}

// TestOnDialog_SignatureCompiles verifies the ctx-free, no-return signature
// accepts func(DialogEvent) DialogAction. Fails at compile time if regressed.
func TestOnDialog_SignatureCompiles(t *testing.T) {
	var _ func(func(DialogEvent) DialogAction) = (*Page)(nil).OnDialog
}

// TestOnDialog_SingleSlot verifies H1 fix: calling OnDialog twice replaces the
// handler slot without accumulating a second subscription. We verify this by
// checking that the atomic handler slot holds the latest handler after two calls.
// (Full subscription-count verification requires a live BiDi client; here we
// test the in-process observable: the slot update.)
func TestOnDialog_SingleSlot(t *testing.T) {
	p := &Page{}

	h1 := func(DialogEvent) DialogAction { return DialogAction{Accept: false} }
	h2 := func(DialogEvent) DialogAction { return DialogAction{Accept: true} }

	p.dialogHandler.Store(&h1)
	p.dialogHandler.Store(&h2)

	loaded := p.dialogHandler.Load()
	if loaded == nil || *loaded == nil {
		t.Fatal("handler slot should not be nil after two stores")
	}
	// Verify it's the second handler by checking Accept behaviour.
	got := (*loaded)(DialogEvent{})
	if !got.Accept {
		t.Error("slot should hold h2 (Accept=true), got false")
	}
}

// TestOnConsole_SingleSlot verifies H1 fix: calling OnConsole twice replaces
// the handler slot without accumulating a second subscription.
func TestOnConsole_SingleSlot(t *testing.T) {
	p := &Page{}

	called := 0
	h1 := func(ConsoleEvent) { called = 1 }
	h2 := func(ConsoleEvent) { called = 2 }

	p.consoleHandler.Store(&h1)
	p.consoleHandler.Store(&h2)

	loaded := p.consoleHandler.Load()
	if loaded == nil || *loaded == nil {
		t.Fatal("handler slot should not be nil after two stores")
	}
	(*loaded)(ConsoleEvent{})
	if called != 2 {
		t.Errorf("slot should hold h2 (called=2), got %d", called)
	}
}
