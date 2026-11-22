package scenario

import (
	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// Parse builds a scenario from an object (*omap.Map) or YAML text
// (parse_script); include paths are relative to baseDir.
func Parse(data any, baseDir string) (*Script, error) {
	if text, ok := data.(string); ok {
		parsed, err := omap.ParseYAML([]byte(text))
		if err != nil {
			return nil, apperr.ScriptValidation("Invalid YAML: " + err.Error())
		}
		if !isTruthy(parsed) {
			parsed = omap.New()
		}
		data = parsed
	}
	m, ok := data.(*omap.Map)
	if !ok {
		return nil, apperr.ScriptValidation("Script root must be an object")
	}
	return build(m, baseDir, nil)
}
