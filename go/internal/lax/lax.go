// Package lax holds pydantic-style lax scalar coercions shared by config and scenario.
package lax

import (
	"math"
	"strconv"
	"strings"
)

// Bool converts bool, 0/1 and the strings pydantic accepts ("yes", "off", ...).
func Bool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case int:
		if t == 0 || t == 1 {
			return t == 1, true
		}
	case float64:
		if t == 0 || t == 1 {
			return t == 1, true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "0", "off", "f", "false", "n", "no":
			return false, true
		case "1", "on", "t", "true", "y", "yes":
			return true, true
		}
	}
	return false, false
}

// Int converts int, integral floats and numeric strings; msg is the error text when it fails.
func Int(v any) (n int, msg string) {
	switch t := v.(type) {
	case int:
		return t, ""
	case float64:
		if t == math.Trunc(t) && !math.IsInf(t, 0) {
			return int(t), ""
		}
		return 0, "Input should be a valid integer, got a number with a fractional part"
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return i, ""
		}
		return 0, "Input should be a valid integer, unable to parse string as an integer"
	}
	return 0, "Input should be a valid integer"
}

// Float converts int, float64 and numeric strings.
func Float(v any) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case float64:
		return t, true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}
