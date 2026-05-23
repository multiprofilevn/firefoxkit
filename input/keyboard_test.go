package input

import (
	"testing"
)

func TestVKValue_KnownKeys(t *testing.T) {
	cases := []struct {
		vk   uint16
		want string
	}{
		{VKReturn, KeyEnter},
		{VKBackspace, KeyBackspace},
		{VKEscape, KeyEscape},
		{VKTab, KeyTab},
		{VKLeft, KeyArrowLeft},
		{VKA, "a"},
		{VKL, "l"},
		{VKT, "t"},
	}
	for _, c := range cases {
		got, ok := VKValue(c.vk)
		if !ok {
			t.Errorf("VKValue(0x%X) missing", c.vk)
			continue
		}
		if got != c.want {
			t.Errorf("VKValue(0x%X) = %q, want %q", c.vk, got, c.want)
		}
	}
}

func TestVKValue_Unknown(t *testing.T) {
	if _, ok := VKValue(0xFFFF); ok {
		t.Error("expected miss on unknown VK")
	}
}

func TestModifierKeys_Order(t *testing.T) {
	got := ModifierKeys(ModCtrl | ModShift | ModAlt | ModWin)
	if len(got) != 4 {
		t.Fatalf("len=%d, want 4", len(got))
	}
	if got[0] != KeyControl || got[1] != KeyShift || got[2] != KeyAlt || got[3] != KeyMeta {
		t.Errorf("unexpected order: %v", got)
	}
}

func TestModifierKeys_Subset(t *testing.T) {
	got := ModifierKeys(ModCtrl | ModShift)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
}

func TestModifierKeys_None(t *testing.T) {
	if len(ModifierKeys(0)) != 0 {
		t.Error("expected empty slice for mods=0")
	}
}

func TestPickTypo(t *testing.T) {
	got, ok := pickTypo('a', func(n int) int { return 0 })
	if !ok {
		t.Fatal("expected typo for 'a'")
	}
	// First neighbor of 'a' is 'q'.
	if got != 'q' {
		t.Errorf("got %q, want %q", got, 'q')
	}
}

func TestPickTypo_Uppercase(t *testing.T) {
	got, ok := pickTypo('A', func(n int) int { return 0 })
	if !ok {
		t.Fatal("expected typo for 'A'")
	}
	if got != 'Q' {
		t.Errorf("got %q, want %q", got, 'Q')
	}
}

func TestPickTypo_NoNeighbors(t *testing.T) {
	if _, ok := pickTypo('1', func(int) int { return 0 }); ok {
		t.Error("expected miss for digit")
	}
}

func TestHumanDelay_Bounds(t *testing.T) {
	got := humanDelay(80, 0, func(int) int { return 0 })
	// max <= min → return min
	if got != 80 {
		t.Errorf("got %v", got)
	}
}
