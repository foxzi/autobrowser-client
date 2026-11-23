package api

// jsonError finds where CPython's json.loads (the C scanner of Python 3.12)
// fails on s and returns its message and position in code points, for the
// json_invalid error FastAPI gives. ok is false when Python would parse s.
func jsonError(s []rune) (msg string, pos int, ok bool) {
	p := &pyJSON{s: s}
	defer func() {
		if r := recover(); r != nil {
			e := r.(pyJSONErr)
			msg, pos, ok = e.msg, e.pos, true
		}
	}()
	end := p.value(p.ws(0))
	if end = p.ws(end); end != len(s) {
		panic(pyJSONErr{"Extra data", end})
	}
	return "", 0, false
}

type pyJSONErr struct {
	msg string
	pos int
}

type pyJSON struct{ s []rune }

func (p *pyJSON) fail(msg string, pos int) { panic(pyJSONErr{msg, pos}) }

func (p *pyJSON) ws(i int) int {
	for i < len(p.s) && (p.s[i] == ' ' || p.s[i] == '\t' || p.s[i] == '\n' || p.s[i] == '\r') {
		i++
	}
	return i
}

func (p *pyJSON) at(i int) rune {
	if i < len(p.s) {
		return p.s[i]
	}
	return 0
}

func (p *pyJSON) lit(i int, word string) bool {
	return i+len(word) <= len(p.s) && string(p.s[i:i+len(word)]) == word
}

// value is scan_once_unicode; a StopIteration becomes "Expecting value".
func (p *pyJSON) value(i int) int {
	if i >= len(p.s) {
		p.fail("Expecting value", i)
	}
	switch p.s[i] {
	case '"':
		return p.str(i + 1)
	case '{':
		return p.object(i + 1)
	case '[':
		return p.array(i + 1)
	}
	for _, word := range []string{"null", "true", "false", "NaN", "Infinity", "-Infinity"} {
		if p.lit(i, word) {
			return i + len(word)
		}
	}
	return p.number(i)
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func (p *pyJSON) number(start int) int {
	i := start
	if p.at(i) == '-' {
		i++
	}
	switch c := p.at(i); {
	case c >= '1' && c <= '9':
		for i++; isDigit(p.at(i)); i++ {
		}
	case c == '0':
		i++
	default:
		p.fail("Expecting value", start)
	}
	if p.at(i) == '.' && isDigit(p.at(i+1)) {
		for i += 2; isDigit(p.at(i)); i++ {
		}
	}
	if c := p.at(i); c == 'e' || c == 'E' {
		e := i
		i++
		if c := p.at(i); c == '-' || c == '+' {
			i++
		}
		for isDigit(p.at(i)) {
			i++
		}
		if !isDigit(p.s[i-1]) {
			i = e
		}
	}
	return i
}

func isHex(r rune) bool {
	return isDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func hexVal(r rune) int {
	switch {
	case isDigit(r):
		return int(r - '0')
	case r >= 'a':
		return int(r-'a') + 10
	}
	return int(r-'A') + 10
}

// str is scanstring_unicode from just after the opening quote.
func (p *pyJSON) str(end int) int {
	begin := end - 1
	n := len(p.s)
	for {
		next := end
		for ; next < n && p.s[next] != '"' && p.s[next] != '\\'; next++ {
			if p.s[next] <= 0x1f {
				p.fail("Invalid control character at", next)
			}
		}
		if next == n {
			p.fail("Unterminated string starting at", begin)
		}
		c := p.s[next]
		next++
		if c == '"' {
			return next
		}
		if next == n {
			p.fail("Unterminated string starting at", begin)
		}
		if c = p.s[next]; c != 'u' {
			end = next + 1
			if !(c == '"' || c == '\\' || c == '/' || c == 'b' || c == 'f' || c == 'n' || c == 'r' || c == 't') {
				p.fail("Invalid \\escape", end-2)
			}
			continue
		}
		next++
		end = next + 4
		if end >= n {
			p.fail("Invalid \\uXXXX escape", next-1)
		}
		u := 0
		for ; next < end; next++ {
			if !isHex(p.s[next]) {
				p.fail("Invalid \\uXXXX escape", end-5)
			}
			u = u<<4 | hexVal(p.s[next])
		}
		if u >= 0xd800 && u <= 0xdbff && end+6 < n && p.s[next] == '\\' && p.s[next+1] == 'u' {
			next += 2
			end += 6
			u2 := 0
			for ; next < end; next++ {
				if !isHex(p.s[next]) {
					p.fail("Invalid \\uXXXX escape", end-5)
				}
				u2 = u2<<4 | hexVal(p.s[next])
			}
			if u2 < 0xdc00 || u2 > 0xdfff {
				end -= 6
			}
		}
	}
}

func (p *pyJSON) object(i int) int {
	if i = p.ws(i); p.at(i) == '}' {
		return i + 1
	}
	for {
		if p.at(i) != '"' || i >= len(p.s) {
			p.fail("Expecting property name enclosed in double quotes", i)
		}
		if i = p.ws(p.str(i + 1)); p.at(i) != ':' || i >= len(p.s) {
			p.fail("Expecting ':' delimiter", i)
		}
		i = p.ws(p.value(p.ws(i + 1)))
		if p.at(i) == '}' && i < len(p.s) {
			return i + 1
		}
		if p.at(i) != ',' || i >= len(p.s) {
			p.fail("Expecting ',' delimiter", i)
		}
		i = p.ws(i + 1)
	}
}

func (p *pyJSON) array(i int) int {
	if i = p.ws(i); p.at(i) == ']' {
		return i + 1
	}
	for {
		i = p.ws(p.value(i))
		if p.at(i) == ']' && i < len(p.s) {
			return i + 1
		}
		if p.at(i) != ',' || i >= len(p.s) {
			p.fail("Expecting ',' delimiter", i)
		}
		i = p.ws(i + 1)
	}
}
