package browser

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/go-rod/rod/lib/input"
)

var namedKeys = map[string]input.Key{
	"enter": input.Enter, "return": input.Enter, "tab": input.Tab, "escape": input.Escape, "esc": input.Escape,
	"backspace": input.Backspace, "delete": input.Delete, "insert": input.Insert,
	"arrowleft": input.ArrowLeft, "arrowright": input.ArrowRight, "arrowup": input.ArrowUp, "arrowdown": input.ArrowDown,
	"home": input.Home, "end": input.End, "pageup": input.PageUp, "pagedown": input.PageDown,
	"space":   input.Space,
	"control": input.ControlLeft, "ctrl": input.ControlLeft, "shift": input.ShiftLeft,
	"alt": input.AltLeft, "meta": input.MetaLeft, "cmd": input.MetaLeft,
	"f1": input.F1, "f2": input.F2, "f3": input.F3, "f4": input.F4, "f5": input.F5, "f6": input.F6,
	"f7": input.F7, "f8": input.F8, "f9": input.F9, "f10": input.F10, "f11": input.F11, "f12": input.F12,
}

// known reports whether rod has a description for the key (Key.Info panics otherwise).
func known(k input.Key) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	k.Info()
	return true
}

// parseKey turns "Control+a" into the keys to press in order.
func parseKey(spec string) ([]input.Key, error) {
	parts := strings.Split(spec, "+")
	if spec == "+" {
		parts = []string{"+"}
	}
	var keys []input.Key
	for _, p := range parts {
		if k, ok := namedKeys[strings.ToLower(p)]; ok {
			keys = append(keys, k)
			continue
		}
		r, size := utf8.DecodeRuneInString(p)
		if size != len(p) || p == "" || !known(input.Key(r)) {
			return nil, fmt.Errorf("Unknown key: %q", p)
		}
		keys = append(keys, input.Key(r))
	}
	return keys, nil
}

// rodKey is the key of a typed character, or 0 when rod has no key for it.
func rodKey(r rune) input.Key {
	if r == '\r' || known(input.Key(r)) {
		return input.Key(r)
	}
	return 0
}
