package input

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestKeyboard_Type_EmitsPerChar(t *testing.T) {
	fb := &fakeBackend{}
	k := NewKeyboard(fb)
	k.MinDelay = 0
	k.MaxDelay = 0
	if err := k.Type(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	got := fb.trace()
	for _, want := range []string{`KeyChar("a")`, `KeyChar("b")`, `KeyChar("c")`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestKeyboard_TypeFast_BulkInsert(t *testing.T) {
	fb := &fakeBackend{}
	k := NewKeyboard(fb)
	if err := k.TypeFast(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fb.trace(), `InsertText("hello")`) {
		t.Errorf("expected InsertText: %s", fb.trace())
	}
}

func TestKeyboard_Combo(t *testing.T) {
	fb := &fakeBackend{}
	k := NewKeyboard(fb)
	if err := k.Combo(context.Background(), VKA, ModCtrl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fb.trace(), "KeyCombo(vk=0x41,mods=1)") {
		t.Errorf("trace: %s", fb.trace())
	}
}

func TestKeyboard_ClearInput_SelectAllThenDelete(t *testing.T) {
	fb := &fakeBackend{}
	k := NewKeyboard(fb)
	if err := k.ClearInput(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := fb.trace()
	if !strings.Contains(got, "KeyCombo(vk=0x41,mods=1)") {
		t.Errorf("missing Ctrl+A: %s", got)
	}
	if !strings.Contains(got, "KeyCombo(vk=0x2E,mods=0)") {
		t.Errorf("missing Delete: %s", got)
	}
}

func TestKeyboard_Type_ContextCancel(t *testing.T) {
	fb := &fakeBackend{}
	k := NewKeyboard(fb)
	k.MinDelay = 0
	k.MaxDelay = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled
	err := k.Type(ctx, "abc")
	if err == nil {
		t.Fatal("expected ctx error")
	}
}

func TestTypeHuman_PreservesText(t *testing.T) {
	fb := &fakeBackend{}
	k := NewKeyboard(fb)
	cfg := HumanTypeConfig{
		MinDelay:    0,
		MaxDelay:    0,
		MistakeRate: 0, // disable typos for deterministic test
		Rand:        rand.New(rand.NewSource(42)),
	}
	if err := k.TypeHuman(context.Background(), "hi", cfg); err != nil {
		t.Fatal(err)
	}
	got := fb.trace()
	if !strings.Contains(got, `KeyChar("h")`) || !strings.Contains(got, `KeyChar("i")`) {
		t.Errorf("missing chars: %s", got)
	}
}

func TestTypeHuman_TypoBackspaceCycle(t *testing.T) {
	fb := &fakeBackend{}
	k := NewKeyboard(fb)
	cfg := HumanTypeConfig{
		MinDelay:        0,
		MaxDelay:        0,
		MistakeRate:     1.0, // force typo on every char
		CorrectionPause: 1 * time.Millisecond,
		Rand:            rand.New(rand.NewSource(1)),
	}
	if err := k.TypeHuman(context.Background(), "a", cfg); err != nil {
		t.Fatal(err)
	}
	got := fb.trace()
	// Expect: wrong char + Backspace combo + correct char
	if !strings.Contains(got, "KeyCombo(vk=0x8,mods=0)") {
		t.Errorf("missing backspace correction in:\n%s", got)
	}
	if !strings.Contains(got, `KeyChar("a")`) {
		t.Errorf("missing correct char in:\n%s", got)
	}
}

func TestHumanTypeConfig_Defaults(t *testing.T) {
	c := HumanTypeConfig{}.withDefaults()
	if c.MinDelay <= 0 || c.MaxDelay <= 0 || c.CorrectionPause <= 0 {
		t.Errorf("defaults not applied: %+v", c)
	}
	if c.MaxDelay < c.MinDelay {
		t.Errorf("max < min: %+v", c)
	}
}

func TestHumanTypeConfig_ClampsMistakeRate(t *testing.T) {
	c := HumanTypeConfig{MistakeRate: 1.5}.withDefaults()
	if c.MistakeRate > 1 {
		t.Errorf("MistakeRate not clamped: %v", c.MistakeRate)
	}
	c2 := HumanTypeConfig{MistakeRate: -0.1}.withDefaults()
	if c2.MistakeRate < 0 {
		t.Errorf("MistakeRate not clamped: %v", c2.MistakeRate)
	}
}
