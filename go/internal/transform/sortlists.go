package transform

import "sort"

// SortScalarSlices recursively sorts, in place, every slice within data whose
// elements are all strings, and recurses into nested maps and slices. Slices
// that contain maps or nested slices are left in their original order because
// the order of structured elements (e.g. assignments, settings) can be
// significant.
//
// It canonicalizes the order of multivalued scalar attributes that some APIs
// return unstably between reads — notably Microsoft Graph's proxyAddresses — so
// a resource serializes to identical bytes across runs. This is the array
// analogue of the YAML marshaller already emitting map keys in sorted order,
// and is applied unconditionally so output is deterministic regardless of the
// configured transformer pipeline.
func SortScalarSlices(data map[string]interface{}) {
	for _, v := range data {
		sortScalarSlicesValue(v)
	}
}

// sortScalarSlicesValue walks a single value, recursing into maps and slices
// and sorting any slice whose elements are all strings.
func sortScalarSlicesValue(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for _, val := range t {
			sortScalarSlicesValue(val)
		}
	case []interface{}:
		for _, e := range t {
			sortScalarSlicesValue(e)
		}
		sortIfAllStrings(t)
	}
}

// sortIfAllStrings sorts s lexically in place only when every element is a
// string; otherwise it leaves s untouched.
func sortIfAllStrings(s []interface{}) {
	for _, e := range s {
		if _, ok := e.(string); !ok {
			return
		}
	}
	sort.Slice(s, func(i, j int) bool {
		return s[i].(string) < s[j].(string)
	})
}
