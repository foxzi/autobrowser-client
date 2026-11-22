package engine

import (
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// sleepMs is replaced in tests.
var sleepMs = func(ms float64) { time.Sleep(time.Duration(ms * float64(time.Millisecond))) }

func search(pattern, s string) bool {
	re, err := regexp.Compile(pattern)
	return err == nil && re.MatchString(s)
}

func checkVar(c *scenario.Condition, vars *omap.Map) bool {
	value, err := lookup(*c.Var, vars)
	if err != nil {
		return false
	}
	checked := false
	if c.HasEquals {
		checked = true
		if !pyEq(value, c.Equals) {
			return false
		}
	}
	if c.Contains != nil {
		checked = true
		ok := false
		if items, isList := listItems(value); isList {
			for _, item := range items {
				if pyEq(item, *c.Contains) {
					ok = true
					break
				}
			}
		} else if d, isDict := dictEntries(value); isDict {
			_, ok = d[*c.Contains]
		} else {
			ok = strings.Contains(omap.Str(value), *c.Contains)
		}
		if !ok {
			return false
		}
	}
	if c.Matches != nil {
		checked = true
		if !search(*c.Matches, omap.Str(value)) {
			return false
		}
	}
	if !checked {
		return pyTruthy(value)
	}
	return true
}

func checkLeaf(b Browser, c *scenario.Condition, vars *omap.Map, pageID string) bool {
	if c.Var != nil {
		return checkVar(c, vars)
	}
	page, err := b.Page(pageID)
	if err == nil {
		switch {
		case c.Exists != nil:
			var n int
			if n, err = page.Count(*c.Exists); err == nil {
				return n > 0
			}
		case c.Visible != nil:
			var ok bool
			if ok, err = page.IsVisible(*c.Visible); err == nil {
				return ok
			}
		case c.Text != nil:
			var text string
			if text, err = page.InnerText(); err == nil {
				return strings.Contains(text, *c.Text)
			}
		case c.URL != nil:
			var u string
			if u, err = page.URL(); err == nil {
				return strings.Contains(u, *c.URL)
			}
		case c.URLMatches != nil:
			var u string
			if u, err = page.URL(); err == nil {
				return search(*c.URLMatches, u)
			}
		default:
			var title string
			if title, err = page.Title(); err == nil {
				return strings.Contains(title, *c.Title)
			}
		}
	}
	// not found, page navigating, closed page: the check is false
	slog.Debug("condition check failed: " + err.Error())
	return false
}

// CheckCondition evaluates a condition whose variables are already
// substituted. Page errors make a leaf false; a leaf with a timeout is
// polled every 250 ms.
func CheckCondition(b Browser, c *scenario.Condition, vars *omap.Map, pageID string) bool {
	if c.Not != nil {
		return !CheckCondition(b, c.Not, vars, pageID)
	}
	if c.All != nil {
		for _, sub := range c.All {
			if !CheckCondition(b, sub, vars, pageID) {
				return false
			}
		}
		return true
	}
	if c.Any != nil {
		for _, sub := range c.Any {
			if CheckCondition(b, sub, vars, pageID) {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(time.Duration(c.Timeout) * time.Millisecond)
	for !checkLeaf(b, c, vars, pageID) {
		if !time.Now().Before(deadline) {
			return false
		}
		sleepMs(250)
	}
	return true
}
