package engine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

type fakePage struct {
	count      int
	counts     []int // consumed one per Count call, then count
	visible    bool
	text, url  string
	title      string
	countErr   error
	shotErr    error
	content    string
	screenshot []byte
}

func (p *fakePage) Count(scenario.Locator) (int, error) {
	if p.countErr != nil {
		return 0, p.countErr
	}
	if len(p.counts) > 0 {
		n := p.counts[0]
		p.counts = p.counts[1:]
		return n, nil
	}
	return p.count, nil
}
func (p *fakePage) IsVisible(scenario.Locator) (bool, error) { return p.visible, nil }
func (p *fakePage) InnerText() (string, error)               { return p.text, nil }
func (p *fakePage) URL() (string, error)                     { return p.url, nil }
func (p *fakePage) Title() (string, error)                   { return p.title, nil }
func (p *fakePage) Content() (string, error)                 { return p.content, nil }
func (p *fakePage) Screenshot(path string) error {
	if p.shotErr != nil {
		return p.shotErr
	}
	return os.WriteFile(path, []byte("png"), 0o644)
}

type fakeBrowser struct {
	page    *fakePage
	pageErr error
	log     []*scenario.Step
	// queue holds results per action: an error fails the step, other values are the data.
	queue map[string][]any
}

func (b *fakeBrowser) Page(string) (Page, error) {
	if b.pageErr != nil {
		return nil, b.pageErr
	}
	return b.page, nil
}

func (b *fakeBrowser) ActionTimeout() int { return 10000 }

func (b *fakeBrowser) ExecuteStep(step *scenario.Step, _ string) *StepResult {
	b.log = append(b.log, step)
	r := &StepResult{Action: step.Action, Success: true}
	q := b.queue[step.Action]
	var data any
	if len(q) > 1 {
		data, b.queue[step.Action] = q[0], q[1:]
	} else if len(q) == 1 {
		data = q[0]
	}
	if err, ok := data.(error); ok {
		r.Success, r.Error = false, err.Error()
	} else {
		r.Data = data
	}
	return r
}

func (b *fakeBrowser) urls(action string) []string {
	var out []string
	for _, s := range b.log {
		if s.Action == action {
			out = append(out, s.URL)
		}
	}
	return out
}

func newBrowser() *fakeBrowser { return &fakeBrowser{page: &fakePage{}, queue: map[string][]any{}} }

var sleeps []float64

func noSleep(t *testing.T) *[]float64 {
	t.Helper()
	sleeps = nil
	old := sleepMs
	sleepMs = func(ms float64) { sleeps = append(sleeps, ms) }
	t.Cleanup(func() { sleepMs = old })
	return &sleeps
}

func inTempDir(t *testing.T) string {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	return dir
}

func script(t *testing.T, yaml string) *scenario.Script {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := scenario.Load(path)
	if err != nil {
		t.Fatalf("%s: %v", yaml, err)
	}
	return s
}

func run(t *testing.T, b Browser, yaml string, opts ...Options) *omap.Map {
	t.Helper()
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	res, err := Run(b, script(t, yaml), o)
	if err != nil {
		t.Fatal(err)
	}
	return res.Map()
}

func get(v any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			v, _ = v.(*omap.Map).Get(k)
		case int:
			switch l := v.(type) {
			case []any:
				v = l[k]
			case *omap.List:
				v = l.Items[k]
			}
		}
	}
	return v
}

func steps(out *omap.Map) []any { v, _ := out.Get("steps"); return v.([]any) }
func ok(out *omap.Map) bool     { return get(out, "success") == true }
func length(v any) int {
	if l, ok := v.(*omap.List); ok {
		return len(l.Items)
	}
	return len(v.([]any))
}

func vars(kv ...any) *omap.Map { return omap.New(kv...) }

