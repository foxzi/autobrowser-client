package scenario

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

func intOf(f float64) (int, string) {
	if f != math.Trunc(f) || math.IsInf(f, 0) {
		return 0, "Input should be a valid integer, got a number with a fractional part"
	}
	return int(f), ""
}

func readData(path string) (*omap.Map, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, apperr.ScriptValidation("Script file does not exist: " + path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data any
	switch filepath.Ext(path) {
	case ".yaml", ".yml":
		if data, err = omap.ParseYAML(raw); err != nil {
			return nil, apperr.ScriptValidation("Invalid YAML in " + path + ": " + err.Error())
		}
		if !isTruthy(data) {
			data = omap.New()
		}
	case ".json":
		if data, err = omap.ParseJSON(raw); err != nil {
			return nil, apperr.ScriptValidation("Invalid JSON in " + path + ": " + err.Error())
		}
	default:
		return nil, apperr.ScriptValidation("Unsupported script format: " + filepath.Ext(path))
	}
	m, ok := data.(*omap.Map)
	if !ok {
		return nil, apperr.ScriptValidation("Script root must be an object")
	}
	return m, nil
}

// resolve is Path.resolve(): absolute, symlinks followed as far as the path exists.
func resolve(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	rest := ""
	cur := abs
	for {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

func loadInclude(item *omap.Map, baseDir string, stack []string, vars *omap.Map) ([]any, error) {
	target, _ := item.Get("include")
	if item.Len() != 1 {
		return nil, apperr.ScriptValidation("'include' step takes no other keys")
	}
	name, ok := target.(string)
	if !ok || name == "" {
		return nil, apperr.ScriptValidation("'include' expects a file path, got: " + omap.Repr(target))
	}
	if filepath.IsAbs(name) {
		return nil, apperr.ScriptValidation("Absolute include paths are not allowed: " + name)
	}
	path := resolve(filepath.Join(baseDir, name))
	if rel, err := filepath.Rel(resolve(baseDir), path); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, apperr.ScriptValidation("Include path escapes the scenario directory: " + name)
	}
	for _, s := range stack {
		if s == path {
			names := make([]string, 0, len(stack)+1)
			for _, p := range append(append([]string(nil), stack...), path) {
				names = append(names, filepath.Base(p))
			}
			return nil, apperr.ScriptValidation("Include cycle: " + strings.Join(names, " -> "))
		}
	}

	data, err := readData(path)
	if err != nil {
		return nil, err
	}
	if iv, ok := data.Get("vars"); ok {
		if m, ok := iv.(*omap.Map); ok {
			for _, k := range m.Keys() {
				if !vars.Has(k) {
					v, _ := m.Get(k)
					vars.Set(k, v)
				}
			}
		}
	}
	steps, _ := data.Get("steps")
	if !isTruthy(steps) {
		return nil, nil
	}
	list, ok := steps.([]any)
	if !ok {
		return nil, apperr.ScriptValidation("'steps' of an included file must be a list: " + name)
	}
	return expandIncludes(list, filepath.Dir(path), append(append([]string(nil), stack...), path), vars)
}

var blockBodies = []string{"then", "else", "try", "catch", "finally", "do"}

// expandIncludes replaces include steps with the steps of the included file (recursively).
func expandIncludes(steps []any, baseDir string, stack []string, vars *omap.Map) ([]any, error) {
	result := make([]any, 0, len(steps))
	for _, item := range steps {
		if m, ok := item.(*omap.Map); ok {
			if m.Has("include") {
				included, err := loadInclude(m, baseDir, stack, vars)
				if err != nil {
					return nil, err
				}
				result = append(result, included...)
				continue
			}
			for _, key := range blockBodies {
				if body, ok := m.Get(key); ok {
					if list, ok := body.([]any); ok {
						expanded, err := expandIncludes(list, baseDir, stack, vars)
						if err != nil {
							return nil, err
						}
						m.Set(key, expanded)
					}
				}
			}
		}
		result = append(result, item)
	}
	return result, nil
}

func build(data *omap.Map, baseDir string, stack []string) (*Script, error) {
	if steps, ok := data.Get("steps"); ok {
		if list, ok := steps.([]any); ok {
			vars := omap.New()
			if v, ok := data.Get("vars"); ok {
				if m, ok := v.(*omap.Map); ok {
					vars = m
				}
			} else {
				data.Set("vars", vars)
			}
			expanded, err := expandIncludes(list, baseDir, stack, vars)
			if err != nil {
				return nil, err
			}
			data.Set("steps", expanded)
		}
	}
	sc, err := buildScript(data)
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return nil, apperr.ScriptValidation(ve.Error())
		}
		return nil, err
	}
	return sc, nil
}

// Load reads and validates a YAML/JSON scenario file, expanding includes.
func Load(path string) (*Script, error) {
	data, err := readData(path)
	if err != nil {
		return nil, err
	}
	resolved := resolve(path)
	return build(data, filepath.Dir(resolved), []string{resolved})
}
