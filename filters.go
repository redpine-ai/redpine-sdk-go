package redpine

import "fmt"

// Filter is an immutable node of the structured filter DSL the search API accepts:
//
//	leaf        {"field": name, "<op>": value}
//	combinator  {"and"|"or"|"not": [nodes]}   (always a list)
//	ops         eq ne in not_in gt gte lt lte between ([lo, hi])
//
//	F("issn").Eq("1664-302X").Or(F("issn").Eq("1932-6203"))
//	F("journal_metric.2yr_mean_citedness").Gte(5).And(F("doi").Eq("10.1/x").Not())
type Filter struct{ node map[string]any }

// Map returns a fresh copy of the wire representation.
func (f Filter) Map() map[string]any { return deepCopy(f.node) }

func deepCopy(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch t := v.(type) {
		case map[string]any:
			out[k] = deepCopy(t)
		case []any:
			cp := make([]any, len(t))
			for i, e := range t {
				if em, ok := e.(map[string]any); ok {
					cp[i] = deepCopy(em)
				} else {
					cp[i] = e
				}
			}
			out[k] = cp
		default:
			out[k] = v
		}
	}
	return out
}

func (f Filter) combine(op string, other Filter) Filter {
	only := func(n map[string]any) ([]any, bool) {
		if len(n) != 1 {
			return nil, false
		}
		l, ok := n[op].([]any)
		return l, ok
	}
	var left, right []any
	if l, ok := only(f.node); ok {
		left = l
	} else {
		left = []any{f.node}
	}
	if r, ok := only(other.node); ok {
		right = r
	} else {
		right = []any{other.node}
	}
	return Filter{map[string]any{op: append(append([]any{}, left...), right...)}}
}

// And combines two filters; chained Ands flatten into one list.
func (f Filter) And(other Filter) Filter { return f.combine("and", other) }

// Or combines two filters; chained Ors flatten into one list.
func (f Filter) Or(other Filter) Filter { return f.combine("or", other) }

// Not negates the filter.
func (f Filter) Not() Filter { return Filter{map[string]any{"not": []any{f.node}}} }

// Field names a filterable field; apply one operator to get a Filter.
type Field struct{ name string }

// F starts a filter on a field: F("issn").Eq("1664-302X").
func F(name string) Field { return Field{name} }

func (fd Field) leaf(op string, v any) Filter {
	return Filter{map[string]any{"field": fd.name, op: v}}
}

func (fd Field) Eq(v any) Filter           { return fd.leaf("eq", v) }
func (fd Field) Ne(v any) Filter           { return fd.leaf("ne", v) }
func (fd Field) In(v ...any) Filter        { return fd.leaf("in", append([]any{}, v...)) }
func (fd Field) NotIn(v ...any) Filter     { return fd.leaf("not_in", append([]any{}, v...)) }
func (fd Field) Gt(v any) Filter           { return fd.leaf("gt", v) }
func (fd Field) Gte(v any) Filter          { return fd.leaf("gte", v) }
func (fd Field) Lt(v any) Filter           { return fd.leaf("lt", v) }
func (fd Field) Lte(v any) Filter          { return fd.leaf("lte", v) }
func (fd Field) Between(lo, hi any) Filter { return fd.leaf("between", []any{lo, hi}) }

// toFilterMap accepts a Filter, a raw map (either DSL form), or nil.
func toFilterMap(v any) (*map[string]any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case Filter:
		m := t.Map()
		return &m, nil
	case map[string]any:
		return &t, nil
	default:
		return nil, fmt.Errorf("filters must be a Filter, map[string]any or nil, got %T", v)
	}
}