func TestSubstituteTypesAndInterpolation(t *testing.T) {
	v := vars("n", 5, "items", []any{"a", "b"},
		"user", omap.New("name", "bob", "tags", []any{"x", "y"}))
	cases := []struct {
		in   any
		want any
	}{
		{"${n}", 5},
		{"${items}", []any{"a", "b"}},
		{"n=${n}!", "n=5!"},
		{"${user.name}", "bob"},
		{"${user.tags.1}", "y"},
		{"${items.0}-${ items.1 }", "a-b"},
		{[]any{"${n}", 7}, []any{5, 7}},
		{3, 3},
	}
	for _, c := range cases {
		got, err := Substitute(c.in, v)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %v, %v want %v", c.in, got, err, c.want)
		}
	}
	got, _ := Substitute(omap.New("k", []any{"${n}", 7}), v)
	if omap.Marshal(got, 0) != `{"k": [5, 7]}` {
		t.Errorf("%s", omap.Marshal(got, 0))
	}
}

func TestSubstituteEnvAndEscape(t *testing.T) {
	t.Setenv("AB_TEST_VAR", "secret")
	for in, want := range map[string]string{
		"${env.AB_TEST_VAR}": "secret",
		"$${x} and ${a}":     "${x} and 1",
		"$${x}":              "${x}",
	} {
		got, err := Substitute(in, vars("a", 1))
		if err != nil || got != want {
			t.Errorf("%s: got %v, %v", in, got, err)
		}
	}
}

func TestSubstituteUndefined(t *testing.T) {
	os.Unsetenv("AB_NOT_SET_XYZ")
	v := vars("user", omap.New(), "items", []any{})
	for _, in := range []string{"${nope}", "${user.age}", "${items.5}", "${env.AB_NOT_SET_XYZ}", "a${nope}b"} {
		_, err := Substitute(in, v)
		var te *TemplateError
		if !errors.As(err, &te) || !te.Undefined || !strings.Contains(err.Error(), "undefined variable") {
			t.Errorf("%s: %v", in, err)
		}
	}
}

func TestVarsDefaultsAndOverride(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "vars: {host: a.com, path: x}\nsteps:\n  - open: https://${host}/${path}\n", Options{Vars: vars("host", "b.org")})
	if !ok(out) || b.log[0].URL != "https://b.org/x" {
		t.Fatalf("%v", out)
	}
	if omap.Marshal(get(out, "vars"), 0) != `{"host": "b.org", "path": "x"}` {
		t.Errorf("%s", omap.Marshal(get(out, "vars"), 0))
	}
}

func TestSetKeepsTypesAndChains(t *testing.T) {
	b := newBrowser()
	out := run(t, b, `
steps:
  - set: {n: 3, names: [a]}
  - set: {title: "t-${n}", again: "${names}"}
  - open: "u/${title}"
`)
	if got := omap.Marshal(get(out, "vars"), 0); got != `{"n": 3, "names": ["a"], "title": "t-3", "again": ["a"]}` {
		t.Errorf("%s", got)
	}
	if got := omap.Marshal(get(steps(out)[1], "data"), 0); got != `{"vars": {"title": "t-3", "again": ["a"]}}` {
		t.Errorf("%s", got)
	}
	if b.log[0].URL != "u/t-3" {
		t.Errorf("%v", b.log[0].URL)
	}
}

func TestAsStoresValue(t *testing.T) {
	b := newBrowser()
	b.queue["extract"] = []any{omap.New("value", "42")}
	b.queue["evaluate"] = []any{omap.New("result", []any{1, 2})}
	b.queue["read"] = []any{omap.New("text", "hi")}
	out := run(t, b, `
steps:
  - {extract: h1, as: title}
  - {evaluate: "1", as: res}
  - {read: h1, as: whole}
  - {click: "#${title}"}
`)
	if got := omap.Marshal(get(out, "vars"), 0); got != `{"title": "42", "res": [1, 2], "whole": {"text": "hi"}}` {
		t.Errorf("%s", got)
	}
	if b.log[len(b.log)-1].Locator.Value != "#42" {
		t.Errorf("%+v", b.log[len(b.log)-1].Locator)
	}
}

