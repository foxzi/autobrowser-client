package engine

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// stepFailure is an unhandled failure; it propagates up to the nearest try block.
type stepFailure struct {
	message string
	action  string
	name    string
	nodes   []*omap.Map // result nodes the failure passed through, marked handled by try
}

func (f *stepFailure) Error() string { return f.message }

type runCtx struct {
	b             Browser
	vars          *omap.Map
	stopOnError   bool
	pageID        string
	failed        bool
	artifactsDir  string
	artifactCount int
}

// nameValue is a result/err value for an optional name: null when empty.
func nameValue(name string) any {
	if name == "" {
		return nil
	}
	return name
}

// saveArtifacts saves a screenshot and the HTML of the page; errors are logged, never returned.
func (c *runCtx) saveArtifacts(action string) *omap.Map {
	c.artifactCount++
	base := filepath.Join(c.artifactsDir, fmt.Sprintf("%03d-%s", c.artifactCount, action))
	saved := omap.New()
	err := func() error {
		page, err := c.b.Page(c.pageID)
		if err != nil {
			return err
		}
		shot, err := ValidateOutputPath(base + ".png")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(shot), 0o755); err != nil {
			return err
		}
		if err := page.Screenshot(shot); err != nil {
			return err
		}
		saved.Set("screenshot", shot)
		html, err := ValidateOutputPath(base + ".html")
		if err != nil {
			return err
		}
		content, err := page.Content()
		if err != nil {
			return err
		}
		if err := os.WriteFile(html, []byte(content), 0o644); err != nil {
			return err
		}
		saved.Set("html", html)
		return nil
	}()
	if err != nil {
		slog.Warn("Saving error artifacts failed: " + err.Error())
	}
	return saved
}

func storedValue(step *scenario.Step, data any) any {
	key := ""
	switch step.Action {
	case "extract":
		key = "value"
	case "evaluate":
		key = "result"
	default:
		return data
	}
	v, _ := mapGet(data, key)
	return v
}

func templateErr(err error) error {
	if te, ok := err.(*TemplateError); ok {
		return te
	}
	return &TemplateError{Msg: err.Error()}
}

func invalidAfterSubstitution(err *scenario.ValidationError) error {
	return &TemplateError{Msg: "invalid value after substitution: " + err.Details()}
}

func substituteStep(step *scenario.Step, vars *omap.Map) (*scenario.Step, error) {
	data, err := Substitute(step.Raw, vars)
	if err != nil {
		return nil, templateErr(err)
	}
	concrete, err := scenario.ParseStep(data)
	if err != nil {
		return nil, invalidAfterSubstitution(err.(*scenario.ValidationError))
	}
	return concrete, nil
}

func substituteCondition(cond *scenario.Condition, vars *omap.Map) (*scenario.Condition, error) {
	data, err := Substitute(cond.Raw, vars)
	if err != nil {
		return nil, templateErr(err)
	}
	concrete, err := scenario.ParseCondition(data)
	if err != nil {
		return nil, invalidAfterSubstitution(err.(*scenario.ValidationError))
	}
	return concrete, nil
}

func elapsedMs(t0 time.Time) float64 { return float64(time.Since(t0)) / float64(time.Millisecond) }

// attempt substitutes variables and runs one step once.
func (c *runCtx) attempt(step *scenario.Step) *StepResult {
	concrete, err := substituteStep(step, c.vars)
	if err != nil {
		return &StepResult{Action: step.Action, Error: err.Error()}
	}
	t0 := time.Now()
	switch concrete.Action {
	case "set":
		for _, k := range concrete.Vars.Keys() {
			v, _ := concrete.Vars.Get(k)
			c.vars.Set(k, v)
		}
		return &StepResult{Action: "set", Success: true, Data: omap.New("vars", concrete.Vars), DurationMs: elapsedMs(t0)}
	case "fail":
		return &StepResult{Action: "fail", Error: concrete.Message}
	case "assert":
		ok := CheckCondition(c.b, concrete.Condition, c.vars, c.pageID)
		result := &StepResult{Action: "assert", Success: ok}
		if ok {
			result.Data = omap.New("condition", true)
		} else if concrete.Message != "" {
			result.Error = concrete.Message
		} else {
			result.Error = "assertion failed: " + omap.Marshal(concrete.Condition.Raw, 0)
		}
		result.DurationMs = elapsedMs(t0)
		return result
	}
	result := c.b.ExecuteStep(concrete, c.pageID)
	if result.Success && concrete.AsName != nil {
		c.vars.Set(*concrete.AsName, storedValue(concrete, result.Data))
	}
	return result
}

