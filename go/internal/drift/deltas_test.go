package drift

import (
	"strings"
	"testing"
)

func TestComputeDeltas(t *testing.T) {
	t.Run("nested scalar change", func(t *testing.T) {
		old := map[string]interface{}{"a": map[string]interface{}{"b": "x", "keep": 1}}
		cur := map[string]interface{}{"a": map[string]interface{}{"b": "y", "keep": 1}}
		got := ComputeDeltas(old, cur)
		if len(got) != 1 || got[0].Path != "a.b" || got[0].Old != "x" || got[0].New != "y" {
			t.Errorf("deltas = %+v, want [a.b: x -> y]", got)
		}
	})

	t.Run("added and removed keys", func(t *testing.T) {
		old := map[string]interface{}{"gone": true}
		cur := map[string]interface{}{"fresh": 7}
		got := ComputeDeltas(old, cur)
		if len(got) != 2 {
			t.Fatalf("deltas = %+v, want 2", got)
		}
		// Sorted by path: fresh before gone.
		if got[0].Path != "fresh" || got[0].Old != "(absent)" || got[0].New != "7" {
			t.Errorf("added delta = %+v", got[0])
		}
		if got[1].Path != "gone" || got[1].Old != "true" || got[1].New != "(absent)" {
			t.Errorf("removed delta = %+v", got[1])
		}
	})

	t.Run("equal-length slices compare per index", func(t *testing.T) {
		old := map[string]interface{}{"list": []interface{}{"a", "b"}}
		cur := map[string]interface{}{"list": []interface{}{"a", "c"}}
		got := ComputeDeltas(old, cur)
		if len(got) != 1 || got[0].Path != "list[1]" || got[0].Old != "b" || got[0].New != "c" {
			t.Errorf("deltas = %+v, want [list[1]: b -> c]", got)
		}
	})

	t.Run("length-changed slices summarise as one delta", func(t *testing.T) {
		old := map[string]interface{}{"list": []interface{}{"a"}}
		cur := map[string]interface{}{"list": []interface{}{"a", "b"}}
		got := ComputeDeltas(old, cur)
		if len(got) != 1 || got[0].Path != "list" {
			t.Fatalf("deltas = %+v, want one summary delta at list", got)
		}
		if !strings.Contains(got[0].Old, "(1 items)") || !strings.Contains(got[0].New, "(2 items)") {
			t.Errorf("summary delta = %+v, want item counts", got[0])
		}
	})

	t.Run("long values are truncated", func(t *testing.T) {
		long := strings.Repeat("x", 500)
		old := map[string]interface{}{"v": "short"}
		cur := map[string]interface{}{"v": long}
		got := ComputeDeltas(old, cur)
		if len(got) != 1 {
			t.Fatalf("deltas = %+v, want 1", got)
		}
		if len(got[0].New) > maxDeltaValueLen+len("…") {
			t.Errorf("value not truncated: %d chars", len(got[0].New))
		}
	})

	t.Run("identical maps yield no deltas", func(t *testing.T) {
		data := map[string]interface{}{"a": map[string]interface{}{"b": []interface{}{1, 2}}}
		if got := ComputeDeltas(data, data); len(got) != 0 {
			t.Errorf("deltas = %+v, want none", got)
		}
	})
}
