// Package mcp is the MCP stdio server (port of mcp_server.py): one browser
// session per server process, started lazily.
package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/browser"
	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/engine"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

const instructions = `Stealth browser (patchright / cloakbrowser) for sites with anti-bot protection.
Workflow: browser_navigate -> browser_snapshot -> act on elements by their ref
(e.g. target="e12") -> browser_snapshot again to see the result.
Refs come from the latest snapshot and go stale after the page changes.
Use other locators with by="css" | "xpath" | "text" | ... when needed.
On a Cloudflare "Just a moment" page call browser_solve_challenge.
page_id is optional everywhere: the first tab is used by default.
`

// toolsJSON is the tool list exactly as the Python server lists it
// (names, descriptions, pydantic input and output schemas).
//
//go:embed tools.json
var toolsJSON []byte

// toolError is a failure reported to the model as is (ToolError).
type toolError string

func (e toolError) Error() string { return string(e) }

// result is a tool's output: text, or a PNG for the screenshot tool.
type result struct {
	text string
	png  []byte
}

type toolFunc func(args *omap.Map) (result, error)

// Server is the MCP server with its browser session.
type Server struct {
	settings *config.Settings
	profile  string
	mu       sync.Mutex
	sess     *browser.Session
	sdk      *sdk.Server
}

// New builds the server (create_server); the browser starts on the first tool call.
func New(settings *config.Settings, profile string) *Server {
	s := &Server{settings: settings, profile: profile}
	s.sdk = sdk.NewServer(&sdk.Implementation{Name: "autobrowser"}, &sdk.ServerOptions{
		Instructions: instructions,
		Capabilities: &sdk.ServerCapabilities{
			Prompts:   &sdk.PromptCapabilities{},
			Resources: &sdk.ResourceCapabilities{},
			Tools:     &sdk.ToolCapabilities{},
		},
	})
	var defs []struct {
		Name         string          `json:"name"`
		Description  string          `json:"description"`
		InputSchema  json.RawMessage `json:"inputSchema"`
		OutputSchema json.RawMessage `json:"outputSchema"`
	}
	if err := json.Unmarshal(toolsJSON, &defs); err != nil {
		panic(err)
	}
	funcs := s.tools()
	for _, d := range defs {
		schema, err := omap.ParseJSON(d.InputSchema)
		if err != nil {
			panic(err)
		}
		tool := &sdk.Tool{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema}
		if d.OutputSchema != nil {
			tool.OutputSchema = d.OutputSchema
		}
		s.sdk.AddTool(tool, handler(d.Name, schema.(*omap.Map), funcs[d.Name]))
	}
	s.sdk.AddReceivingMiddleware(unknownTool(funcs))
	return s
}

// Run serves the transport until the client goes away or ctx ends, then
// closes the browser (run_server).
func (s *Server) Run(ctx context.Context, t sdk.Transport) error {
	defer s.closeBrowser()
	return s.sdk.Run(ctx, t)
}

func handler(name string, schema *omap.Map, fn toolFunc) sdk.ToolHandler {
	return func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		args := omap.New()
		if raw := req.Params.Arguments; len(raw) > 0 {
			v, err := omap.ParseJSON(raw)
			if err != nil {
				return nil, err
			}
			if m, ok := v.(*omap.Map); ok {
				args = m
			}
		}
		err := validateArgs(name, schema, args)
		var out result
		if err == nil {
			out, err = fn(args)
		}
		if err != nil {
			return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{
				&sdk.TextContent{Text: fmt.Sprintf("Error executing tool %s: %s", name, err)}}}, nil
		}
		if out.png != nil {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.ImageContent{Data: out.png, MIMEType: "image/png"}}}, nil
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: out.text}},
			StructuredContent: map[string]string{"result": out.text}}, nil
	}
}

// unknownTool answers a call of a missing tool with an error result like
// Python, not with a protocol error.
func unknownTool(funcs map[string]toolFunc) sdk.Middleware {
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			if p, ok := req.GetParams().(*sdk.CallToolParamsRaw); ok && method == "tools/call" && funcs[p.Name] == nil {
				return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{
					&sdk.TextContent{Text: "Unknown tool: " + p.Name}}}, nil
			}
			return next(ctx, method, req)
		}
	}
}

// session is the running browser, started (or restarted when it died or
// has no tabs left) on demand.
func (s *Server) session() (*browser.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sess != nil && s.sess.State() == browser.StateRunning && s.sess.ListPages().Len() > 0 {
		return s.sess, nil
	}
	if s.sess != nil {
		s.sess.Close(false)
		s.sess = nil
	}
	sess, err := browser.Start(s.settings, browser.Options{Profile: s.profile})
	if err != nil {
		return nil, toolError("Browser start failed: " + err.Error())
	}
	s.sess = sess
	return sess, nil
}

func (s *Server) closeBrowser() {
	s.mu.Lock()
	sess := s.sess
	s.sess = nil
	s.mu.Unlock()
	if sess != nil {
		sess.Close(false)
	}
}

