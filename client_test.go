package redpine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

const key = "sk_test_fake_abc123"

type recorded struct {
	Method, Path, Auth string
	Body               map[string]any
}

func stub(t *testing.T, responses ...func(w http.ResponseWriter)) (*[]recorded, *int32) {
	t.Helper()
	var calls []recorded
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := atomic.AddInt32(&n, 1) - 1
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &b)
		}
		calls = append(calls, recorded{r.Method, r.URL.Path, r.Header.Get("Authorization"), b})
		if int(i) < len(responses) {
			responses[i](w)
		} else {
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("REDPINE_BASE_URL", srv.URL)
	t.Setenv("REDPINE_API_KEY", "")
	t.Setenv("CONNECT_API_KEY", "")
	return &calls, &n
}

func ok(body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}
func fail(status int, code string, h map[string]string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		for k, v := range h {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"code":"` + code + `","message":"m","requestId":"r"}}`))
	}
}

const searchOK = `{"results":[{"id":"d1","text":"hello","metadata":null,"collection":null}],"queryId":"q-1","latencyMs":12}`

func TestConfig(t *testing.T) {
	t.Setenv("REDPINE_API_KEY", "")
	t.Setenv("CONNECT_API_KEY", "sk_test_fake_connect")
	if k, _ := resolveAPIKey(""); k != "sk_test_fake_connect" {
		t.Fatal("fallback env")
	}
	t.Setenv("REDPINE_API_KEY", "sk_test_fake_redpine")
	if k, _ := resolveAPIKey(""); k != "sk_test_fake_redpine" {
		t.Fatal("primary env")
	}
	if k, _ := resolveAPIKey("sk_test_fake_arg"); k != "sk_test_fake_arg" {
		t.Fatal("explicit wins")
	}
	t.Setenv("REDPINE_API_KEY", "")
	t.Setenv("CONNECT_API_KEY", "")
	_, err := New()
	var a *AuthError
	if err == nil || !errors.As(err, &a) {
		t.Fatalf("missing key must be AuthError, got %v", err)
	}
	origBaseURL, wasSet := os.LookupEnv("REDPINE_BASE_URL")
	os.Unsetenv("REDPINE_BASE_URL")
	t.Cleanup(func() {
		if wasSet {
			os.Setenv("REDPINE_BASE_URL", origBaseURL)
		} else {
			os.Unsetenv("REDPINE_BASE_URL")
		}
	})
	if v, err := resolveBaseURL(); err != nil || v != DefaultBaseURL || DefaultBaseURL != "https://api.redpine.ai" {
		t.Fatalf("default base url: v=%q err=%v", v, err)
	}
	t.Setenv("REDPINE_BASE_URL", "http://127.0.0.1:8000/")
	if v, err := resolveBaseURL(); err != nil || v != "http://127.0.0.1:8000" {
		t.Fatalf("env base url, trailing slash stripped: v=%q err=%v", v, err)
	}
	t.Setenv("REDPINE_BASE_URL", "")
	if _, err := resolveBaseURL(); err == nil {
		t.Fatal("REDPINE_BASE_URL set but empty must error")
	}
	t.Setenv("REDPINE_BASE_URL", "///")
	if _, err := resolveBaseURL(); err == nil {
		t.Fatal("REDPINE_BASE_URL trimming to empty must error")
	}
}

func TestSearchSendsBearerAndBody(t *testing.T) {
	calls, _ := stub(t, ok(searchOK))
	c, err := New(WithAPIKey(key))
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Search(context.Background(), "crispr", SearchOptions{
		Collections: []string{"corpus"}, Limit: 5,
		Filters: F("issn").Eq("1664-302X").Or(F("issn").Eq("1932-6203")),
	})
	if err != nil || r.QueryId != "q-1" || len(r.Results) == 0 || r.Results[0].Id != "d1" {
		t.Fatalf("bad result %+v %v", r, err)
	}
	got := (*calls)[0]
	if got.Method != "POST" || got.Path != "/api/v1/search/query" || got.Auth != "Bearer "+key {
		t.Fatalf("bad request %+v", got)
	}
	// Image defaults (imageMaxHeight/imageMaxWidth/imageQuality) ride along on every
	// search-family POST — the generated fields are pointers with omitempty, so the client
	// must set them explicitly (mirrors the TS client's IMAGE_DEFAULTS constants).
	want := `{"collections":["corpus"],"filters":{"or":[{"eq":"1664-302X","field":"issn"},{"eq":"1932-6203","field":"issn"}]},"imageMaxHeight":600,"imageMaxWidth":800,"imageQuality":75,"includeFigures":false,"includeMetadata":true,"limit":5,"query":"crispr"}`
	if js(got.Body) != want {
		t.Fatalf("body\n got %s\nwant %s", js(got.Body), want)
	}
}

func TestSearchCollectionPathAndBody(t *testing.T) {
	calls, _ := stub(t, ok(searchOK))
	c, _ := New(WithAPIKey(key))
	if _, err := c.SearchCollection(context.Background(), "corpus", "q", SearchCollectionOptions{}); err != nil {
		t.Fatal(err)
	}
	got := (*calls)[0]
	want := `{"imageMaxHeight":600,"imageMaxWidth":800,"imageQuality":75,"includeFigures":false,"includeMetadata":true,"limit":10,"query":"q"}`
	if got.Path != "/api/v1/search/corpus" || js(got.Body) != want {
		t.Fatalf("bad %+v %s", got, js(got.Body))
	}
}

func TestOtherEndpoints(t *testing.T) {
	calls, _ := stub(t,
		ok(`{"status":"results","queryUnderstanding":{},"results":[],"clarification":null,"billing":{},"queryId":"q-2","latencyMs":1,"iterationsRun":1}`),
		ok(searchOK),
		ok(`{"dailyLimit":10,"dailyUsed":1,"dailyRemaining":9,"monthlyLimit":100,"monthlyUsed":1,"monthlyRemaining":99}`),
		ok(`{"collections":[{"name":"corpus","documentCount":3}],"count":1}`),
	)
	c, _ := New(WithAPIKey(key))
	ctx := context.Background()
	f := false
	a, err := c.AssistedSearch(ctx, "why", AssistedSearchOptions{Collection: "corpus", AllowClarification: &f})
	if err != nil || a.QueryId != "q-2" || (*calls)[0].Body["allowClarification"] != false {
		t.Fatalf("assisted %+v %v", a, err)
	}
	if r, err := c.GetResults(ctx, "q-1"); err != nil || r.QueryId != "q-1" || (*calls)[1].Path != "/api/v1/search/results/q-1" {
		t.Fatalf("get results %v", err)
	}
	if q, err := c.Quota(ctx); err != nil || q.DailyRemaining != 9 {
		t.Fatalf("quota %v", err)
	}
	if l, err := c.Collections(ctx); err != nil || l.Count != 1 {
		t.Fatalf("collections %v", err)
	}
}

func TestSearchRequiresOneTarget(t *testing.T) {
	calls, _ := stub(t, ok(searchOK))
	c, _ := New(WithAPIKey(key))
	if _, err := c.Search(context.Background(), "q", SearchOptions{}); err == nil {
		t.Fatal("no target must error")
	}
	if _, err := c.Search(context.Background(), "q", SearchOptions{Collection: "a", Collections: []string{"b"}}); err == nil {
		t.Fatal("both targets must error")
	}
	if _, err := c.Search(context.Background(), "q", SearchOptions{Collections: []string{}}); err == nil {
		t.Fatal("empty Collections slice must count as absent, not present")
	}
	if _, err := c.Search(context.Background(), "q", SearchOptions{Collection: "a", Collections: []string{}}); err != nil {
		t.Fatalf("empty Collections slice + Collection must not error: %v", err)
	}
	body := (*calls)[len(*calls)-1].Body
	if _, ok := body["collection"]; !ok {
		t.Fatalf("wire body missing collection: %v", body)
	}
	if _, ok := body["collections"]; ok {
		t.Fatalf("wire body must not carry collections when Collections is empty: %v", body)
	}
}

func TestErrorsAndRetries(t *testing.T) {
	zero := map[string]string{"Retry-After": "0"}
	t.Run("403", func(t *testing.T) {
		stub(t, fail(403, "FORBIDDEN", nil))
		c, _ := New(WithAPIKey(key))
		_, err := c.Search(context.Background(), "q", SearchOptions{Collection: "c"})
		var ad *AccessDeniedError
		if !errors.As(err, &ad) || ad.Code != "FORBIDDEN" || ad.RequestID != "r" {
			t.Fatalf("want AccessDeniedError got %v", err)
		}
	})
	t.Run("429 retries then ok", func(t *testing.T) {
		_, n := stub(t, fail(429, "RATE_LIMITED", zero), ok(searchOK))
		c, _ := New(WithAPIKey(key), WithMaxRetries(2))
		if _, err := c.Search(context.Background(), "q", SearchOptions{Collection: "c"}); err != nil || *n != 2 {
			t.Fatalf("err=%v calls=%d", err, *n)
		}
	})
	t.Run("429 exhausts", func(t *testing.T) {
		_, n := stub(t, fail(429, "R", zero), fail(429, "R", zero), fail(429, "R", zero))
		c, _ := New(WithAPIKey(key), WithMaxRetries(2))
		_, err := c.Search(context.Background(), "q", SearchOptions{Collection: "c"})
		var qe *QuotaExceededError
		if !errors.As(err, &qe) || *n != 3 {
			t.Fatalf("want QuotaExceededError after 3 calls; got %v after %d", err, *n)
		}
	})
	t.Run("401 no retry", func(t *testing.T) {
		_, n := stub(t, fail(401, "UNAUTHORIZED", nil))
		c, _ := New(WithAPIKey(key), WithMaxRetries(3))
		_, err := c.Search(context.Background(), "q", SearchOptions{Collection: "c"})
		var ae *AuthError
		if !errors.As(err, &ae) || *n != 1 {
			t.Fatalf("want AuthError after 1 call; got %v after %d", err, *n)
		}
	})
	t.Run("context cancel aborts backoff", func(t *testing.T) {
		stub(t, fail(429, "R", map[string]string{"Retry-After": "30"}))
		c, _ := New(WithAPIKey(key), WithMaxRetries(1))
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if _, err := c.Search(ctx, "q", SearchOptions{Collection: "c"}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want DeadlineExceeded got %v", err)
		}
	})
}