func TestUndefinedVariableIsStepError(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "steps:\n  - open: ${missing}\n  - open: b\n")
	if ok(out) || len(steps(out)) != 1 || get(steps(out)[0], "error") != "undefined variable: missing" || len(b.log) != 0 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestNonStringCoercedForTextField(t *testing.T) {
	b := newBrowser()
	run(t, b, "vars: {n: 7}\nsteps:\n  - fill: ['#q', '${n}']\n")
	if b.log[0].Value == nil || *b.log[0].Value != "7" {
		t.Fatalf("%+v", b.log[0])
	}
}

func TestInvalidValueAfterSubstitutionFailsStep(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "vars: {d: {a: 1}}\nsteps:\n  - open: ${d}\n  - open: b\n")
	if ok(out) || len(b.log) != 0 || len(steps(out)) != 1 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
	if e, _ := get(steps(out)[0], "error").(string); !strings.HasPrefix(e, "invalid value after substitution: ") {
		t.Errorf("%v", e)
	}
}

func cond(t *testing.T, b Browser, yaml string, v *omap.Map) bool {
	t.Helper()
	raw, err := omap.ParseYAML([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	c, err := scenario.ParseCondition(raw)
	if err != nil {
		t.Fatalf("%s: %v", yaml, err)
	}
	if v == nil {
		v = omap.New()
	}
	return CheckCondition(b, c, v, "")
}

func TestConditionPageChecks(t *testing.T) {
	b := newBrowser()
	b.page.count, b.page.visible = 2, false
	b.page.text, b.page.url, b.page.title = "Hello world", "https://a.com/x?y=1", "My Title"
	for yaml, want := range map[string]bool{
		"{exists: '#a'}":          true,
		"{visible: '#a'}":         false,
		"{text: world}":           true,
		"{text: bye}":             false,
		"{url: a.com/x}":          true,
		`{urlMatches: 'x\?y=\d'}`: true,
		`{urlMatches: '^http:'}`:  false,
		"{title: Title}":          true,
	} {
		if got := cond(t, b, yaml, nil); got != want {
			t.Errorf("%s: %v", yaml, got)
		}
	}
	b.page.count = 0
	if cond(t, b, "{exists: '#a'}", nil) {
		t.Error("exists with 0")
	}
}

func TestConditionNeverFailsForPageErrors(t *testing.T) {
	b := newBrowser()
	b.page.countErr = errors.New("boom")
	if cond(t, b, "{exists: '#a'}", nil) {
		t.Error("count error")
	}
	b.pageErr = errors.New("no page")
	if cond(t, b, "{text: x}", nil) {
		t.Error("page error")
	}
}

func TestConditionVar(t *testing.T) {
	v := vars("a", 5, "s", "hello", "l", []any{"x"}, "z", 0, "u", omap.New("k", "v"))
	b := newBrowser()
	for yaml, want := range map[string]bool{
		"{var: a}":                     true,
		"{var: z}":                     false,
		"{var: undefined}":             false,
		"{var: a, equals: 5}":          true,
		"{var: a, equals: 6}":          false,
		"{var: s, contains: ell}":      true,
		"{var: l, contains: x}":        true,
		"{var: u, contains: k}":        true,
		"{var: s, matches: '^h.l+o$'}": true,
		"{var: u.k, equals: v}":        true,
		"{var: undefined, equals: 1}":  false,
	} {
		if got := cond(t, b, yaml, v); got != want {
			t.Errorf("%s: %v", yaml, got)
		}
	}
}

func TestConditionCombinators(t *testing.T) {
	v := vars("a", 1)
	b := newBrowser()
	tr, fa := "{var: a}", "{var: nope}"
	for yaml, want := range map[string]bool{
		"{not: " + fa + "}":               true,
		"{not: " + tr + "}":               false,
		"{all: [" + tr + ", " + tr + "]}": true,
		"{all: [" + tr + ", " + fa + "]}": false,
		"{any: [" + fa + ", " + tr + "]}": true,
		"{any: [" + fa + ", " + fa + "]}": false,
	} {
		if got := cond(t, b, yaml, v); got != want {
			t.Errorf("%s: %v", yaml, got)
		}
	}
}

func TestConditionTimeoutPolls(t *testing.T) {
	s := noSleep(t)
	b := newBrowser()
	b.page.counts = []int{0, 0, 1}
	if !cond(t, b, "{exists: '#a', timeout: 5000}", nil) {
		t.Fatal("should pass")
	}
	if !reflect.DeepEqual(*s, []float64{250, 250}) {
		t.Errorf("%v", *s)
	}
	b.page.count = 0
	if cond(t, b, "{exists: '#a', timeout: 0}", nil) {
		t.Error("should fail")
	}
}

func TestAssertStep(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "vars: {a: 1}\nsteps:\n  - assert: {var: a, equals: 1}\n")
	if !ok(out) || omap.Marshal(get(steps(out)[0], "data"), 0) != `{"condition": true}` {
		t.Errorf("%s", omap.Marshal(out, 0))
	}
	out = run(t, b, "vars: {a: 1, b: 2}\nsteps:\n  - assert: {var: a, equals: '${b}'}\n    message: a is ${a}\n")
	if ok(out) || get(steps(out)[0], "error") != "a is 1" {
		t.Errorf("%s", omap.Marshal(out, 0))
	}
	out = run(t, b, "vars: {a: 1}\nsteps:\n  - assert: {var: a, equals: 2}\n")
	if e, _ := get(steps(out)[0], "error").(string); e != `assertion failed: {"var": "a", "equals": 2}` {
		t.Errorf("%q", e)
	}
}

func TestResultShapeAndKeyOrder(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "steps:\n  - {open: x, name: go}\n")
	if got := out.Keys(); !reflect.DeepEqual(got, []string{"success", "steps", "vars"}) {
		t.Errorf("%v", got)
	}
	if got := steps(out)[0].(*omap.Map).Keys(); !reflect.DeepEqual(got, []string{"action", "success", "durationMs", "name"}) {
		t.Errorf("%v", got)
	}
	if got := omap.Marshal(get(run(t, b, "steps: []\n"), "steps"), 0); got != "[]" {
		t.Errorf("%s", got)
	}
}

