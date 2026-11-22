package omap

import "testing"

func TestMarshalPythonStyle(t *testing.T) {
	m := New("b", 1, "a", []any{}, "f", 12.0, "s", "é\n\"", "n", nil, "o", New(), "x", New("k", true))
	want := "{\n  \"b\": 1,\n  \"a\": [],\n  \"f\": 12.0,\n  \"s\": \"é\\n\\\"\",\n  \"n\": null,\n  \"o\": {},\n  \"x\": {\n    \"k\": true\n  }\n}"
	if got := Marshal(m, 2); got != want {
		t.Fatalf("%s", got)
	}
	if got := Marshal(New("a", 1, "b", []any{1, "x"}), 0); got != `{"a": 1, "b": [1, "x"]}` {
		t.Fatal(got)
	}
}

func TestFormatFloat(t *testing.T) {
	for f, want := range map[float64]string{1: "1.0", 0.5: "0.5", 1e16: "1e+16", 1.5e-5: "1.5e-05", 123456.789: "123456.789", 1e15: "1000000000000000.0"} {
		if got := FormatFloat(f); got != want {
			t.Errorf("%v: %s want %s", f, got, want)
		}
	}
}

func TestParseKeepsOrder(t *testing.T) {
	for _, parse := range []func([]byte) (any, error){ParseJSON, ParseYAML} {
		v, err := parse([]byte(`{"z": 1, "a": {"y": [1, 2.5, "s", null, true]}, "m": 3}`))
		if err != nil {
			t.Fatal(err)
		}
		if got := Marshal(v, 0); got != `{"z": 1, "a": {"y": [1, 2.5, "s", null, true]}, "m": 3}` {
			t.Fatal(got)
		}
	}
}
