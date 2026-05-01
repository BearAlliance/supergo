package supergo

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type bodyMatcher interface {
	match(body []byte) error
}

// exactMatcher checks for exact string equality (trimmed).
type exactMatcher struct{ expected string }

func (m exactMatcher) match(body []byte) error {
	got := strings.TrimSpace(string(body))
	want := strings.TrimSpace(m.expected)
	if got != want {
		return fmt.Errorf("body mismatch\nexpected: %s\n     got: %s", want, got)
	}
	return nil
}

// containsMatcher checks that the body contains a substring.
type containsMatcher struct{ substr string }

func (m containsMatcher) match(body []byte) error {
	if !strings.Contains(string(body), m.substr) {
		return fmt.Errorf("body does not contain %q\nbody: %s", m.substr, string(body))
	}
	return nil
}

// jsonDeepEqualMatcher unmarshals both sides and compares with reflect.DeepEqual.
type jsonDeepEqualMatcher struct{ expected interface{} }

func (m jsonDeepEqualMatcher) match(body []byte) error {
	var actual interface{}
	if err := json.Unmarshal(body, &actual); err != nil {
		return fmt.Errorf("response body is not valid JSON: %v\nbody: %s", err, string(body))
	}
	// Normalize expected through JSON round-trip so types match.
	b, _ := json.Marshal(m.expected)
	var normalized interface{}
	json.Unmarshal(b, &normalized) //nolint:errcheck
	if !reflect.DeepEqual(normalized, actual) {
		return fmt.Errorf("body JSON mismatch\nexpected: %s\n     got: %s", string(b), string(body))
	}
	return nil
}

// jsonContainsMatcher checks that all keys/values in expected appear in actual
// (subset check — extra keys in actual are allowed).
type jsonContainsMatcher struct{ expected interface{} }

func (m jsonContainsMatcher) match(body []byte) error {
	var actual interface{}
	if err := json.Unmarshal(body, &actual); err != nil {
		return fmt.Errorf("response body is not valid JSON: %v\nbody: %s", err, string(body))
	}
	// Normalize expected through JSON round-trip.
	b, _ := json.Marshal(m.expected)
	var normalized interface{}
	json.Unmarshal(b, &normalized) //nolint:errcheck
	if err := jsonContains(normalized, actual); err != nil {
		return fmt.Errorf("body JSON subset mismatch: %v\nexpected subset: %s\n           got: %s", err, string(b), string(body))
	}
	return nil
}

// jsonContains recursively checks that every key/value in expected exists in actual.
func jsonContains(expected, actual interface{}) error {
	switch exp := expected.(type) {
	case map[string]interface{}:
		act, ok := actual.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected object, got %T", actual)
		}
		for k, ev := range exp {
			av, exists := act[k]
			if !exists {
				return fmt.Errorf("missing key %q", k)
			}
			if err := jsonContains(ev, av); err != nil {
				return fmt.Errorf("at key %q: %v", k, err)
			}
		}
	case []interface{}:
		act, ok := actual.([]interface{})
		if !ok {
			return fmt.Errorf("expected array, got %T", actual)
		}
		if len(exp) > len(act) {
			return fmt.Errorf("expected array of at least %d elements, got %d", len(exp), len(act))
		}
		for i, ev := range exp {
			if err := jsonContains(ev, act[i]); err != nil {
				return fmt.Errorf("at index %d: %v", i, err)
			}
		}
	default:
		if !reflect.DeepEqual(expected, actual) {
			return fmt.Errorf("expected %v, got %v", expected, actual)
		}
	}
	return nil
}

// dotPathGet traverses a JSON-decoded value using a dot-separated path.
// Integer segments are treated as array indices (e.g. "users.0.name").
func dotPathGet(v interface{}, path string) (interface{}, error) {
	if path == "" {
		return v, nil
	}
	parts := strings.SplitN(path, ".", 2)
	key := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = parts[1]
	}

	switch node := v.(type) {
	case map[string]interface{}:
		val, ok := node[key]
		if !ok {
			return nil, fmt.Errorf("key %q not found", key)
		}
		return dotPathGet(val, rest)
	case []interface{}:
		idx, err := strconv.Atoi(key)
		if err != nil {
			return nil, fmt.Errorf("expected array index, got %q", key)
		}
		if idx < 0 || idx >= len(node) {
			return nil, fmt.Errorf("index %d out of range (len %d)", idx, len(node))
		}
		return dotPathGet(node[idx], rest)
	default:
		if key == "" {
			return v, nil
		}
		return nil, fmt.Errorf("cannot traverse into %T with key %q", v, key)
	}
}
