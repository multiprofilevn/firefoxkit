package bidi

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDecodeRemoteValue_Primitives(t *testing.T) {
	cases := []struct {
		raw  string
		want any
	}{
		{`{"type":"string","value":"hi"}`, "hi"},
		{`{"type":"number","value":42}`, 42.0},
		{`{"type":"boolean","value":true}`, true},
		{`{"type":"null"}`, nil},
		{`{"type":"undefined"}`, nil},
		{`{"type":"bigint","value":"9007199254740993"}`, "9007199254740993"},
	}
	for _, c := range cases {
		got, err := DecodeRemoteValue(json.RawMessage(c.raw))
		if err != nil {
			t.Errorf("%s: %v", c.raw, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v (%T), want %v (%T)", c.raw, got, got, c.want, c.want)
		}
	}
}

func TestDecodeRemoteValue_Array(t *testing.T) {
	raw := `{"type":"array","value":[
		{"type":"string","value":"a"},
		{"type":"number","value":1},
		{"type":"boolean","value":false}
	]}`
	got, err := DecodeRemoteValue(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want []any", got)
	}
	if len(arr) != 3 || arr[0] != "a" || arr[1] != 1.0 || arr[2] != false {
		t.Errorf("array contents: %v", arr)
	}
}

func TestDecodeRemoteValue_Object(t *testing.T) {
	raw := `{"type":"object","value":[
		[{"type":"string","value":"x"}, {"type":"number","value":10}],
		[{"type":"string","value":"y"}, {"type":"string","value":"hi"}]
	]}`
	got, err := DecodeRemoteValue(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map[string]any", got)
	}
	if m["x"] != 10.0 || m["y"] != "hi" {
		t.Errorf("object contents: %v", m)
	}
}

func TestDecodeRemoteValue_Node_KeepsHandle(t *testing.T) {
	raw := `{"type":"node","handle":"abc123","value":{"nodeType":1}}`
	got, err := DecodeRemoteValue(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if m["$bidi"] != "node" || m["handle"] != "abc123" {
		t.Errorf("node: %v", m)
	}
}
