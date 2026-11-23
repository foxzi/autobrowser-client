package browser

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/scenario"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

// injectedScript is Playwright's injected script, which builds the aria
// snapshot (page.aria_snapshot(mode="ai")) and resolves aria-ref selectors.
// Regenerate it with injected/extract.sh.
//
//go:embed injected/injected_script.js
var injectedScript string

// Snapshot is the result of page_snapshot.
type Snapshot struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Snapshot  string `json:"snapshot"`
	Truncated bool   `json:"truncated"`
}

// Snapshot is actions.page_snapshot: the accessibility tree with [ref=eN]
// marks, cut to output.maxContentBytes.
func (s *Session) Snapshot(pageID string, timeoutMs int) (*Snapshot, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, err
	}
	text, err := t.ariaSnapshot(timeoutMs)
	if err != nil {
		return nil, err
	}
	return s.snapshotResult(t, text)
}

// ElementSnapshot is page_snapshot with a locator: the aria snapshot of the
// first match (Locator.aria_snapshot(mode="ai")), whose refs work like the
// refs of a page snapshot.
func (s *Session) ElementSnapshot(pageID string, l scenario.Locator, timeoutMs int) (*Snapshot, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, err
	}
	lc, err := t.locator(l, nil)
	if err != nil {
		return nil, err
	}
	lc.repr += ".first"
	el, err := lc.waitFor("aria_snapshot", "attached", timeoutMs)
	if err != nil {
		return nil, err
	}
	text, err := t.elementAriaSnapshot(el)
	if err != nil {
		return nil, err
	}
	return s.snapshotResult(t, text)
}

func (s *Session) snapshotResult(t *tab, text string) (*Snapshot, error) {
	var err error
	res := &Snapshot{Snapshot: text}
	if max := s.output.MaxContentBytes; max > 0 && len(text) > max {
		res.Snapshot, res.Truncated = strings.ToValidUTF8(text[:max], ""), true
	}
	if res.URL, err = t.url(); err != nil {
		return nil, err
	}
	if res.Title, err = t.Title(); err != nil {
		return nil, err
	}
	return res, nil
}

// ariaTab keeps what Playwright's frame manager knows about a tab: the frame
// sequence numbers that prefix refs in child frames (f1e2) and the injected
// script of every frame, which remembers the refs of its latest snapshot.
type ariaTab struct {
	mu      sync.Mutex
	seqs    map[proto.PageFrameID]int
	next    int
	mainURL string
	worlds  map[proto.PageFrameID]*ariaWorld
}

// ariaWorld is the injected script living in an isolated world of a frame.
type ariaWorld struct {
	page *rod.Page // the session of the frame: the tab or an out-of-process iframe
	obj  proto.RuntimeRemoteObjectID
	ctx  proto.RuntimeExecutionContextID
}

