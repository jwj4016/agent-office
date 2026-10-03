package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ErrRoutePath means the condition's fieldPath could not be resolved.
// The engine fails the condition node instead of guessing a branch.
var ErrRoutePath = errors.New("routing field path not found")

// EvaluateRouting picks the target for a condition from the source JSON
// document: the first matching branch, otherwise DefaultTarget. Values
// are compared as JSON data; nothing is ever executed as code.
func EvaluateRouting(r *Routing, doc []byte) (string, error) {
	var root any
	if err := json.Unmarshal(doc, &root); err != nil {
		return "", fmt.Errorf("routing source is not JSON: %w", err)
	}
	val, found := lookup(root, r.Source.FieldPath)
	for _, b := range r.Branches {
		if b.Operator == OpExists {
			if found {
				return b.TargetStep, nil
			}
			continue
		}
		if !found {
			return "", fmt.Errorf("%w: %q", ErrRoutePath, r.Source.FieldPath)
		}
		var want any
		if err := json.Unmarshal(b.Value, &want); err != nil {
			return "", fmt.Errorf("branch value: %w", err)
		}
		switch b.Operator {
		case OpEq:
			if reflect.DeepEqual(val, want) {
				return b.TargetStep, nil
			}
		case OpNe:
			if !reflect.DeepEqual(val, want) {
				return b.TargetStep, nil
			}
		case OpIn:
			list, ok := want.([]any)
			if !ok {
				return "", fmt.Errorf("in value must be an array")
			}
			for _, x := range list {
				if reflect.DeepEqual(val, x) {
					return b.TargetStep, nil
				}
			}
		default:
			return "", fmt.Errorf("unsupported operator %q", b.Operator)
		}
	}
	return r.DefaultTarget, nil
}

// lookup resolves a dot path; numeric segments index arrays.
func lookup(v any, path string) (any, bool) {
	for _, seg := range strings.Split(path, ".") {
		switch cur := v.(type) {
		case map[string]any:
			next, ok := cur[seg]
			if !ok {
				return nil, false
			}
			v = next
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(cur) {
				return nil, false
			}
			v = cur[i]
		default:
			return nil, false
		}
	}
	return v, true
}
