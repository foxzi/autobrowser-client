package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

func write(t *testing.T, path, text string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadText(t *testing.T, text string) (*Script, error) {
	t.Helper()
	return Load(write(t, filepath.Join(t.TempDir(), "s.yaml"), text))
}

func mustStep(t *testing.T, text string) *Step {
	t.Helper()
	v, err := omap.ParseYAML([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseStep(v)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return s
}

func stepErr(t *testing.T, text string) error {
	t.Helper()
	v, _ := omap.ParseYAML([]byte(text))
	_, err := ParseStep(v)
	if err == nil {
		t.Fatalf("expected error for %s", text)
	}
	return err
}

func TestMinimalAndDefaults(t *testing.T) {
	s, err := loadText(t, "steps:\n  - action: open\n    url: https://example.com\n")
	if err != nil {
		t.Fatal(err)
	}
	st := s.Steps[0].(*Step)
	if len(s.Steps) != 1 || st.Action != "open" || st.URL != "https://example.com" || !s.StopOnError ||
		!s.Session.Headless || st.RetryDelay != 1000 || st.Retry != 0 || s.Vars.Len() != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestFullScript(t *testing.T) {
	s, err := loadText(t, `
session: {headless: false, profile: test, viewport: {width: 800, height: 600}, fingerprint: {seed: 5}, timeout: 10}
output: {file: out.json}
stopOnError: false
errorArtifacts: art
vars: {a: 1}
steps:
  - {action: open, url: "https://example.com"}
  - {action: wait, timeout: 1000}
  - {action: find, locator: {type: css, value: h1}}
  - {action: fill, locator: {type: name, value: q}, value: hello}
  - {action: scroll, deltaY: 100}
  - {action: save, file: out.json}
`)
	if err != nil {
		t.Fatal(err)
	}
	if s.StopOnError || s.Session.Headless || s.Session.Profile != "test" || s.Session.Viewport.Width != 800 ||
		*s.Session.Timeout != 10 || s.OutputFile != "out.json" || s.ErrorArtifacts != "art" || len(s.Steps) != 6 {
		t.Fatalf("%+v", s)
	}
	if fp, ok := s.Session.Fingerprint.(*FingerprintOverrides); !ok || *fp.Seed != 5 {
		t.Fatal("fingerprint")
	}
}

func TestLoadErrors(t *testing.T) {
	bad := []string{
		"steps: [{action: open}]",
		"steps: [{action: click}]",
		"steps: [{action: fill, locator: '#i'}]",
		"steps: [{action: press}]",
		"steps: [{action: scroll}]",
		"steps: [{action: save}]",
		"steps: [{action: fly}]",
		"steps: [{action: find, locator: {type: regex, value: x}}]",
		"steps: [{action: open, url: x}]\nextra: true",
		"{}",
		"steps: [{bogus: x}]",
		"steps: [{action: open, url: x, retry: -1}]",
		"steps: [{action: 'null'}]\nvars: null",
		"session: null\nsteps: []",
	}
	for _, text := range bad {
		_, err := loadText(t, text)
		e, ok := err.(*apperr.Error)
		if !ok || e.Code != apperr.ScriptValidationErr {
			t.Errorf("%q: expected ScriptValidationError, got %v", text, err)
		}
	}
	_, err := loadText(t, "steps: [{action: open}]")
	if !strings.Contains(err.Error(), "'open' step requires 'url'") {
		t.Fatal(err)
	}
}

func TestFileErrors(t *testing.T) {
	if _, err := Load("/nonexistent/script.yaml"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := Load(write(t, filepath.Join(dir, "s.txt"), "hello")); err == nil || !strings.Contains(err.Error(), "Unsupported") {
		t.Fatal(err)
	}
	if _, err := Load(write(t, filepath.Join(dir, "s.yaml"), "- a\n- b\n")); err == nil || !strings.Contains(err.Error(), "object") {
		t.Fatal(err)
	}
	s, err := Load(write(t, filepath.Join(dir, "s.json"), `{"steps": [{"open": "u"}]}`))
	if err != nil || s.Steps[0].(*Step).URL != "u" {
		t.Fatal(err)
	}
}

func TestNewActionValidation(t *testing.T) {
	for _, text := range []string{
		"{action: hover}", "{action: extract}", "{action: select, locator: '#s'}",
		"{action: select, locator: '#s', value: a, options: [b]}", "{action: upload, locator: '#f'}",
		"{action: evaluate}", "{action: history}", "{action: history, direction: sideways}",
		"{action: wait-text}", "{action: mouse-move, x: 1}", "{action: mouse-random, endX: 1}",
		"{action: wait, waitUntil: visible}", "{action: set}", "{action: fail}", "{action: assert}",
		"{action: click, locator: a, as: x}", "{action: extract, locator: a, as: 1bad}",
		"{action: set, vars: {}}",
	} {
		stepErr(t, text)
	}
	s := mustStep(t, "{action: extract, locator: a, attribute: href, all: true}")
	if !s.All || s.Attribute != "href" {
		t.Fatal("extract fields")
	}
	s = mustStep(t, "{action: mouse-random, startX: 1, startY: 2, endX: 3, endY: 4, durationMs: 10}")
	if s.StartX != 1 || *s.EndY != 4 || *s.DurationMs != 10 {
		t.Fatal("mouse-random fields")
	}
}

func TestLocatorString(t *testing.T) {
	cases := map[string][2]string{
		"css=button": {"css", "button"}, "xpath=//div": {"xpath", "//div"}, "name=username": {"name", "username"},
		"id=submit": {"id", "submit"}, "class=btn": {"class", "btn"}, "text=Click me": {"text", "Click me"},
		"attribute=data-id": {"attribute", "data-id"}, "//div[@id='x']": {"xpath", "//div[@id='x']"},
		"/html/body": {"xpath", "/html/body"}, "#submit": {"id", "submit"}, ".btn-primary": {"class", "btn-primary"},
		"button.primary": {"css", "button.primary"}, "button[type=submit]": {"css", "button[type=submit]"},
		"#a b": {"css", "#a b"},
	}
	for raw, want := range cases {
		if l := ParseLocatorString(raw); l.Type != want[0] || l.Value != want[1] {
			t.Errorf("%s: %+v", raw, l)
		}
	}
	s := mustStep(t, "{action: click, locator: 'css=button'}")
	if s.Locator.Type != "css" || s.Locator.Value != "button" {
		t.Fatal("step locator")
	}
	s = mustStep(t, "{click: '${sel}'}")
	if !s.Locator.Shorthand || s.Locator.Value != "${sel}" {
		t.Fatal("deferred locator")
	}
	s = mustStep(t, "{action: click, locator: {type: css, value: x}}")
	if s.Locator.Value != "x" {
		t.Fatal("dict locator")
	}
}

func TestShorthandSteps(t *testing.T) {
	if s := mustStep(t, "{open: 'https://example.com'}"); s.Action != "open" || s.URL != "https://example.com" {
		t.Fatal("open")
	}
	if s := mustStep(t, "{wait: 3000}"); *s.Timeout != 3000 {
		t.Fatal("wait int")
	}
	for _, w := range []string{"networkidle", "load", "domcontentloaded"} {
		if s := mustStep(t, "{wait: "+w+"}"); s.WaitUntil != w {
			t.Fatal(w)
		}
	}
	if s := mustStep(t, "{press: Enter}"); s.Key != "Enter" {
		t.Fatal("press")
	}
	if s := mustStep(t, "{screenshot: out.png, fullPage: true}"); s.File != "out.png" || !s.FullPage {
		t.Fatal("screenshot")
	}
	if s := mustStep(t, "{open: x, timeout: 5000}"); *s.Timeout != 5000 {
		t.Fatal("extra field")
	}
	for _, v := range []string{"get-content", "get-cookies", "get-headers", "browse-tabs", "solve-challenge"} {
		if s := mustStep(t, "{"+v+": null}"); s.Action != v {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"hover", "dblclick", "focus", "clear", "check", "uncheck", "submit", "extract", "find-all", "read", "mouse-scroll-to", "click", "find"} {
		if s := mustStep(t, "{"+v+": '#a'}"); s.Action != v || s.Locator.Type != "id" || s.Locator.Value != "a" {
			t.Fatal(v)
		}
	}
	if s := mustStep(t, "{fill: ['name=user', demo]}"); s.Locator.Type != "name" || *s.Value != "demo" {
		t.Fatal("fill")
	}
	if s := mustStep(t, "{type: ['css=input', hello]}"); s.Action != "type" || *s.Value != "hello" {
		t.Fatal("type")
	}
	if s := mustStep(t, "{select: ['#s', b]}"); *s.Value != "b" || s.Options != nil {
		t.Fatal("select value")
	}
	if s := mustStep(t, "{select: ['#s', [a, b]]}"); len(s.Options) != 2 || s.Value != nil {
		t.Fatal("select options")
	}
	if s := mustStep(t, "{upload: ['#f', a.txt]}"); len(s.Files) != 1 {
		t.Fatal("upload")
	}
	if s := mustStep(t, "{upload: ['#f', [a, b]]}"); len(s.Files) != 2 {
		t.Fatal("upload list")
	}
	if s := mustStep(t, "{evaluate: '1+1'}"); s.Script != "1+1" {
		t.Fatal("evaluate")
	}
	if s := mustStep(t, "{history: back}"); s.Direction != "back" {
		t.Fatal("history")
	}
	if s := mustStep(t, "{wait-text: hi}"); s.Text != "hi" {
		t.Fatal("wait-text")
	}
	if s := mustStep(t, "{fail: boom}"); s.Message != "boom" {
		t.Fatal("fail")
	}
	if s := mustStep(t, "{set: {a: 1}}"); s.Vars.Len() != 1 {
		t.Fatal("set")
	}
	if s := mustStep(t, "{assert: {var: a}, message: m}"); s.Condition.Var == nil || s.Message != "m" {
		t.Fatal("assert")
	}
}

func TestShorthandErrors(t *testing.T) {
	for _, text := range []string{
		"{wait: bogus}", "{fill: not-a-list}", "{fill: [only-one]}", "{open: x, url: y}",
		"{get-content: something}", "{select: '#s'}", "{upload: ['#f']}", "{bogus: value}",
	} {
		stepErr(t, text)
	}
	if err := stepErr(t, "{wait: bogus}"); !strings.Contains(err.Error(), "wait shorthand expects int (ms) or load-state string, got: 'bogus'") {
		t.Fatal(err)
	}
	if err := stepErr(t, "{open: x, url: y}"); !strings.Contains(err.Error(), "shorthand conflict: cannot specify both shorthand value and explicit 'url'") {
		t.Fatal(err)
	}
	v, _ := omap.ParseYAML([]byte("{bogus: value}"))
	got, _ := expandShorthand(v.(*omap.Map))
	if got != v.(*omap.Map) {
		t.Fatal("unknown key passthrough")
	}
}

func TestNumbersCoercedToStrings(t *testing.T) {
	s := mustStep(t, "{action: fill, locator: '#q', value: 7}")
	if *s.Value != "7" {
		t.Fatal(*s.Value)
	}
	s = mustStep(t, "{action: fill, locator: '#q', value: 1.5}")
	if *s.Value != "1.5" {
		t.Fatal(*s.Value)
	}
	if err := stepErr(t, "{action: save, file: x, value: {a: 1}}"); !strings.HasPrefix(err.(*ValidationError).Details(), "value: Input should be a valid string") {
		t.Fatal(err)
	}
}

func TestConditionParsing(t *testing.T) {
	for _, text := range []string{
		"{}", "{exists: a, text: b}", "{equals: 1}", "{not: {var: a}, timeout: 5}", "{var: a, bogus: 1}",
		"{contains: x}", "{all: x}",
	} {
		v, _ := omap.ParseYAML([]byte(text))
		if _, err := ParseCondition(v); err == nil {
			t.Errorf("expected error for %s", text)
		}
	}
	v, _ := omap.ParseYAML([]byte("{not: {all: [{var: a, equals: 1}, {exists: '#x'}]}}"))
	c, err := ParseCondition(v)
	if err != nil || c.Not == nil || len(c.Not.All) != 2 || !c.Not.All[0].HasEquals {
		t.Fatal(err)
	}
	v, _ = omap.ParseYAML([]byte("{exists: '${sel}', timeout: 5000}"))
	c, _ = ParseCondition(v)
	if got := omap.Marshal(c.Raw, 0); got != `{"exists": {"type": "css", "value": "${sel}", "shorthand": true}, "timeout": 5000}` {
		t.Fatal(got)
	}
	v, _ = omap.ParseYAML([]byte("{var: a, equals: null}"))
	if c, err = ParseCondition(v); err != nil || !c.HasEquals {
		t.Fatal("equals null counts as set")
	}
}

func TestBlocks(t *testing.T) {
	s, err := loadText(t, `
steps:
  - if: {var: a}
    then: [{open: x}]
    else: []
    name: n
  - try: [{open: x}]
    catch: []
  - forEach: ["a", "b"]
    as: x
    do: [{open: "${x}"}]
  - forEach: "${urls}"
    do: []
  - repeat: 3
    do: []
  - repeat: "${n}"
    do: []
  - while: {var: go}
    max: 5
    do: [{open: x}]
`)
	if err != nil {
		t.Fatal(err)
	}
	ifb := s.Steps[0].(*IfBlock)
	if !ifb.HasElse || len(ifb.Else) != 0 || ifb.Name != "n" || len(ifb.Then) != 1 {
		t.Fatalf("%+v", ifb)
	}
	if tb := s.Steps[1].(*TryBlock); !tb.HasCatch || tb.HasFinally {
		t.Fatal("try")
	}
	if fe := s.Steps[2].(*ForEachBlock); fe.AsName != "x" || len(fe.Items.([]any)) != 2 {
		t.Fatal("forEach")
	}
	if fe := s.Steps[3].(*ForEachBlock); fe.AsName != "item" || fe.Items.(string) != "${urls}" {
		t.Fatal("forEach default")
	}
	if s.Steps[4].(*RepeatBlock).Count.(int) != 3 || s.Steps[5].(*RepeatBlock).Count.(string) != "${n}" {
		t.Fatal("repeat")
	}
	if w := s.Steps[6].(*WhileBlock); w.Max != 5 || w.While.Var == nil {
		t.Fatal("while")
	}
}

func TestBlockValidation(t *testing.T) {
	for _, bad := range []string{
		"{try: []}", "{forEach: [1], as: 1x, do: []}", "{if: {var: a}}", "{while: {var: a}, max: 0, do: []}",
		"{repeat: 2, do: [], bogus: 1}", "{repeat: 1.5, do: []}", "{forEach: 5, do: []}", "{repeat: [1], do: []}",
	} {
		if _, err := loadText(t, "steps: ["+bad+"]"); err == nil {
			t.Errorf("expected error for %s", bad)
		}
	}
	s, err := loadText(t, "steps: [{repeat: 2.0, do: []}]")
	if err != nil || s.Steps[0].(*RepeatBlock).Count.(int) != 2 {
		t.Fatal(err)
	}
}

func TestNestedStepErrorPath(t *testing.T) {
	_, err := loadText(t, "steps:\n  - if: {var: a}\n    then: [{open: x}, {open: ''}]\n")
	if err == nil || !strings.Contains(err.Error(), "steps.0.then.1") {
		t.Fatal(err)
	}
}

// --- includes ---

func TestIncludeFlattensAndMergesVars(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "common", "login.yaml"), "vars: {user: lib, extra: 1}\nsteps:\n  - open: https://x/login\n")
	main := write(t, filepath.Join(dir, "main.yaml"),
		"vars: {user: me}\nsteps:\n  - include: common/login.yaml\n  - try:\n      - include: common/login.yaml\n    catch: []\n")
	s, err := Load(main)
	if err != nil {
		t.Fatal(err)
	}
	if got := omap.Marshal(s.Vars, 0); got != `{"user": "me", "extra": 1}` {
		t.Fatal(got)
	}
	if s.Steps[0].(*Step).URL != "https://x/login" || s.Steps[1].(*TryBlock).Try[0].(*Step).URL != "https://x/login" {
		t.Fatal("steps not included")
	}
}

func TestNestedIncludeRelativeToIncludingFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a", "b", "leaf.yaml"), "steps: [{open: leaf}]")
	write(t, filepath.Join(dir, "a", "mid.yaml"), "steps: [{include: b/leaf.yaml}]")
	s, err := Load(write(t, filepath.Join(dir, "main.yaml"), "steps: [{include: a/mid.yaml}]"))
	if err != nil || s.Steps[0].(*Step).URL != "leaf" {
		t.Fatal(err)
	}
}

func TestIncludeErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.yaml"), "steps: [{include: b.yaml}]")
	write(t, filepath.Join(dir, "b.yaml"), "steps: [{include: a.yaml}]")
	write(t, filepath.Join(dir, "outside.yaml"), "steps: [{open: x}]")
	cases := []struct{ file, text, want string }{
		{"a.yaml", "", "Include cycle: a.yaml -> b.yaml -> a.yaml"},
		{"sub/main.yaml", "steps: [{include: ../outside.yaml}]", "escapes"},
		{"abs.yaml", "steps: [{include: " + filepath.Join(dir, "outside.yaml") + "}]", "Absolute"},
		{"m.yaml", "steps: [{include: nope.yaml}]", "does not exist"},
		{"m2.yaml", "steps: [{include: a.yaml, name: x}]", "no other keys"},
		{"m3.yaml", "steps: [{include: 5}]", "expects a file path, got: 5"},
	}
	for _, c := range cases {
		path := filepath.Join(dir, c.file)
		if c.text != "" {
			write(t, path, c.text)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.file, err)
		}
	}
}