// newAria starts tracking the frames of a page. Playwright numbers frames from
// the moment the page appears, so it is called when the tab is registered.
func newAria(page *rod.Page) *ariaTab {
	a := &ariaTab{seqs: map[proto.PageFrameID]int{}, worlds: map[proto.PageFrameID]*ariaWorld{}}
	a.mu.Lock()
	defer a.mu.Unlock()
	go page.EachEvent(func(e *proto.PageFrameAttached) {
		a.mu.Lock()
		a.assign(e.FrameID)
		a.mu.Unlock()
	}, func(e *proto.PageFrameDetached) {
		if e.Reason == proto.PageFrameDetachedReasonRemove {
			a.mu.Lock()
			delete(a.seqs, e.FrameID)
			delete(a.worlds, e.FrameID)
			a.mu.Unlock()
		}
	}, func(e *proto.PageFrameNavigated) {
		if e.Frame.ParentID != "" {
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		// A main frame navigation away from a real page gets a new number.
		if a.mainURL != "" && a.mainURL != "about:blank" {
			a.seqs[e.Frame.ID] = a.next
			a.next++
		}
		a.mainURL = e.Frame.URL + e.Frame.URLFragment
	})()
	if tree, err := (proto.PageGetFrameTree{}).Call(page); err == nil {
		a.mainURL = tree.FrameTree.Frame.URL + tree.FrameTree.Frame.URLFragment
		a.addTree(tree.FrameTree)
	}
	return a
}

func (a *ariaTab) assign(id proto.PageFrameID) {
	if _, ok := a.seqs[id]; !ok {
		a.seqs[id] = a.next
		a.next++
	}
}

func (a *ariaTab) addTree(tree *proto.PageFrameTree) {
	a.assign(tree.Frame.ID)
	for _, c := range tree.ChildFrames {
		a.addTree(c)
	}
}

// injectedSource instantiates the injected script like Playwright does.
func injectedSource(frameSeq int) string {
	opts, _ := json.Marshal(map[string]any{
		"isUnderTest": false, "sdkLanguage": "python", "frameSeq": frameSeq,
		"testIdAttributeName": "data-testid", "stableRafCount": 1, "browserName": "chromium",
		"shouldPrependErrorPrefix": true, "isUtilityWorld": true, "customEngines": []any{},
	})
	return "(() => {\nconst module = {};\n" + injectedScript +
		"\nconst injected = new (module.exports.InjectedScript())(globalThis, " + string(opts) + ");" +
		"\ninjected.renderAriaSnapshotAsYaml = renderAriaSnapshotAsYaml;\nreturn injected;\n})()"
}

// world returns the injected script of a frame, creating it in a new isolated
// world when there is none yet or the old one died with a navigation. session
// is where the frame is looked for first: the tab or the parent frame session.
func (t *tab) world(frameID proto.PageFrameID, session *rod.Page) (*ariaWorld, error) {
	a := t.aria
	a.mu.Lock()
	w := a.worlds[frameID]
	a.assign(frameID)
	seq := a.seqs[frameID]
	a.mu.Unlock()
	if w != nil {
		if _, err := w.call(`function() { return 0 }`); err == nil {
			return w, nil
		}
	}
	page := session
	if w != nil {
		page = w.page
	}
	world, err := proto.PageCreateIsolatedWorld{FrameID: frameID, WorldName: "utility", GrantUniveralAccess: true}.Call(page)
	if err != nil {
		// An out-of-process iframe is a target of its own.
		if page, err = t.s.root.PageFromTarget(proto.TargetTargetID(frameID)); err != nil {
			return nil, err
		}
		if world, err = (proto.PageCreateIsolatedWorld{FrameID: frameID, WorldName: "utility", GrantUniveralAccess: true}).Call(page); err != nil {
			return nil, err
		}
	}
	res, err := proto.RuntimeEvaluate{Expression: injectedSource(seq), ContextID: world.ExecutionContextID}.Call(page)
	if err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		return nil, fmt.Errorf("injected script: %s", res.ExceptionDetails.Exception.Description)
	}
	w = &ariaWorld{page: page, obj: res.Result.ObjectID, ctx: world.ExecutionContextID}
	a.mu.Lock()
	a.worlds[frameID] = w
	a.mu.Unlock()
	return w, nil
}

// call runs fn with this = the injected script and returns the value. A
// remote object id argument is passed as that object.
func (w *ariaWorld) call(fn string, args ...any) (*proto.RuntimeRemoteObject, error) {
	var callArgs []*proto.RuntimeCallArgument
	for _, a := range args {
		if id, ok := a.(proto.RuntimeRemoteObjectID); ok {
			callArgs = append(callArgs, &proto.RuntimeCallArgument{ObjectID: id})
			continue
		}
		callArgs = append(callArgs, &proto.RuntimeCallArgument{Value: gson.New(a)})
	}
	res, err := proto.RuntimeCallFunctionOn{
		ObjectID: w.obj, FunctionDeclaration: fn, Arguments: callArgs, ReturnByValue: true,
	}.Call(w.page)
	if err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		return nil, fmt.Errorf("%s", res.ExceptionDetails.Exception.Description)
	}
	return res.Result, nil
}

