package drift

import (
	"fmt"
	"reflect"
	"sort"
)

// maxDeltaValueLen bounds a rendered delta value: the deltas are a summary
// tier, and whole-document output belongs to --log-level debug, not here.
const maxDeltaValueLen = 120

// ComputeDeltas walks two cleaned-data maps and returns the dotted-path
// old → new field changes between them, sorted by path. Values are rendered
// with %v and truncated; a key present on one side only renders the other side
// as "(absent)". Slices of equal length are compared per index ([i] paths);
// slices whose length changed are summarised as one delta.
func ComputeDeltas(old, current map[string]interface{}) []Delta {
	var deltas []Delta
	diffValue("", old, current, &deltas)
	sort.Slice(deltas, func(i, j int) bool { return deltas[i].Path < deltas[j].Path })
	return deltas
}

// diffValue appends the deltas between two values at the given dotted path.
func diffValue(path string, old, current interface{}, deltas *[]Delta) {
	oldMap, oldIsMap := old.(map[string]interface{})
	curMap, curIsMap := current.(map[string]interface{})
	if oldIsMap && curIsMap {
		diffMaps(path, oldMap, curMap, deltas)
		return
	}

	oldSlice, oldIsSlice := old.([]interface{})
	curSlice, curIsSlice := current.([]interface{})
	if oldIsSlice && curIsSlice {
		if len(oldSlice) != len(curSlice) {
			*deltas = append(*deltas, Delta{
				Path: path,
				Old:  fmt.Sprintf("(%d items) %s", len(oldSlice), renderValue(oldSlice)),
				New:  fmt.Sprintf("(%d items) %s", len(curSlice), renderValue(curSlice)),
			})
			return
		}
		for i := range oldSlice {
			diffValue(fmt.Sprintf("%s[%d]", path, i), oldSlice[i], curSlice[i], deltas)
		}
		return
	}

	if !reflect.DeepEqual(old, current) {
		*deltas = append(*deltas, Delta{Path: path, Old: renderValue(old), New: renderValue(current)})
	}
}

// diffMaps appends the deltas over the union of both maps' keys.
func diffMaps(path string, old, current map[string]interface{}, deltas *[]Delta) {
	keys := map[string]bool{}
	for k := range old {
		keys[k] = true
	}
	for k := range current {
		keys[k] = true
	}
	for k := range keys {
		childPath := k
		if path != "" {
			childPath = path + "." + k
		}
		oldVal, inOld := old[k]
		curVal, inCur := current[k]
		switch {
		case inOld && !inCur:
			*deltas = append(*deltas, Delta{Path: childPath, Old: renderValue(oldVal), New: "(absent)"})
		case !inOld && inCur:
			*deltas = append(*deltas, Delta{Path: childPath, Old: "(absent)", New: renderValue(curVal)})
		default:
			diffValue(childPath, oldVal, curVal, deltas)
		}
	}
}

// renderValue renders a value for a delta, truncated to maxDeltaValueLen.
func renderValue(v interface{}) string {
	s := fmt.Sprintf("%v", v)
	if len(s) > maxDeltaValueLen {
		return s[:maxDeltaValueLen] + "…"
	}
	return s
}
