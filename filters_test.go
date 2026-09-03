package redpine

import (
	"encoding/json"
	"testing"
)

func js(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestLeafOps(t *testing.T) {
	cases := map[string]Filter{
		`{"eq":"a","field":"f"}`:              F("f").Eq("a"),
		`{"field":"f","ne":"x"}`:              F("f").Ne("x"),
		`{"field":"f","in":["a","b"]}`:        F("f").In("a", "b"),
		`{"field":"f","not_in":["a"]}`:        F("f").NotIn("a"),
		`{"field":"f","gt":1}`:                F("f").Gt(1),
		`{"field":"f","gte":5}`:               F("f").Gte(5),
		`{"field":"f","lt":2}`:                F("f").Lt(2),
		`{"field":"f","lte":3}`:               F("f").Lte(3),
		`{"between":[2020,2024],"field":"y"}`: F("y").Between(2020, 2024),
	}
	for want, f := range cases {
		if got := js(f.Map()); got != want {
			t.Fatalf("want %s got %s", want, got)
		}
	}
}

func TestCombinators(t *testing.T) {
	if got := js(F("issn").Eq("a").Or(F("issn").Eq("b")).Map()); got != `{"or":[{"eq":"a","field":"issn"},{"eq":"b","field":"issn"}]}` {
		t.Fatal(got)
	}
	if got := js(F("j").Eq("N").And(F("y").Gte(2020)).Map()); got != `{"and":[{"eq":"N","field":"j"},{"field":"y","gte":2020}]}` {
		t.Fatal(got)
	}
	if got := js(F("doi").Eq("x").Not().Map()); got != `{"not":[{"eq":"x","field":"doi"}]}` {
		t.Fatal(got)
	}
}

func TestFlattenAndNest(t *testing.T) {
	if got := js(F("a").Eq(1).And(F("b").Eq(2)).And(F("c").Eq(3)).Map()); got != `{"and":[{"eq":1,"field":"a"},{"eq":2,"field":"b"},{"eq":3,"field":"c"}]}` {
		t.Fatal(got)
	}
	if got := js(F("a").Eq(1).Or(F("b").Eq(2)).And(F("c").Eq(3).Not()).Map()); got != `{"and":[{"or":[{"eq":1,"field":"a"},{"eq":2,"field":"b"}]},{"not":[{"eq":3,"field":"c"}]}]}` {
		t.Fatal(got)
	}
}

func TestToFilterMap(t *testing.T) {
	raw := map[string]any{"journal": "Nature"}
	m, err := toFilterMap(raw)
	if err != nil || m == nil || (*m)["journal"] != "Nature" {
		t.Fatalf("passthrough failed: %v %v", m, err)
	}
	if m, err := toFilterMap(nil); err != nil || m != nil {
		t.Fatalf("nil should be nil,nil")
	}
	if m, err := toFilterMap(F("x").Eq(1)); err != nil || js(*m) != `{"eq":1,"field":"x"}` {
		t.Fatalf("filter failed: %v %v", m, err)
	}
	if _, err := toFilterMap("issn=1"); err == nil {
		t.Fatal("string should be rejected")
	}
}

func TestMapReturnsIsolatedCopy(t *testing.T) {
	// Build a nested filter: (a=1) OR (b IN [x,y])
	original := F("a").Eq(1).Or(F("b").In("x", "y"))
	originalJSON := js(original.Map())

	// Get a copy and mutate it deeply
	m1 := original.Map()
	if m1["or"] == nil {
		t.Fatal("expected or field")
	}
	orSlice := m1["or"].([]any)
	// Mutate the first OR operand's eq value
	orSlice[0].(map[string]any)["eq"] = 99
	// Append to the nested in slice
	inSlice := orSlice[1].(map[string]any)["in"].([]any)
	orSlice[1].(map[string]any)["in"] = append(inSlice, "z")

	// Verify original Filter is unchanged
	if js(original.Map()) != originalJSON {
		t.Fatalf("original filter was mutated: %s != %s", js(original.Map()), originalJSON)
	}
	// Verify a second Map() call returns the same result
	if js(original.Map()) != originalJSON {
		t.Fatalf("second Map() call differs: %s != %s", js(original.Map()), originalJSON)
	}
}
