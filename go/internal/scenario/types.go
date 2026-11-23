// Package scenario ports autobrowser_client/scenario.py: loading, shorthand
// and include expansion, and validation of YAML/JSON scenarios.
package scenario

import "github.com/foxzi/autobrowser-client/go/internal/omap"

// Node is a scenario step or a control block.
type Node interface{ node() }

// Locator is a parsed element locator.
type Locator struct {
	Type  string
	Value string
	// Shorthand marks a string with ${...}: it is parsed after substitution.
	Shorthand bool
}

// Dump returns the canonical mapping form of the locator.
func (l *Locator) Dump() *omap.Map {
	m := omap.New("type", l.Type, "value", l.Value)
	if l.Shorthand {
		m.Set("shorthand", true)
	}
	return m
}

// Condition is a check used by if, while and assert: exactly one condition key.
type Condition struct {
	Exists, Visible              *Locator
	Text, URL, URLMatches, Title *string
	Var                          *string
	Equals                       any
	HasEquals                    bool
	Contains, Matches            *string
	Not                          *Condition
	All, Any                     []*Condition // non-nil when the key is present
	Timeout                      int
	// Raw is the canonical mapping (set keys only, in field order); it is what
	// gets variable-substituted and re-parsed, and what assert prints on failure.
	Raw *omap.Map
}

// Step is one action. Pointer fields are nil when not set.
type Step struct {
	Action      string
	Page        string
	URL         string
	Locator     *Locator
	Value       *string
	Key         string
	Format      string
	WaitUntil   string
	Timeout     *int
	DeltaX      *int
	DeltaY      *int
	Delay       *int
	Index       *int
	FullPage    bool
	File        string
	StopOnError *bool
	Options     []string
	Files       []string
	Script      string
	Args        any
	Direction   string
	Text        string
	Gone        bool
	Attribute   string
	All         bool
	X, Y        *float64
	MoveSteps   *int // yaml key "steps"
	StartX      float64
	StartY      float64
	EndX, EndY  *float64
	DurationMs  *int
	Jitter      *float64
	Seed        *int
	AsName      *string // yaml key "as"
	Vars        *omap.Map
	Condition   *Condition
	Message     string
	Name        string
	Retry       int
	RetryDelay  int
	Optional    bool
	// Raw is the canonical mapping (shorthand expanded, locators and
	// conditions in mapping form); the engine substitutes it and re-parses it.
	Raw *omap.Map
}

type IfBlock struct {
	If      *Condition
	Then    []Node
	Else    []Node
	HasElse bool
	Name    string
}

type TryBlock struct {
	Try        []Node
	Catch      []Node
	HasCatch   bool
	Finally    []Node
	HasFinally bool
	Name       string
}

type ForEachBlock struct {
	Items  any // []any or a string such as "${urls}"
	AsName string
	Do     []Node
	Name   string
}

type RepeatBlock struct {
	Count any // int or string
	Do    []Node
	Name  string
}

type WhileBlock struct {
	While *Condition
	Max   int
	Do    []Node
	Name  string
}

func (*Step) node()         {}
func (*IfBlock) node()      {}
func (*TryBlock) node()     {}
func (*ForEachBlock) node() {}
func (*RepeatBlock) node()  {}
func (*WhileBlock) node()   {}

type Viewport struct {
	Width  int
	Height int
}

// FingerprintOverrides are the optional fingerprint settings of a scenario.
type FingerprintOverrides struct {
	Seed                       *int
	Canvas, Webgl              *bool
	WebglVendor, WebglRenderer *string
	Audio, ClientRects, Fonts  *bool
}

type SessionConfig struct {
	Headless        bool
	Profile         string
	ProfileDir      string
	UserAgent       string
	Proxy           string
	Viewport        *Viewport
	Timeout         *int
	PreserveSession bool
	// Fingerprint is nil, a bool or *FingerprintOverrides.
	Fingerprint any
}

// Script is a validated scenario (ScriptDefinition).
type Script struct {
	Session        SessionConfig
	Steps          []Node
	OutputFile     string
	StopOnError    bool
	Vars           *omap.Map
	ErrorArtifacts string
}
