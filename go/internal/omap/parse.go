package omap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ParseJSON decodes JSON keeping object key order: objects become *Map,
// arrays []any, integers int, other numbers float64.
func ParseJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("extra data after JSON document")
	}
	return v, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		if t == '{' {
			m := New()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				v, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				m.Set(kt.(string), v)
			}
			_, err := dec.Token()
			return m, err
		}
		list := []any{}
		for dec.More() {
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		_, err := dec.Token()
		return list, err
	case json.Number:
		return parseNumber(string(t)), nil
	}
	return tok, nil
}

func parseNumber(s string) any {
	if !strings.ContainsAny(s, ".eE") {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return int(i)
		}
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// ParseYAML decodes a YAML document keeping mapping order. An empty document
// gives nil. Mappings become *Map, sequences []any, scalars int, float64,
// bool, string or nil.
func ParseYAML(data []byte) (any, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 {
		return nil, nil
	}
	return fromNode(&doc)
}

func fromNode(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return fromNode(n.Content[0])
	case yaml.AliasNode:
		return fromNode(n.Alias)
	case yaml.MappingNode:
		m := New()
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, err := fromNode(n.Content[i])
			if err != nil {
				return nil, err
			}
			v, err := fromNode(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m.Set(scalarKey(key), v)
		}
		return m, nil
	case yaml.SequenceNode:
		list := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := fromNode(c)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case time.Time:
		return n.Value, nil
	case uint64:
		return float64(t), nil
	}
	return v, nil
}

func scalarKey(k any) string {
	if s, ok := k.(string); ok {
		return s
	}
	return fmt.Sprint(k)
}