func TestRetryAttemptsAndDelay(t *testing.T) {
	s := noSleep(t)
	b := newBrowser()
	b.queue["click"] = []any{errors.New("e1"), errors.New("e2"), nil}
	out := run(t, b, "steps:\n  - {click: '#a', retry: 2, retryDelay: 10, name: go}\n")
	st := steps(out)[0]
	if !ok(out) || get(st, "attempts") != 3 || get(st, "name") != "go" || len(b.log) != 3 || !reflect.DeepEqual(*s, []float64{10, 10}) {
		t.Fatalf("%s %v", omap.Marshal(out, 0), *s)
	}
}

func TestRetryExhaustedFails(t *testing.T) {
	s := noSleep(t)
	b := newBrowser()
	b.queue["click"] = []any{errors.New("nope")}
	out := run(t, b, "steps:\n  - {click: '#a', retry: 1}\n  - open: x\n")
	st := steps(out)[0]
	if ok(out) || len(steps(out)) != 1 || get(st, "attempts") != 2 || get(st, "error") != "nope" || !reflect.DeepEqual(*s, []float64{1000}) {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestOptionalFailureRecordedAndIgnored(t *testing.T) {
	b := newBrowser()
	b.queue["click"] = []any{errors.New("nope")}
	out := run(t, b, "steps:\n  - {click: '#a', optional: true}\n  - open: x\n")
	first := steps(out)[0]
	if !ok(out) || len(steps(out)) != 2 || get(first, "success") != true || get(first, "optional") != true || get(first, "error") != "nope" {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestStopOnErrorFalseContinues(t *testing.T) {
	b := newBrowser()
	b.queue["click"] = []any{errors.New("nope")}
	out := run(t, b, "steps:\n  - {click: '#a', stopOnError: false}\n  - open: x\n")
	if ok(out) || len(steps(out)) != 2 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
	out = run(t, b, "stopOnError: false\nsteps:\n  - {click: '#a'}\n  - open: x\n")
	if ok(out) || len(steps(out)) != 2 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestErrorArtifacts(t *testing.T) {
	dir := inTempDir(t)
	b := newBrowser()
	b.page.content = "<html></html>"
	b.queue["click"] = []any{errors.New("nope")}
	out := run(t, b, "errorArtifacts: art\nsteps:\n  - {click: '#a', stopOnError: false}\n  - {click: '#b', stopOnError: false}\n")
	if got := omap.Marshal(get(steps(out)[0], "artifacts"), 0); got != `{"screenshot": "art/001-click.png", "html": "art/001-click.html"}` {
		t.Errorf("%s", got)
	}
	if get(steps(out)[1], "artifacts", "screenshot") != "art/002-click.png" {
		t.Errorf("%s", omap.Marshal(steps(out)[1], 0))
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "art", "001-click.html")); string(data) != "<html></html>" {
		t.Errorf("%q", data)
	}
}

func TestErrorArtifactsErrorsNotRaised(t *testing.T) {
	inTempDir(t)
	b := newBrowser()
	b.page.shotErr = errors.New("closed")
	b.queue["click"] = []any{errors.New("nope")}
	out := run(t, b, "steps:\n  - click: '#a'\n", Options{ArtifactsDir: "art"})
	st := steps(out)[0].(*omap.Map)
	if st.Has("artifacts") || get(st, "error") != "nope" {
		t.Errorf("%s", omap.Marshal(st, 0))
	}
}

func TestArtifactsDirMustBeInsideCwd(t *testing.T) {
	inTempDir(t)
	if _, err := Run(newBrowser(), script(t, "steps:\n  - open: x\n"), Options{ArtifactsDir: "/tmp/art"}); err == nil {
		t.Error("absolute path accepted")
	}
	if _, err := Run(newBrowser(), script(t, "steps:\n  - open: x\n"), Options{ArtifactsDir: "../art"}); err == nil {
		t.Error("traversal accepted")
	}
}

func TestIfBranches(t *testing.T) {
	b := newBrowser()
	out := run(t, b, `
vars: {a: 1}
steps:
  - {if: {var: a}, then: [{open: then}], else: [{open: else}]}
  - {if: {var: nope}, then: [{open: x}]}
  - {if: {var: a}, then: [], name: n}
`)
	s := steps(out)
	if !ok(out) || !reflect.DeepEqual(b.urls("open"), []string{"then"}) {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
	if get(s[0], "condition") != true || get(s[0], "branch") != "then" || length(get(s[0], "steps")) != 1 {
		t.Errorf("%s", omap.Marshal(s[0], 0))
	}
	if get(s[1], "condition") != false || get(s[1], "branch") != nil || length(get(s[1], "steps")) != 0 {
		t.Errorf("%s", omap.Marshal(s[1], 0))
	}
	if get(s[2], "name") != "n" {
		t.Errorf("%s", omap.Marshal(s[2], 0))
	}
	if got := s[0].(*omap.Map).Keys(); !reflect.DeepEqual(got, []string{"action", "success", "condition", "branch", "steps"}) {
		t.Errorf("%v", got)
	}
}

func TestIfElseBranchTaken(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "steps:\n  - {if: {var: a}, then: [{open: t}], else: [{open: e}]}\n", Options{Vars: vars("a", 0)})
	if get(steps(out)[0], "branch") != "else" || b.log[0].URL != "e" {
		t.Errorf("%s", omap.Marshal(out, 0))
	}
}

func TestIfConditionSubstitutedAndUndefined(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "vars: {a: 1, b: 1}\nsteps:\n  - {if: {var: a, equals: '${b}'}, then: [{open: t}]}\n")
	if get(steps(out)[0], "condition") != true {
		t.Errorf("%s", omap.Marshal(out, 0))
	}
	out = run(t, b, "vars: {a: 1}\nsteps:\n  - {if: {var: a, equals: '${zz}'}, then: []}\n")
	if ok(out) || get(steps(out)[0], "error") != "undefined variable: zz" {
		t.Errorf("%s", omap.Marshal(out, 0))
	}
}

func TestTryCatchHandlesFailure(t *testing.T) {
	b := newBrowser()
	b.queue["click"] = []any{errors.New("nope")}
	out := run(t, b, `
steps:
  - try: [{open: a}, {click: "#x"}, {open: skipped}]
    catch: [{open: "c/${error.message}/${error.action}"}]
    finally: [{open: f}]
  - {open: after}
`)
	if !ok(out) || !reflect.DeepEqual(b.urls("open"), []string{"a", "c/nope/click", "f", "after"}) {
		t.Fatalf("%v %s", b.urls("open"), omap.Marshal(out, 0))
	}
	node := steps(out)[0]
	inner := get(node, "steps", 1)
	if get(node, "success") != true || get(inner, "handled") != true || get(inner, "success") != false ||
		length(get(node, "catch")) != 1 || length(get(node, "finally")) != 1 {
		t.Errorf("%s", omap.Marshal(node, 0))
	}
	if got := omap.Marshal(get(out, "vars", "error"), 0); got != `{"message": "nope", "action": "click", "name": null}` {
		t.Errorf("%s", got)
	}
}

func TestTryWithoutCatchPropagatesAfterFinally(t *testing.T) {
	b := newBrowser()
	b.queue["click"] = []any{errors.New("nope")}
	out := run(t, b, "steps:\n  - {try: [{click: '#x'}], finally: [{open: f}]}\n  - {open: after}\n")
	node := steps(out)[0]
	if ok(out) || len(steps(out)) != 1 || get(node, "success") != false || length(get(node, "finally")) != 1 ||
		!reflect.DeepEqual(b.urls("open"), []string{"f"}) {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestFailureInCatchPropagatesAfterFinally(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "steps:\n  - {try: [{fail: one}], catch: [{fail: two}], finally: [{open: f}]}\n")
	node := steps(out)[0]
	if ok(out) || get(node, "success") != false || get(node, "catch", 0, "error") != "two" ||
		get(node, "finally", 0, "success") != true || get(node, "steps", 0, "handled") != true {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestNestedTryOuterCatchesInnerPropagation(t *testing.T) {
	b := newBrowser()
	out := run(t, b, `
steps:
  - try: [{try: [{fail: deep}], finally: [{open: inner-f}]}]
    catch: [{open: outer-c}]
`)
	outer := steps(out)[0]
	inner := get(outer, "steps", 0)
	if !ok(out) || get(outer, "success") != true || get(inner, "success") != false || get(inner, "handled") != true ||
		get(inner, "steps", 0, "handled") != true || !reflect.DeepEqual(b.urls("open"), []string{"inner-f", "outer-c"}) {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestTryDoesNotCatchRecordedFailure(t *testing.T) {
	b := newBrowser()
	b.queue["click"] = []any{errors.New("nope")}
	out := run(t, b, "steps:\n  - {try: [{click: '#x', stopOnError: false}, {open: next}], catch: [{open: c}]}\n")
	if ok(out) || !reflect.DeepEqual(b.urls("open"), []string{"next"}) || get(steps(out)[0], "success") != false {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestOptionalInsideTryDoesNotTriggerCatch(t *testing.T) {
	b := newBrowser()
	b.queue["click"] = []any{errors.New("nope")}
	out := run(t, b, "steps:\n  - {try: [{click: '#x', optional: true}], catch: [{open: c}]}\n")
	if !ok(out) || len(b.urls("open")) != 0 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestFailActionAndTopLevelStop(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "vars: {a: 1}\nsteps:\n  - {fail: 'bad ${a}'}\n  - {open: x}\n")
	if ok(out) || len(steps(out)) != 1 || get(steps(out)[0], "error") != "bad 1" || len(b.log) != 0 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestFailureInNestedBlockStopsScript(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "steps:\n  - {repeat: 3, do: [{open: a}, {fail: x}]}\n  - {open: never}\n")
	node := steps(out)[0]
	if ok(out) || len(b.log) != 1 || get(node, "success") != false || length(get(node, "iterations")) != 1 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestForEachItemsIndexAndLiteral(t *testing.T) {
	b := newBrowser()
	out := run(t, b, `
steps:
  - {forEach: [a, b], as: x, do: [{open: "${index}:${x}"}]}
  - {set: {urls: [u1, u2]}}
  - {forEach: "${urls}", do: [{open: "${item}"}]}
`)
	if !reflect.DeepEqual(b.urls("open"), []string{"0:a", "1:b", "u1", "u2"}) || length(get(steps(out)[0], "iterations")) != 2 || !ok(out) {
		t.Fatalf("%v %s", b.urls("open"), omap.Marshal(out, 0))
	}
}

func TestForEachNotAList(t *testing.T) {
	out := run(t, newBrowser(), "vars: {v: str}\nsteps:\n  - {forEach: '${v}', do: []}\n")
	if e, _ := get(steps(out)[0], "error").(string); ok(out) || e != "forEach expects a list, got str" {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestRepeat(t *testing.T) {
	b := newBrowser()
	out := run(t, b, "steps:\n  - {repeat: 3, do: [{open: 'u${index}'}]}\n")
	if !reflect.DeepEqual(b.urls("open"), []string{"u0", "u1", "u2"}) || length(get(steps(out)[0], "iterations")) != 3 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
	out = run(t, b, "vars: {n: '2'}\nsteps:\n  - {repeat: '${n}', do: []}\n")
	if length(get(steps(out)[0], "iterations")) != 2 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
	out = run(t, b, "vars: {n: abc}\nsteps:\n  - {repeat: '${n}', do: []}\n")
	if e, _ := get(steps(out)[0], "error").(string); ok(out) || e != "repeat expects a non-negative integer, got 'abc'" {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestWhileRunsUntilFalseAndMax(t *testing.T) {
	b := newBrowser()
	out := run(t, b, `
vars: {go: true}
steps:
  - while: {var: go}
    max: 5
    do:
      - {open: x}
      - {if: {var: index, equals: 1}, then: [{set: {go: false}}]}
`)
	if !ok(out) || length(get(steps(out)[0], "iterations")) != 2 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
	out = run(t, b, "vars: {go: true}\nsteps:\n  - {while: {var: go}, max: 3, do: [{open: x}]}\n")
	if ok(out) || length(get(steps(out)[0], "iterations")) != 3 || get(steps(out)[0], "error") != "while exceeded max 3 iterations" {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestWhileExactMaxIsOK(t *testing.T) {
	out := run(t, newBrowser(), `
vars: {go: true}
steps:
  - while: {var: go}
    max: 2
    do: [{if: {var: index, equals: 1}, then: [{set: {go: false}}]}]
`)
	if !ok(out) {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

func TestEvaluateArgsTemplated(t *testing.T) {
	b := newBrowser()
	run(t, b, `
vars: {q: hi, n: 5}
steps:
  - evaluate: "(a) => a"
    args: {q: "${q}", n: "${n}", t: "x-${q}"}
`)
	if got := omap.Marshal(b.log[0].Args, 0); got != `{"q": "hi", "n": 5, "t": "x-hi"}` {
		t.Errorf("%s", got)
	}
}

const waitAnyYAML = `
steps:
  - wait: {any: [{text: alpha}, {urlMatches: "/sorry/"}]}
    as: gate
%s
  - if: {var: gate, equals: 1}
    then: [{set: {branch: second}}]
    else: [{set: {branch: first}}]
`

func TestWaitAnyFirstMatches(t *testing.T) {
	sl := noSleep(t)
	b := newBrowser()
	b.page.text, b.page.url = "alpha beta", "https://a.com/sorry/x"
	out := run(t, b, strings.Replace(waitAnyYAML, "%s", "", 1))
	if !ok(out) || get(steps(out)[0], "action") != "wait" || omap.Marshal(get(steps(out)[0], "data"), 0) != `{"matched": 0}` {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
	if get(out, "vars", "gate") != 0 || get(out, "vars", "branch") != "first" || len(*sl) != 0 || len(b.log) != 0 {
		t.Fatalf("%s %v", omap.Marshal(out, 0), *sl)
	}
}

func TestWaitAnySecondMatchesAfterPolling(t *testing.T) {
	sl := noSleep(t)
	b := newBrowser()
	b.page.text = "x"
	b.page.url = "https://a.com/"
	b.page.counts = nil
	// the url flips to /sorry/ after two polling passes
	pg := &flippingPage{fakePage: b.page, flipAfter: 2}
	b2 := &pageBrowser{fakeBrowser: b, page: pg}
	out := run(t, b2, strings.Replace(waitAnyYAML, "%s", "", 1))
	if !ok(out) || omap.Marshal(get(steps(out)[0], "data"), 0) != `{"matched": 1}` {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
	if get(out, "vars", "gate") != 1 || get(out, "vars", "branch") != "second" || !reflect.DeepEqual(*sl, []float64{250, 250}) {
		t.Fatalf("%s %v", omap.Marshal(out, 0), *sl)
	}
}

func TestWaitAnyTimeoutError(t *testing.T) {
	noSleep(t)
	b := newBrowser()
	b.page.text, b.page.url = "nothing", "https://a.com/"
	out := run(t, b, strings.Replace(waitAnyYAML, "%s", "    timeout: 20", 1))
	if ok(out) || len(steps(out)) != 1 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
	want := `wait any timed out after 20ms: none of [{"text": "alpha"}, {"urlMatches": "/sorry/"}]`
	if e, _ := get(steps(out)[0], "error").(string); e != want {
		t.Fatalf("%q", e)
	}
	if _, has := get(out, "vars").(*omap.Map).Get("gate"); has {
		t.Fatal("gate stored on timeout")
	}
}

func TestWaitAnyTemplated(t *testing.T) {
	noSleep(t)
	b := newBrowser()
	b.page.text = "hello bob"
	out := run(t, b, "vars: {who: bob}\nsteps:\n  - wait: {any: [{text: '${who}'}]}\n    as: i\n")
	if !ok(out) || get(out, "vars", "i") != 0 {
		t.Fatalf("%s", omap.Marshal(out, 0))
	}
}

// pageBrowser serves a different page than the embedded fakeBrowser's.
type pageBrowser struct {
	*fakeBrowser
	page Page
}

func (b *pageBrowser) Page(string) (Page, error) { return b.page, nil }

// flippingPage reports /sorry/ in the url after flipAfter URL calls.
type flippingPage struct {
	*fakePage
	flipAfter, calls int
}

func (p *flippingPage) URL() (string, error) {
	p.calls++
	if p.calls > p.flipAfter {
		return "https://a.com/sorry/", nil
	}
	return p.fakePage.URL()
}
