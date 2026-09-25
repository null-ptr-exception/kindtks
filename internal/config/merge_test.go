package config

import (
	"reflect"
	"testing"
)

func TestMergeValues(t *testing.T) {
	base := map[string]any{
		"images":  map[string]any{"a": "base-a", "b": "base-b"},
		"gateway": map[string]any{"extraHosts": []any{"x"}},
		"keep":    "k",
	}
	override := map[string]any{
		"images":  map[string]any{"a": "over-a"},
		"gateway": map[string]any{"extraHosts": []any{"y", "z"}},
		"new":     true,
	}

	got := MergeValues(base, override)

	want := map[string]any{
		"images":  map[string]any{"a": "over-a", "b": "base-b"},
		"gateway": map[string]any{"extraHosts": []any{"y", "z"}},
		"keep":    "k",
		"new":     true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
	if base["images"].(map[string]any)["a"] != "base-a" {
		t.Error("MergeValues must not modify base")
	}
}

func TestMergeValues_ScalarReplacesObject(t *testing.T) {
	got := MergeValues(map[string]any{"a": map[string]any{"b": 1}}, map[string]any{"a": nil})
	if got.(map[string]any)["a"] != nil {
		t.Errorf("want nil, got %#v", got)
	}
}