// refElement returns the remote object of the element behind a ref of the
// latest snapshot in this world, or nil.
func (w *ariaWorld) refElement(ref string) (proto.RuntimeRemoteObjectID, error) {
	res, err := proto.RuntimeCallFunctionOn{
		ObjectID: w.obj,
		FunctionDeclaration: `function(ref) {
			const r = this._lastAriaSnapshotForQuery && this._lastAriaSnapshotForQuery.info.get(ref);
			return r && r.element.isConnected ? r.element : null;
		}`,
		Arguments: []*proto.RuntimeCallArgument{{Value: gson.New(ref)}},
	}.Call(w.page)
	if err != nil {
		return "", err
	}
	if res.ExceptionDetails != nil {
		return "", fmt.Errorf("%s", res.ExceptionDetails.Exception.Description)
	}
	return res.Result.ObjectID, nil
}

// frameSnapshot is the JSON aria snapshot of one frame (ariaSnapshotJSONForFrame
// of Playwright): its own tree with the trees of its iframes merged in. ok is
// false when the frame has no body yet.
func (t *tab) frameSnapshot(frameID proto.PageFrameID, session *rod.Page) (nodes []any, ok bool, err error) {
	w, err := t.world(frameID, session)
	if err != nil {
		return nil, false, err
	}
	res, err := w.call(`function(opts) {
		const root = this.document.querySelector('body,frameset');
		return root ? this.ariaSnapshotJSON(root, opts) : null;
	}`, map[string]any{"mode": "ai"})
	if err != nil {
		return nil, false, err
	}
	return t.mergeSnapshot(w, res)
}

// elementAriaSnapshot is the YAML aria snapshot of one element, taken in the
// injected script of its frame.
func (t *tab) elementAriaSnapshot(el *rod.Element) (string, error) {
	doc, err := proto.RuntimeCallFunctionOn{ObjectID: el.Object.ObjectID,
		FunctionDeclaration: `function() { return this.ownerDocument }`}.Call(el.Page())
	if err != nil {
		return "", err
	}
	docNode, err := proto.DOMDescribeNode{ObjectID: doc.Result.ObjectID}.Call(el.Page())
	if err != nil {
		return "", err
	}
	node, err := proto.DOMDescribeNode{ObjectID: el.Object.ObjectID}.Call(el.Page())
	if err != nil {
		return "", err
	}
	// The main document of a session carries no frame id.
	frameID := docNode.Node.FrameID
	if frameID == "" {
		frameID = el.Page().FrameID
	}
	w, err := t.world(frameID, el.Page())
	if err != nil {
		return "", err
	}
	obj, err := proto.DOMResolveNode{BackendNodeID: node.Node.BackendNodeID, ExecutionContextID: w.ctx}.Call(w.page)
	if err != nil {
		return "", err
	}
	res, err := w.call(`function(opts, root) { return this.ariaSnapshotJSON(root, opts); }`,
		map[string]any{"mode": "ai"}, obj.Object.ObjectID)
	if err != nil {
		return "", err
	}
	nodes, _, err := t.mergeSnapshot(w, res)
	if err != nil {
		return "", err
	}
	out, err := w.call(`function(json) { return this.renderAriaSnapshotAsYaml(json); }`, nodes)
	if err != nil {
		return "", err
	}
	return out.Value.Str(), nil
}

// mergeSnapshot merges the trees of the rendered iframes into the result of
// ariaSnapshotJSON.
func (t *tab) mergeSnapshot(w *ariaWorld, res *proto.RuntimeRemoteObject) (nodes []any, ok bool, err error) {
	if res.Value.Nil() {
		return nil, false, nil
	}
	var snap struct {
		JSON         []any          `json:"json"`
		IframeRefs   []string       `json:"iframeRefs"`
		IframeDepths map[string]int `json:"iframeDepths"`
	}
	if err := json.Unmarshal([]byte(res.Value.JSON("", "")), &snap); err != nil {
		return nil, false, err
	}
	children := map[string][]any{}
	for _, ref := range snap.IframeRefs {
		if _, rendered := snap.IframeDepths[ref]; rendered {
			children[ref] = t.iframeSnapshot(w, ref)
		}
	}
	var merge func(n any)
	merge = func(n any) {
		node, isNode := n.(map[string]any)
		if !isNode {
			return
		}
		if ref, isRef := node["ref"].(string); isRef && node["role"] == "iframe" {
			if c := children[ref]; len(c) > 0 {
				node["children"] = c
			}
			return
		}
		if list, isList := node["children"].([]any); isList {
			for _, c := range list {
				merge(c)
			}
		}
	}
	for _, n := range snap.JSON {
		merge(n)
	}
	return snap.JSON, true, nil
}

