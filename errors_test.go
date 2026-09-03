package redpine

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func body(code, msg, rid string) []byte {
	return []byte(`{"error":{"code":"` + code + `","message":"` + msg + `","requestId":"` + rid + `"}}`)
}

func TestErrorFromResponseMapsStatus(t *testing.T) {
	cases := []struct {
		status int
		check  func(error) bool
	}{
		{401, func(e error) bool { var x *AuthError; return errors.As(e, &x) }},
		{402, func(e error) bool { var x *InsufficientCreditsError; return errors.As(e, &x) }},
		{403, func(e error) bool { var x *AccessDeniedError; return errors.As(e, &x) }},
		{404, func(e error) bool { var x *NotFoundError; return errors.As(e, &x) }},
		{410, func(e error) bool { var x *ExpiredError; return errors.As(e, &x) }},
		{422, func(e error) bool { var x *ValidationError; return errors.As(e, &x) }},
		{429, func(e error) bool { var x *QuotaExceededError; return errors.As(e, &x) }},
		{503, func(e error) bool { var x *AssistedUnavailableError; return errors.As(e, &x) }},
	}
	for _, c := range cases {
		err := errorFromResponse(c.status, body("X", "msg", "req-1"), nil)
		if !c.check(err) {
			t.Fatalf("status %d mapped to %T", c.status, err)
		}
		var base *APIError
		if !errors.As(err, &base) {
			t.Fatalf("status %d: not an *APIError", c.status)
		}
		if base.Status != c.status || base.Code != "X" || base.Message != "msg" || base.RequestID != "req-1" {
			t.Fatalf("status %d: fields %+v", c.status, base)
		}
	}
}

func TestErrorFromResponseUnknownStatusIsBase(t *testing.T) {
	if _, ok := errorFromResponse(500, body("X", "m", "r"), nil).(*APIError); !ok {
		t.Fatal("500 should be *APIError")
	}
}

func TestErrorFromResponseNonJSON(t *testing.T) {
	err := errorFromResponse(502, []byte("<html>"), nil)
	var base *APIError
	if !errors.As(err, &base) || base.Code != "" || !strings.Contains(err.Error(), "502") {
		t.Fatalf("bad: %v", err)
	}
}

func TestQuotaExceededRetryAfter(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "7")
	var q *QuotaExceededError
	if !errors.As(errorFromResponse(429, body("R", "m", "r"), h), &q) || q.RetryAfter == nil || *q.RetryAfter != 7 {
		t.Fatalf("retry-after not parsed: %+v", q)
	}
	if !errors.As(errorFromResponse(429, body("R", "m", "r"), nil), &q) || q.RetryAfter != nil {
		t.Fatalf("retry-after should be nil")
	}
}

func TestErrorStringIncludesParts(t *testing.T) {
	s := errorFromResponse(403, body("FORBIDDEN", "no access", "abc"), nil).Error()
	for _, p := range []string{"403", "FORBIDDEN", "no access", "abc"} {
		if !strings.Contains(s, p) {
			t.Fatalf("missing %q in %q", p, s)
		}
	}
}