// runStep runs a step with retries and records its result in out; it
// returns a *stepFailure when the step stops the script.
func (c *runCtx) runStep(step *scenario.Step, out *omap.List) error {
	attempts := 0
	var result *StepResult
	for {
		attempts++
		result = c.attempt(step)
		if result.Success || attempts > step.Retry {
			break
		}
		slog.Info(fmt.Sprintf("Step '%s' failed (attempt %d): %s", step.Action, attempts, result.Error))
		sleepMs(float64(step.RetryDelay))
	}

	node := result.Map()
	if step.Name != "" {
		node.Set("name", step.Name)
	}
	if step.Retry != 0 {
		node.Set("attempts", attempts)
	}
	out.Append(node)
	if result.Success {
		return nil
	}
	if step.Optional {
		node.Set("success", true)
		node.Set("optional", true)
		return nil
	}
	if c.artifactsDir != "" {
		if artifacts := c.saveArtifacts(step.Action); artifacts.Len() > 0 {
			node.Set("artifacts", artifacts)
		}
	}
	stop := c.stopOnError
	if step.StopOnError != nil {
		stop = *step.StopOnError
	}
	if stop {
		return &stepFailure{message: result.Error, action: step.Action, name: step.Name, nodes: []*omap.Map{node}}
	}
	c.failed = true
	return nil
}

func blockFail(node *omap.Map, kind, name, message string) *stepFailure {
	node.Set("error", message)
	return &stepFailure{message: message, action: kind, name: name, nodes: []*omap.Map{node}}
}

func (c *runCtx) substituteBlock(value any, node *omap.Map, kind, name string) (any, *stepFailure) {
	var out any
	var err error
	if cond, ok := value.(*scenario.Condition); ok {
		out, err = substituteCondition(cond, c.vars)
	} else {
		out, err = Substitute(value, c.vars)
	}
	if err != nil {
		return nil, blockFail(node, kind, name, err.Error())
	}
	return out, nil
}

func (c *runCtx) runIf(block *scenario.IfBlock, node *omap.Map) error {
	v, fail := c.substituteBlock(block.If, node, "if", block.Name)
	if fail != nil {
		return fail
	}
	matched := CheckCondition(c.b, v.(*scenario.Condition), c.vars, c.pageID)
	branch := block.Else
	var branchName any
	if matched {
		branch, branchName = block.Then, "then"
	} else if block.HasElse {
		branchName = "else"
	}
	node.Set("condition", matched)
	node.Set("branch", branchName)
	steps := &omap.List{}
	node.Set("steps", steps)
	return c.runSteps(branch, steps)
}

func (c *runCtx) runTry(block *scenario.TryBlock, node *omap.Map) error {
	steps := &omap.List{}
	node.Set("steps", steps)
	var pending error
	if err := c.runSteps(block.Try, steps); err != nil {
		pending = err
		if block.HasCatch {
			f := err.(*stepFailure)
			for _, failed := range f.nodes {
				failed.Set("handled", true)
			}
			c.vars.Set("error", omap.New("message", f.message, "action", f.action, "name", nameValue(f.name)))
			catch := &omap.List{}
			node.Set("catch", catch)
			pending = c.runSteps(block.Catch, catch)
		}
	}
	if block.HasFinally {
		finally := &omap.List{}
		node.Set("finally", finally)
		if err := c.runSteps(block.Finally, finally); err != nil {
			return err
		}
	}
	return pending
}

func (c *runCtx) runIteration(steps []scenario.Node, node *omap.Map, index int) error {
	c.vars.Set("index", index)
	iteration := &omap.List{}
	list, _ := node.Get("iterations")
	list.(*omap.List).Append(iteration)
	return c.runSteps(steps, iteration)
}

func (c *runCtx) runForEach(block *scenario.ForEachBlock, node *omap.Map) error {
	v, fail := c.substituteBlock(block.Items, node, "forEach", block.Name)
	if fail != nil {
		return fail
	}
	items, ok := listItems(v)
	if !ok {
		return blockFail(node, "forEach", block.Name, "forEach expects a list, got "+pyTypeName(v))
	}
	node.Set("iterations", &omap.List{})
	for i, item := range items {
		c.vars.Set(block.AsName, item)
		if err := c.runIteration(block.Do, node, i); err != nil {
			return err
		}
	}
	return nil
}

func (c *runCtx) runRepeat(block *scenario.RepeatBlock, node *omap.Map) error {
	raw, fail := c.substituteBlock(block.Count, node, "repeat", block.Name)
	if fail != nil {
		return fail
	}
	count, ok := pyInt(raw)
	if !ok || count < 0 {
		return blockFail(node, "repeat", block.Name, "repeat expects a non-negative integer, got "+omap.Repr(raw))
	}
	node.Set("iterations", &omap.List{})
	for i := 0; i < count; i++ {
		if err := c.runIteration(block.Do, node, i); err != nil {
			return err
		}
	}
	return nil
}

