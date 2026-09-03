package redpine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type fixture struct {
	Op      string         `json:"op"`
	Args    map[string]any `json:"args"`
	Request struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   any    `json:"body"`
	} `json:"request"`
	Response struct {
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
		Body    any               `json:"body"`
	} `json:"response"`
	Expect struct {
		Error      string   `json:"error"`
		RetryAfter *float64 `json:"retry_after"`
		Field      string   `json:"field"`
		Value      any      `json:"value"`
	} `json:"expect"`
}

func str(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}
func strs(m map[string]any, k string) []string {
	l, ok := m[k].([]any)
	if !ok {
		return nil
	}
	out := make([]string, len(l))
	for i, v := range l {
		out[i] = v.(string)
	}
	return out
}
func num(m map[string]any, k string) int {
	if v, ok := m[k].(float64); ok {
		return int(v)
	}
	return 0
}
func bptr(m map[string]any, k string) *bool {
	if v, ok := m[k].(bool); ok {
		return &v
	}
	return nil
}

var errTypes = map[string]func(error) bool{
	"AuthError":           func(e error) bool { var x *AuthError; return errors.As(e, &x) },
	"AccessDenied":        func(e error) bool { var x *AccessDeniedError; return errors.As(e, &x) },
	"NotFound":            func(e error) bool { var x *NotFoundError; return errors.As(e, &x) },
	"ValidationError":     func(e error) bool { var x *ValidationError; return errors.As(e, &x) },
	"QuotaExceeded":       func(e error) bool { var x *QuotaExceededError; return errors.As(e, &x) },
	"AssistedUnavailable": func(e error) bool { var x *AssistedUnavailableError; return errors.As(e, &x) },
}

func TestContractFixtures(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "spec", "fixtures", "*.json"))
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no fixtures found")
	}
	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var fx fixture
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatal(err)
			}
			var gotMethod, gotPath string
			var gotBody any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				b, _ := io.ReadAll(r.Body)
				if len(b) > 0 {
					_ = json.Unmarshal(b, &gotBody)
				}
				for k, v := range fx.Response.Headers {
					w.Header().Set(k, v)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(fx.Response.Status)
				_ = json.NewEncoder(w).Encode(fx.Response.Body)
			}))
			defer srv.Close()
			t.Setenv("REDPINE_BASE_URL", srv.URL)
			c, err := New(WithAPIKey("sk_test_fake_contract"), WithMaxRetries(0))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			a := fx.Args
			var result any
			var callErr error
			switch fx.Op {
			case "search":
				result, callErr = c.Search(ctx, str(a, "query"), SearchOptions{
					Collection: str(a, "collection"), Collections: strs(a, "collections"), Limit: num(a, "limit"),
					Filters: a["filters"], IncludeMetadata: bptr(a, "include_metadata"), IncludeFigures: bptr(a, "include_figures")})
			case "search_collection":
				result, callErr = c.SearchCollection(ctx, str(a, "collection"), str(a, "query"), SearchCollectionOptions{
					Limit: num(a, "limit"), Filters: a["filters"], IncludeMetadata: bptr(a, "include_metadata"), IncludeFigures: bptr(a, "include_figures")})
			case "assisted_search":
				result, callErr = c.AssistedSearch(ctx, str(a, "query"), AssistedSearchOptions{
					Collection: str(a, "collection"), Collections: strs(a, "collections"), Limit: num(a, "limit"),
					Filters: a["filters"], AllowClarification: bptr(a, "allow_clarification"), IncludeMetadata: bptr(a, "include_metadata")})
			case "get_results":
				result, callErr = c.GetResults(ctx, str(a, "query_id"))
			case "quota":
				result, callErr = c.Quota(ctx)
			case "collections":
				result, callErr = c.Collections(ctx)
			default:
				t.Fatalf("unknown op %s", fx.Op)
			}

			if fx.Expect.Error != "" {
				if callErr == nil || !errTypes[fx.Expect.Error](callErr) {
					t.Fatalf("want %s got %v", fx.Expect.Error, callErr)
				}
				if fx.Expect.RetryAfter != nil {
					var q *QuotaExceededError
					if !errors.As(callErr, &q) || q.RetryAfter == nil || *q.RetryAfter != *fx.Expect.RetryAfter {
						t.Fatalf("retry_after mismatch: %+v", q)
					}
				}
			} else {
				if callErr != nil {
					t.Fatal(callErr)
				}
				rb, _ := json.Marshal(result)
				var rm map[string]any
				_ = json.Unmarshal(rb, &rm)
				if !reflect.DeepEqual(rm[fx.Expect.Field], fx.Expect.Value) {
					t.Fatalf("field %s: got %v want %v", fx.Expect.Field, rm[fx.Expect.Field], fx.Expect.Value)
				}
			}
			if gotMethod != fx.Request.Method || gotPath != fx.Request.Path {
				t.Fatalf("request %s %s, want %s %s", gotMethod, gotPath, fx.Request.Method, fx.Request.Path)
			}
			if !reflect.DeepEqual(gotBody, fx.Request.Body) {
				gb, _ := json.Marshal(gotBody)
				wb, _ := json.Marshal(fx.Request.Body)
				t.Fatalf("body\n got %s\nwant %s", gb, wb)
			}
		})
	}
}
