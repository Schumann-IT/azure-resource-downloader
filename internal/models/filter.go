package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// PropertyFilter matches a single resource property value against a compiled
// regular expression. The property name is a dot-separated path into the
// resource's properties (e.g. "displayName" or "properties.subnet.id").
type PropertyFilter struct {
	Property string
	Pattern  *regexp.Regexp
}

// ResourceFilter holds the property filters configured for one resource type.
// A resource is kept only when every property filter matches (logical AND).
type ResourceFilter struct {
	ResourceType string
	Properties   []PropertyFilter
}

// ParseResourceFilters builds resource filters from a raw configuration map of
// the form resourceType -> {propertyPath -> regex}. It returns every filter it
// could parse together with an aggregated error describing entries that were
// skipped because of an invalid type or regular expression, so callers can warn
// and continue with the valid filters.
func ParseResourceFilters(raw map[string]interface{}) ([]ResourceFilter, error) {
	var filters []ResourceFilter
	var errs []error

	for resourceType, rawProps := range raw {
		propMap, ok := rawProps.(map[string]interface{})
		if !ok {
			errs = append(errs, fmt.Errorf("filter for %q must be a map of property to regex", resourceType))
			continue
		}

		var props []PropertyFilter
		for property, rawPattern := range propMap {
			pattern, ok := rawPattern.(string)
			if !ok {
				errs = append(errs, fmt.Errorf("filter %s.%s must be a string regex", resourceType, property))
				continue
			}

			re, err := regexp.Compile(pattern)
			if err != nil {
				errs = append(errs, fmt.Errorf("invalid regex for %s.%s (%q): %w", resourceType, property, pattern, err))
				continue
			}

			props = append(props, PropertyFilter{Property: property, Pattern: re})
		}

		if len(props) > 0 {
			filters = append(filters, ResourceFilter{ResourceType: resourceType, Properties: props})
		}
	}

	if len(errs) > 0 {
		return filters, errors.Join(errs...)
	}
	return filters, nil
}

// GetResourceFilter returns the filter configured for the given resource type
// (case-insensitive), or nil when no filter applies.
func GetResourceFilter(filters []ResourceFilter, resourceType string) *ResourceFilter {
	for i := range filters {
		if strings.EqualFold(filters[i].ResourceType, resourceType) {
			return &filters[i]
		}
	}
	return nil
}

// Matches reports whether the given resource properties satisfy every property
// filter. A missing property or a value that does not match its pattern
// excludes the resource.
func (f *ResourceFilter) Matches(properties map[string]interface{}) bool {
	for _, pf := range f.Properties {
		value, ok := lookupProperty(properties, pf.Property)
		if !ok {
			return false
		}
		if !pf.Pattern.MatchString(fmt.Sprintf("%v", value)) {
			return false
		}
	}
	return true
}

// lookupProperty resolves a dot-separated path within a properties map. Path
// segments are matched case-insensitively against the map keys.
func lookupProperty(properties map[string]interface{}, path string) (interface{}, bool) {
	var current interface{} = properties
	for _, segment := range strings.Split(path, ".") {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		value, found := lookupKey(m, segment)
		if !found {
			return nil, false
		}
		current = value
	}
	return current, true
}

// lookupKey looks up a key in a map, preferring an exact match and falling back
// to a case-insensitive match.
func lookupKey(m map[string]interface{}, key string) (interface{}, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}