func (c *runCtx) runWhile(block *scenario.WhileBlock, node *omap.Map) error {
	iterations := &omap.List{}
	node.Set("iterations", iterations)
	for {
		v, fail := c.substituteBlock(block.While, node, "while", block.Name)
		if fail != nil {
			return fail
		}
		if !CheckCondition(c.b, v.(*scenario.Condition), c.vars, c.pageID) {
			return nil
		}
		if len(iterations.Items) >= block.Max {
			return blockFail(node, "while", block.Name, fmt.Sprintf("while exceeded max %d iterations", block.Max))
		}
		if err := c.runIteration(block.Do, node, len(iterations.Items)); err != nil {
			return err
		}
	}
}

// childrenOK: a block succeeds when all its results succeeded or were handled by a try.
func childrenOK(node *omap.Map) bool {
	var groups []*omap.List
	for _, key := range []string{"steps", "catch", "finally"} {
		if v, ok := node.Get(key); ok {
			groups = append(groups, v.(*omap.List))
		}
	}
	if v, ok := node.Get("iterations"); ok {
		for _, it := range v.(*omap.List).Items {
			groups = append(groups, it.(*omap.List))
		}
	}
	for _, g := range groups {
		for _, item := range g.Items {
			r := item.(*omap.Map)
			if ok, _ := r.Get("success"); ok == true {
				continue
			}
			if h, _ := r.Get("handled"); h == true {
				continue
			}
			return false
		}
	}
	return true
}

func (c *runCtx) runBlock(block scenario.Node, out *omap.List) error {
	var kind, name string
	var runner func(*omap.Map) error
	switch b := block.(type) {
	case *scenario.IfBlock:
		kind, name, runner = "if", b.Name, func(n *omap.Map) error { return c.runIf(b, n) }
	case *scenario.TryBlock:
		kind, name, runner = "try", b.Name, func(n *omap.Map) error { return c.runTry(b, n) }
	case *scenario.ForEachBlock:
		kind, name, runner = "forEach", b.Name, func(n *omap.Map) error { return c.runForEach(b, n) }
	case *scenario.RepeatBlock:
		kind, name, runner = "repeat", b.Name, func(n *omap.Map) error { return c.runRepeat(b, n) }
	case *scenario.WhileBlock:
		kind, name, runner = "while", b.Name, func(n *omap.Map) error { return c.runWhile(b, n) }
	}
	node := omap.New("action", kind)
	if name != "" {
		node.Set("name", name)
	}
	node.Set("success", true)
	out.Append(node)
	if err := runner(node); err != nil {
		node.Set("success", false)
		f := err.(*stepFailure)
		seen := false
		for _, n := range f.nodes {
			seen = seen || n == node
		}
		if !seen {
			f.nodes = append(f.nodes, node)
		}
		return f
	}
	node.Set("success", childrenOK(node))
	return nil
}

func (c *runCtx) runSteps(steps []scenario.Node, out *omap.List) error {
	for _, n := range steps {
		if step, ok := n.(*scenario.Step); ok {
			slog.Info("Executing step: " + step.Action)
			if err := c.runStep(step, out); err != nil {
				return err
			}
		} else if err := c.runBlock(n, out); err != nil {
			return err
		}
	}
	return nil
}

// Options are the per-run settings of Run.
type Options struct {
	// PageID is the default tab for steps (empty: current/first).
	PageID string
	// Vars override the scenario's vars.
	Vars *omap.Map
	// ArtifactsDir overrides the scenario's errorArtifacts.
	ArtifactsDir string
}

// Result is the outcome of a scenario run.
type Result struct {
	Success bool
	Steps   []any // *omap.Map nodes
	Vars    *omap.Map
}

// Map returns {success, steps, vars} in that key order.
func (r *Result) Map() *omap.Map {
	return omap.New("success", r.Success, "steps", r.Steps, "vars", r.Vars)
}

// Run executes a scenario on an open browser. A failing step is reported in
// the Result; an error is returned only for invalid options (artifacts path).
func Run(b Browser, script *scenario.Script, opts Options) (*Result, error) {
	artifactsDir := opts.ArtifactsDir
	if artifactsDir == "" {
		artifactsDir = script.ErrorArtifacts
	}
	if artifactsDir != "" {
		if _, err := ValidateOutputPath(artifactsDir); err != nil {
			return nil, err
		}
	}
	vars := script.Vars.Copy()
	if opts.Vars != nil {
		for _, k := range opts.Vars.Keys() {
			v, _ := opts.Vars.Get(k)
			vars.Set(k, v)
		}
	}
	c := &runCtx{b: b, vars: vars, stopOnError: script.StopOnError, pageID: opts.PageID, artifactsDir: artifactsDir}
	results := &omap.List{}
	if err := c.runSteps(script.Steps, results); err != nil {
		slog.Error("Stopping script: " + err.Error())
		c.failed = true
	}
	return &Result{Success: !c.failed, Steps: results.Items, Vars: c.vars}, nil
}
