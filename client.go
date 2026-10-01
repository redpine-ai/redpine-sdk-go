package redpine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultBaseURL is the production API. Override only via env REDPINE_BASE_URL.
const DefaultBaseURL = "https://api.redpine.ai"

const (
	envAPIKey         = "REDPINE_API_KEY"
	envAPIKeyFallback = "CONNECT_API_KEY"
	envBaseURL        = "REDPINE_BASE_URL"
)

// Image defaults the search endpoints require on every wire request (not user-configurable
// yet). Generated fields are *int with omitempty, so a nil pointer would silently drop them
// off the wire — mirrors the TS client's IMAGE_DEFAULTS constants. Fixtures in
// spec/fixtures/*.json are authoritative for these exact values.
const (
	imageMaxHeight = 600
	imageMaxWidth  = 800
	imageQuality   = 75
)

func intp(n int) *int { return &n }

func resolveAPIKey(explicit string) (string, error) {
	key := explicit
	if key == "" {
		key = os.Getenv(envAPIKey)
	}
	if key == "" {
		key = os.Getenv(envAPIKeyFallback)
	}
	if key == "" {
		return "", &AuthError{APIError{Status: 401, Message: "no API key: use WithAPIKey or set " + envAPIKey + " (Redpine dashboard > Settings > API Keys)"}}
	}
	return key, nil
}

func resolveBaseURL() (string, error) {
	v, set := os.LookupEnv(envBaseURL)
	if !set {
		return DefaultBaseURL, nil
	}
	trimmed := strings.TrimRight(v, "/")
	if trimmed == "" {
		return "", fmt.Errorf("redpine: %s is set but empty", envBaseURL)
	}
	return trimmed, nil
}

// Redpine is the hand-written client over the generated core. Named Redpine (not Client)
// because generated.go already declares a low-level type Client; mirrors the TS SDK's
// exported Redpine class.
type Redpine struct {
	gen        *ClientWithResponses
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

// Option configures New.
type Option func(*config)

type config struct {
	apiKey     string
	timeout    time.Duration
	maxRetries int
	httpClient *http.Client
}

func WithAPIKey(k string) Option         { return func(c *config) { c.apiKey = k } }
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }
func WithMaxRetries(n int) Option        { return func(c *config) { c.maxRetries = n } }
func WithHTTP(h *http.Client) Option     { return func(c *config) { c.httpClient = h } }

// New builds a Redpine client. Key from WithAPIKey, else REDPINE_API_KEY, else CONNECT_API_KEY.
func New(opts ...Option) (*Redpine, error) {
	cfg := config{timeout: 30 * time.Second, maxRetries: 2}
	for _, o := range opts {
		o(&cfg)
	}
	key, err := resolveAPIKey(cfg.apiKey)
	if err != nil {
		return nil, err
	}
	hc := cfg.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.timeout}
	}
	baseURL, err := resolveBaseURL()
	if err != nil {
		return nil, err
	}
	gen, err := NewClientWithResponses(baseURL,
		WithHTTPClient(hc),
		WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+key)
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	return &Redpine{gen: gen, maxRetries: cfg.maxRetries, sleep: sleepCtx}, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// call runs fn under the retry policy and returns the 2xx body.
func (c *Redpine) call(ctx context.Context, fn func() (*http.Response, []byte, error)) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		resp, body, err := fn()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return body, nil
		}
		apiErr := errorFromResponse(resp.StatusCode, body, resp.Header)
		if shouldRetry(resp.StatusCode, attempt, c.maxRetries) {
			var ra *float64
			if q, ok := apiErr.(*QuotaExceededError); ok {
				ra = q.RetryAfter
			}
			if err := c.sleep(ctx, backoff(attempt, ra, nil)); err != nil {
				return nil, err
			}
			continue
		}
		return nil, apiErr
	}
}

func decode[T any](raw []byte) (*T, error) {
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("redpine: decode response: %w", err)
	}
	return &out, nil
}

