package redpine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// APIError is the base for every error the SDK returns on a non-2xx response.
// (Named APIError because the generated code already declares the model type Error.)
type APIError struct {
	Status    int
	Code      string
	Message   string
	RequestID string
	Body      map[string]any
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "HTTP %d", e.Status)
	if e.Code != "" {
		b.WriteString(" " + e.Code)
	}
	b.WriteString(" " + e.Message)
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (request %s)", e.RequestID)
	}
	return b.String()
}

// AuthError is 401: missing or invalid API key.
type AuthError struct{ APIError }

// InsufficientCreditsError is 402: billing suspended, or not enough credits to unlock the requested rows.
type InsufficientCreditsError struct{ APIError }

// AccessDeniedError is 403: key has no access to the requested collection.
type AccessDeniedError struct{ APIError }

// NotFoundError is 404: collection or queryId not found.
type NotFoundError struct{ APIError }

// ExpiredError is 410: the cached result or preview is past its 7-day window. Run the search again.
type ExpiredError struct{ APIError }

// ValidationError is 422: request rejected (query too long, bad filter, ...).
type ValidationError struct{ APIError }

// QuotaExceededError is 429. RetryAfter is seconds when the server said.
type QuotaExceededError struct {
	APIError
	RetryAfter *float64
}

// AssistedUnavailableError is 503: assisted search temporarily unavailable.
type AssistedUnavailableError struct{ APIError }

func (e *AuthError) Unwrap() error                { return &e.APIError }
func (e *InsufficientCreditsError) Unwrap() error { return &e.APIError }
func (e *AccessDeniedError) Unwrap() error        { return &e.APIError }
func (e *NotFoundError) Unwrap() error            { return &e.APIError }
func (e *ExpiredError) Unwrap() error             { return &e.APIError }
func (e *ValidationError) Unwrap() error          { return &e.APIError }
func (e *QuotaExceededError) Unwrap() error       { return &e.APIError }
func (e *AssistedUnavailableError) Unwrap() error { return &e.APIError }

func parseRetryAfter(h http.Header) *float64 {
	if h == nil {
		return nil
	}
	v := h.Get("Retry-After")
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &f
}

func errorFromResponse(status int, body []byte, header http.Header) error {
	base := APIError{Status: status, Message: fmt.Sprintf("request failed with status %d", status)}
	var parsed map[string]any
	if len(body) > 0 && json.Unmarshal(body, &parsed) == nil {
		base.Body = parsed
		if inner, ok := parsed["error"].(map[string]any); ok {
			if s, ok := inner["code"].(string); ok {
				base.Code = s
			}
			if s, ok := inner["message"].(string); ok && s != "" {
				base.Message = s
			}
			if s, ok := inner["requestId"].(string); ok {
				base.RequestID = s
			}
		}
	}
	switch status {
	case 401:
		return &AuthError{base}
	case 402:
		return &InsufficientCreditsError{base}
	case 403:
		return &AccessDeniedError{base}
	case 404:
		return &NotFoundError{base}
	case 410:
		return &ExpiredError{base}
	case 422:
		return &ValidationError{base}
	case 429:
		return &QuotaExceededError{APIError: base, RetryAfter: parseRetryAfter(header)}
	case 503:
		return &AssistedUnavailableError{base}
	default:
		return &base
	}
}