// iframeSnapshot is the snapshot of the frame inside an iframe element; any
// failure gives an empty tree, as in Playwright.
func (t *tab) iframeSnapshot(w *ariaWorld, ref string) []any {
	obj, err := w.refElement(ref)
	if err != nil || obj == "" {
		return nil
	}
	node, err := proto.DOMDescribeNode{ObjectID: obj}.Call(w.page)
	if err != nil || node.Node.FrameID == "" {
		return nil
	}
	nodes, _, err := t.frameSnapshot(node.Node.FrameID, w.page)
	if err != nil {
		return nil
	}
	return nodes
}

// ariaSnapshot is Page.aria_snapshot(mode="ai"), retried like Playwright
// while the page has no body or its context is being replaced.
func (t *tab) ariaSnapshot(ms int) (string, error) {
	deadline := time.Now().Add(time.Duration(ms) * time.Millisecond)
	waits := []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for i := 0; ; i++ {
		wait := waits[min(i, len(waits)-1)]
		if time.Now().Add(wait).After(deadline) {
			return "", fmt.Errorf("Page.aria_snapshot: Timeout %dms exceeded.", ms)
		}
		time.Sleep(wait)
		nodes, ok, err := t.frameSnapshot(t.page.FrameID, t.page)
		if err != nil || !ok {
			continue
		}
		w, err := t.world(t.page.FrameID, t.page)
		if err != nil {
			continue
		}
		res, err := w.call(`function(json) { return this.renderAriaSnapshotAsYaml(json); }`, nodes)
		if err != nil {
			continue
		}
		return res.Value.Str(), nil
	}
}

var frameRef = regexp.MustCompile(`^f(\d+)e\d+$`)

// invalidSelector is Playwright's InvalidSelectorError, reported by the
// locator wait with the API name and call log.
type invalidSelector string

func (e invalidSelector) Error() string { return string(e) }

// resolveRef returns the element behind a ref of the latest snapshot (the
// aria-ref selector), as a main world element of its frame; none if the ref
// is unknown or its element is gone. index is the nth() of the locator.
func (t *tab) resolveRef(ref string, index *int) (rod.Elements, error) {
	frameID := t.page.FrameID
	if m := frameRef.FindStringSubmatch(ref); m != nil {
		seq, _ := strconv.Atoi(m[1])
		a := t.aria
		a.mu.Lock()
		frameID = ""
		for id, s := range a.seqs {
			if s == seq {
				frameID = id
			}
		}
		a.mu.Unlock()
		if frameID == "" {
			sel := "aria-ref=" + ref
			if index != nil {
				sel += fmt.Sprintf(" >> nth=%d", *index)
			}
			return nil, invalidSelector(fmt.Sprintf("Invalid frame in aria-ref selector %q", sel))
		}
	}
	w, err := t.world(frameID, t.page)
	if err != nil {
		return nil, err
	}
	obj, err := w.refElement(ref)
	if err != nil || obj == "" {
		return nil, err
	}
	node, err := proto.DOMDescribeNode{ObjectID: obj}.Call(w.page)
	if err != nil {
		return nil, err
	}
	el, err := w.page.ElementFromNode(&proto.DOMNode{BackendNodeID: node.Node.BackendNodeID})
	if err != nil {
		return nil, err
	}
	return rod.Elements{el}, nil
}
