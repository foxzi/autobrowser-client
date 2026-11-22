// Package engine ports autobrowser_client/browser/engine.py: variables,
// conditions, control blocks and the scenario runner. It talks to the browser
// through the Browser and Page interfaces.
package engine

import (
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/pathutil"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// Page is what the engine needs from one browser tab.
type Page interface {
	// Count is the number of elements matching the locator.
	Count(loc scenario.Locator) (int, error)
	// IsVisible reports whether the first match is visible.
	IsVisible(loc scenario.Locator) (bool, error)
	// InnerText is the text of the body.
	InnerText() (string, error)
	URL() (string, error)
	Title() (string, error)
	// Screenshot writes a PNG to path (parent directory exists).
	Screenshot(path string) error
	Content() (string, error)
}

// Browser is the session the engine drives.
type Browser interface {
	// Page returns a tab by id; an empty id is the current/first tab.
	Page(pageID string) (Page, error)
	// ExecuteStep runs a concrete (substituted) action step; pageID is the
	// scenario default tab, the step's own Page overrides it. It never panics
	// for action failures: they are reported in the result.
	ExecuteStep(step *scenario.Step, pageID string) *StepResult
}

// StepResult is the outcome of one step.
type StepResult struct {
	Action     string
	Success    bool
	DurationMs float64
	// Data is any JSON-able value (use *omap.Map for objects to keep key order); nil is omitted.
	Data any
	// Error is the failure text; an empty text is omitted for successful steps.
	Error string
}

// Map builds the result node: action, success, durationMs, data, error.
func (r *StepResult) Map() *omap.Map {
	m := omap.New("action", r.Action, "success", r.Success, "durationMs", math.Round(r.DurationMs*10)/10)
	if r.Data != nil {
		m.Set("data", r.Data)
	}
	if r.Error != "" || !r.Success {
		m.Set("error", r.Error)
	}
	return m
}

// ValidateOutputPath rejects absolute paths and paths outside the working directory.
func ValidateOutputPath(file string) (string, error) {
	if filepath.IsAbs(file) {
		return "", apperr.Invalid("Absolute paths are not allowed: " + file)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", apperr.Invalid("Invalid file path: " + file)
	}
	rel, err := filepath.Rel(pathutil.Resolve(cwd), pathutil.Resolve(file))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", apperr.Invalid("Path traversal is not allowed: " + file)
	}
	return file, nil
}