func oneTarget(collection string, collections []string) error {
	if (collection == "") == (len(collections) == 0) {
		return fmt.Errorf("redpine: pass exactly one of Collection or Collections")
	}
	return nil
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func limitOr(n int) int {
	if n == 0 {
		return 10
	}
	return n
}

// SearchOptions for Search. Exactly one of Collection / Collections is required.
type SearchOptions struct {
	Collection      string
	Collections     []string
	Limit           int
	Filters         any // Filter, map[string]any, or nil
	IncludeMetadata *bool
	IncludeFigures  *bool
}

// SearchCollectionOptions for SearchCollection.
type SearchCollectionOptions struct {
	Limit           int
	Filters         any
	IncludeMetadata *bool
	IncludeFigures  *bool
}

// AssistedSearchOptions for AssistedSearch. Exactly one of Collection / Collections is
// required. IncludeFigures is not offered here: the API always forces it false for assisted
// search (matches the TS client), so there is no user-facing knob for it.
type AssistedSearchOptions struct {
	Collection         string
	Collections        []string
	Limit              int
	Filters            any
	AllowClarification *bool
	IncludeMetadata    *bool
}

func (c *Redpine) Search(ctx context.Context, query string, o SearchOptions) (*SearchResponse, error) {
	if err := oneTarget(o.Collection, o.Collections); err != nil {
		return nil, err
	}
	filters, err := toFilterMap(o.Filters)
	if err != nil {
		return nil, err
	}
	body := SearchQueryJSONRequestBody{Query: query}
	if o.Collection != "" {
		body.Collection = &o.Collection
	}
	if len(o.Collections) > 0 {
		cs := append([]string{}, o.Collections...)
		body.Collections = &cs
	}
	lim, im, ifg := limitOr(o.Limit), boolOr(o.IncludeMetadata, true), boolOr(o.IncludeFigures, false)
	body.Limit, body.IncludeMetadata, body.IncludeFigures, body.Filters = &lim, &im, &ifg, filters
	body.ImageMaxHeight, body.ImageMaxWidth, body.ImageQuality = intp(imageMaxHeight), intp(imageMaxWidth), intp(imageQuality)
	raw, err := c.call(ctx, func() (*http.Response, []byte, error) {
		r, err := c.gen.SearchQueryWithResponse(ctx, body)
		if err != nil {
			return nil, nil, err
		}
		return r.HTTPResponse, r.Body, nil
	})
	if err != nil {
		return nil, err
	}
	return decode[SearchResponse](raw)
}

func (c *Redpine) SearchCollection(ctx context.Context, collection, query string, o SearchCollectionOptions) (*SearchResponse, error) {
	filters, err := toFilterMap(o.Filters)
	if err != nil {
		return nil, err
	}
	lim, im, ifg := limitOr(o.Limit), boolOr(o.IncludeMetadata, true), boolOr(o.IncludeFigures, false)
	body := SearchCollectionJSONRequestBody{Query: query, Limit: &lim, IncludeMetadata: &im, IncludeFigures: &ifg, Filters: filters}
	body.ImageMaxHeight, body.ImageMaxWidth, body.ImageQuality = intp(imageMaxHeight), intp(imageMaxWidth), intp(imageQuality)
	raw, err := c.call(ctx, func() (*http.Response, []byte, error) {
		r, err := c.gen.SearchCollectionWithResponse(ctx, collection, body)
		if err != nil {
			return nil, nil, err
		}
		return r.HTTPResponse, r.Body, nil
	})
	if err != nil {
		return nil, err
	}
	return decode[SearchResponse](raw)
}

func (c *Redpine) AssistedSearch(ctx context.Context, query string, o AssistedSearchOptions) (*AssistedSearchResponse, error) {
	if err := oneTarget(o.Collection, o.Collections); err != nil {
		return nil, err
	}
	filters, err := toFilterMap(o.Filters)
	if err != nil {
		return nil, err
	}
	body := SearchAssistedJSONRequestBody{Query: query}
	if o.Collection != "" {
		body.Collection = &o.Collection
	}
	if len(o.Collections) > 0 {
		cs := append([]string{}, o.Collections...)
		body.Collections = &cs
	}
	lim, ac, im, ifg := limitOr(o.Limit), boolOr(o.AllowClarification, true), boolOr(o.IncludeMetadata, true), false
	body.Limit, body.AllowClarification, body.IncludeMetadata, body.IncludeFigures, body.Filters = &lim, &ac, &im, &ifg, filters
	body.ImageMaxHeight, body.ImageMaxWidth, body.ImageQuality = intp(imageMaxHeight), intp(imageMaxWidth), intp(imageQuality)
	raw, err := c.call(ctx, func() (*http.Response, []byte, error) {
		r, err := c.gen.SearchAssistedWithResponse(ctx, body)
		if err != nil {
			return nil, nil, err
		}
		return r.HTTPResponse, r.Body, nil
	})
	if err != nil {
		return nil, err
	}
	return decode[AssistedSearchResponse](raw)
}

// PreviewOptions configures Preview. Exactly one of Collection / Collections.
type PreviewOptions struct {
	Collection  string
	Collections []string
	Limit       int
	Filters     any // Filter, map[string]any, or nil
}

// Preview is free: teaser rows plus the cost to unlock each. Never charged, never a quota slot.
func (c *Redpine) Preview(ctx context.Context, query string, o PreviewOptions) (*PreviewUnlockResponse, error) {
	if err := oneTarget(o.Collection, o.Collections); err != nil {
		return nil, err
	}
	filters, err := toFilterMap(o.Filters)
	if err != nil {
		return nil, err
	}
	body := SearchPreviewJSONRequestBody{Query: query}
	if o.Collection != "" {
		body.Collection = &o.Collection
	}
	if len(o.Collections) > 0 {
		cs := append([]string{}, o.Collections...)
		body.Collections = &cs
	}
	lim := limitOr(o.Limit)
	body.Limit, body.Filters = &lim, filters
	raw, err := c.call(ctx, func() (*http.Response, []byte, error) {
		r, err := c.gen.SearchPreviewWithResponse(ctx, body)
		if err != nil {
			return nil, nil, err
		}
		return r.HTTPResponse, r.Body, nil
	})
	if err != nil {
		return nil, err
	}
	return decode[PreviewUnlockResponse](raw)
}

// Unlock pays for previewed rows. A nil resultIDs unlocks every row; re-sending paid ids is free.
func (c *Redpine) Unlock(ctx context.Context, queryID string, resultIDs []string) (*PreviewUnlockResponse, error) {
	body := SearchUnlockJSONRequestBody{QueryId: queryID}
	if resultIDs != nil {
		ids := append([]string{}, resultIDs...)
		body.ResultIds = &ids
	}
	raw, err := c.call(ctx, func() (*http.Response, []byte, error) {
		r, err := c.gen.SearchUnlockWithResponse(ctx, body)
		if err != nil {
			return nil, nil, err
		}
		return r.HTTPResponse, r.Body, nil
	})
	if err != nil {
		return nil, err
	}
	return decode[PreviewUnlockResponse](raw)
}

func (c *Redpine) GetResults(ctx context.Context, queryID string) (*SearchResultsPreviewResponse, error) {
	raw, err := c.call(ctx, func() (*http.Response, []byte, error) {
		// Image options aren't exposed on GetResults yet (matches Unlock, matches TS); nil omits them all.
		r, err := c.gen.GetCachedResultWithResponse(ctx, queryID, nil)
		if err != nil {
			return nil, nil, err
		}
		return r.HTTPResponse, r.Body, nil
	})
	if err != nil {
		return nil, err
	}
	return decode[SearchResultsPreviewResponse](raw)
}

func (c *Redpine) Quota(ctx context.Context) (*QuotaInfo, error) {
	raw, err := c.call(ctx, func() (*http.Response, []byte, error) {
		r, err := c.gen.GetQuotaWithResponse(ctx)
		if err != nil {
			return nil, nil, err
		}
		return r.HTTPResponse, r.Body, nil
	})
	if err != nil {
		return nil, err
	}
	return decode[QuotaInfo](raw)
}

func (c *Redpine) Collections(ctx context.Context) (*CollectionsResponse, error) {
	raw, err := c.call(ctx, func() (*http.Response, []byte, error) {
		r, err := c.gen.ListCollectionsWithResponse(ctx)
		if err != nil {
			return nil, nil, err
		}
		return r.HTTPResponse, r.Body, nil
	})
	if err != nil {
		return nil, err
	}
	return decode[CollectionsResponse](raw)
}