// run runs fn on the session and words its failure for the model.
func (s *Server) run(fn func(*browser.Session) (string, error)) (result, error) {
	sess, err := s.session()
	if err != nil {
		return result{}, err
	}
	var text string
	err = sess.Call(func() error {
		var err error
		text, err = fn(sess)
		return err
	})
	return result{text: text}, toolErr(err)
}

func toolErr(err error) error {
	var te toolError
	var ae *apperr.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &te):
		return te
	case errors.As(err, &ae):
		return toolError(fmt.Sprintf("%s: %s", ae.Code, ae.Message))
	}
	// patchright prefixes messages with an empty API name: ": Timeout ..."
	msg := []rune(strings.TrimPrefix(err.Error(), ": "))
	if len(msg) > 2000 {
		msg = msg[:2000]
	}
	return toolError(string(msg))
}

// pageIDOf is the given tab or the first one.
func pageIDOf(sess *browser.Session, pageID string) string {
	if pageID != "" {
		return pageID
	}
	if keys := sess.ListPages().Keys(); len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// pageInfo is the {pageId, url, title} JSON the element actions return.
func pageInfo(sess *browser.Session, pageID string) (string, error) {
	p, err := sess.Page(pageID)
	if err != nil {
		return "", err
	}
	u, err := p.URL()
	if err != nil {
		return "", err
	}
	title, err := p.Title()
	if err != nil {
		return "", err
	}
	return dumpsASCII(omap.New("pageId", pageID, "url", u, "title", title)), nil
}

// dumpsASCII is json.dumps(v) with the default ensure_ascii=True.
func dumpsASCII(v any) string {
	var sb strings.Builder
	for _, r := range omap.Marshal(v, 0) {
		switch {
		case r < 0x80:
			sb.WriteRune(r)
		case r > 0xffff:
			r -= 0x10000
			fmt.Fprintf(&sb, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
		default:
			fmt.Fprintf(&sb, `\u%04x`, r)
		}
	}
	return sb.String()
}

func str(args *omap.Map, key string) string {
	v, _ := args.Get(key)
	s, _ := v.(string)
	return s
}

func strPtr(args *omap.Map, key string) *string {
	if v, _ := args.Get(key); v != nil {
		s := v.(string)
		return &s
	}
	return nil
}

func boolean(args *omap.Map, key string) bool {
	v, _ := args.Get(key)
	b, _ := v.(bool)
	return b
}

func integer(args *omap.Map, key string) int {
	v, _ := args.Get(key)
	n, _ := v.(int)
	return n
}

func stringList(args *omap.Map, key string) []string {
	v, _ := args.Get(key)
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func target(args *omap.Map) *scenario.Locator {
	return &scenario.Locator{Type: str(args, "by"), Value: str(args, "target")}
}

// act runs element steps on the tab and returns its pageInfo.
func (s *Server) act(args *omap.Map, steps ...*scenario.Step) (result, error) {
	return s.run(func(sess *browser.Session) (string, error) {
		pid := pageIDOf(sess, str(args, "page_id"))
		for _, step := range steps {
			if _, err := sess.Do(step, pid); err != nil {
				return "", err
			}
		}
		return pageInfo(sess, pid)
	})
}

func (s *Server) tools() map[string]toolFunc {
	actionMs := s.settings.Timeouts.ActionMs
	navMs := s.settings.Timeouts.NavigationMs
	return map[string]toolFunc{
		"browser_navigate": func(a *omap.Map) (result, error) {
			return s.run(func(sess *browser.Session) (string, error) {
				m, err := sess.Navigate(pageIDOf(sess, str(a, "page_id")), str(a, "url"), navMs)
				return dumpsASCII(m), err
			})
		},
		"browser_history": func(a *omap.Map) (result, error) {
			return s.run(func(sess *browser.Session) (string, error) {
				m, err := sess.History(pageIDOf(sess, str(a, "page_id")), str(a, "direction"), navMs)
				return dumpsASCII(m), err
			})
		},
		"browser_snapshot": func(a *omap.Map) (result, error) {
			return s.run(func(sess *browser.Session) (string, error) {
				snap, err := sess.Snapshot(pageIDOf(sess, str(a, "page_id")), actionMs)
				if err != nil {
					return "", err
				}
				text := fmt.Sprintf("URL: %s\nTitle: %s\n%s", snap.URL, snap.Title, snap.Snapshot)
				if snap.Truncated {
					text += "\n[truncated]"
				}
				return text, nil
			})
		},
		"browser_screenshot": func(a *omap.Map) (result, error) {
			var png []byte
			_, err := s.run(func(sess *browser.Session) (string, error) {
				var loc *scenario.Locator
				if str(a, "target") != "" {
					loc = target(a)
				}
				var err error
				png, err = sess.Screenshot(pageIDOf(sess, str(a, "page_id")), boolean(a, "full_page"), loc, actionMs)
				return "", err
			})
			return result{png: png}, err
		},
		"browser_click": func(a *omap.Map) (result, error) {
			action := "click"
			if boolean(a, "double") {
				action = "dblclick"
			}
			return s.act(a, &scenario.Step{Action: action, Locator: target(a), Timeout: &actionMs})
		},
		"browser_hover": func(a *omap.Map) (result, error) {
			return s.act(a, &scenario.Step{Action: "hover", Locator: target(a), Timeout: &actionMs})
		},
		"browser_type": func(a *omap.Map) (result, error) {
			text := str(a, "text")
			steps := []*scenario.Step{{Action: "fill", Locator: target(a), Value: &text, Timeout: &actionMs}}
			if boolean(a, "submit") {
				steps = append(steps, &scenario.Step{Action: "press", Locator: target(a), Key: "Enter", Timeout: &actionMs})
			}
			return s.act(a, steps...)
		},
		"browser_press_key": func(a *omap.Map) (result, error) {
			return s.act(a, &scenario.Step{Action: "press", Key: str(a, "key")})
		},
		"browser_select_option": func(a *omap.Map) (result, error) {
			return s.act(a, &scenario.Step{Action: "select", Locator: target(a),
				Options: stringList(a, "values"), Timeout: &actionMs})
		},
		"browser_file_upload": func(a *omap.Map) (result, error) {
			return s.act(a, &scenario.Step{Action: "upload", Locator: target(a),
				Files: stringList(a, "paths"), Timeout: &actionMs})
		},
		"browser_evaluate": func(a *omap.Map) (result, error) {
			return s.run(func(sess *browser.Session) (string, error) {
				data, err := sess.Do(&scenario.Step{Action: "evaluate", Script: str(a, "expression")},
					pageIDOf(sess, str(a, "page_id")))
				if err != nil {
					return "", err
				}
				v, _ := data.(*omap.Map).Get("result")
				return omap.Marshal(v, 0), nil
			})
		},
		"browser_wait_for": func(a *omap.Map) (result, error) {
			ms := integer(a, "timeout_ms")
			return s.act(a, &scenario.Step{Action: "wait-text", Text: str(a, "text"),
				Gone: boolean(a, "gone"), Timeout: &ms})
		},
		"browser_get_text": func(a *omap.Map) (result, error) {
			return s.run(func(sess *browser.Session) (string, error) {
				return sess.Content(pageIDOf(sess, str(a, "page_id")), "text")
			})
		},
		"browser_tabs": func(a *omap.Map) (result, error) {
			return s.run(func(sess *browser.Session) (string, error) {
				switch str(a, "action") {
				case "new":
					if _, err := sess.NewTab(str(a, "url")); err != nil {
						return "", err
					}
				case "close":
					if str(a, "page_id") == "" {
						return "", toolError("page_id is required to close a tab")
					}
					if err := sess.CloseTab(str(a, "page_id")); err != nil {
						return "", err
					}
				}
				return dumpsASCII(sess.ListPages()), nil
			})
		},
		"browser_logs": func(a *omap.Map) (result, error) {
			return s.run(func(sess *browser.Session) (string, error) {
				items, err := sess.Log(pageIDOf(sess, str(a, "page_id")), str(a, "kind"), boolean(a, "clear"))
				return omap.Marshal(items, 0), err
			})
		},
		"browser_handle_dialogs": func(a *omap.Map) (result, error) {
			sess, err := s.session()
			if err != nil {
				return result{}, err
			}
			sess.SetDialogPolicy(boolean(a, "accept"), strPtr(a, "prompt_text"))
			return result{text: dumpsASCII(sess.DialogPolicy())}, nil
		},
		"browser_solve_challenge": func(a *omap.Map) (result, error) {
			return s.run(func(sess *browser.Session) (string, error) {
				m, err := sess.SolveChallenge(str(a, "page_id"), integer(a, "timeout_ms"))
				return dumpsASCII(m), err
			})
		},
		"browser_run_script": s.runScript,
		"browser_close": func(*omap.Map) (result, error) {
			s.closeBrowser()
			return result{text: "closed"}, nil
		},
	}
}

func (s *Server) runScript(a *omap.Map) (result, error) {
	script, file := strPtr(a, "script"), strPtr(a, "file")
	if (script == nil) == (file == nil) {
		return result{}, toolError("Pass exactly one of script or file")
	}
	var def *scenario.Script
	var err error
	if file != nil {
		def, err = scenario.Load(*file)
	} else {
		cwd, _ := os.Getwd()
		def, err = scenario.Parse(*script, cwd)
	}
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return result{}, toolErr(err)
	} else if err != nil {
		msg := []rune("Invalid script: " + err.Error())
		if len(msg) > 2000 {
			msg = msg[:2000]
		}
		return result{}, toolError(string(msg))
	}
	var vars *omap.Map
	if v, _ := a.Get("vars"); v != nil {
		vars = v.(*omap.Map)
	}
	return s.run(func(sess *browser.Session) (string, error) {
		res, err := engine.Run(sess, def, engine.Options{PageID: str(a, "page_id"), Vars: vars})
		if err != nil {
			return "", err
		}
		return omap.Marshal(res.Map(), 0), nil
	})
}
