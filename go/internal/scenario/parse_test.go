package scenario

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

func TestParseDictTextAndBaseDir(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "inc.yaml"), "steps: [{open: inc}]")
	obj, _ := omap.ParseJSON([]byte(`{"steps": [{"open": "a"}]}`))
	s, err := Parse(obj, dir)
	if err != nil || s.Steps[0].(*Step).URL != "a" {
		t.Fatalf("dict: %v %v", s, err)
	}
	s, err = Parse("steps:\n  - include: inc.yaml\n", dir)
	if err != nil || s.Steps[0].(*Step).URL != "inc" {
		t.Fatalf("text: %v %v", s, err)
	}
	bad, _ := omap.ParseJSON([]byte(`{"steps": [{"action": "open"}]}`))
	for _, data := range []any{"- just a list", "steps: [", bad} {
		var ae *apperr.Error
		if _, err := Parse(data, dir); !errors.As(err, &ae) || ae.Code != apperr.ScriptValidationErr {
			t.Errorf("%v: %v", data, err)
		}
	}
}
