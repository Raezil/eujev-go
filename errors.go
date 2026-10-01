package eujev

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrResponseTooLarge indicates that a response exceeded the 4 MiB read limit.
var ErrResponseTooLarge = errors.New("eujev: response body exceeds 4 MiB")

// APIError is returned for every non-2xx HTTP response, including redirects.
// Use errors.As to inspect it. Headers and Body are retained even when the
// response is not JSON; Body is limited to 4 MiB.
type APIError struct {
	StatusCode    int
	Message       string
	Code          string
	ContactURL    string
	RequestID     string
	RetryAfter    string
	Header        http.Header
	Body          string
	BodyTruncated bool
	cause         error
}

func (e *APIError) Error() string {
	message := fmt.Sprintf("eujev: HTTP %d", e.StatusCode)
	if e.Code != "" {
		message += " (" + e.Code + ")"
	}
	if e.Message != "" {
		message += ": " + e.Message
	}
	if e.RequestID != "" {
		message += " [request_id=" + e.RequestID + "]"
	}
	return message
}

// Unwrap exposes a response body read error, if any.
func (e *APIError) Unwrap() error { return e.cause }

func newAPIError(response *http.Response, body []byte, truncated bool, cause error) *APIError {
	var payload struct {
		Error      string `json:"error"`
		Code       string `json:"code"`
		ContactURL string `json:"contact_url"`
	}
	// A proxy may return HTML or plain text instead of the API error schema.
	if err := json.Unmarshal(body, &payload); err != nil {
		payload.Error = ""
		payload.Code = ""
		payload.ContactURL = ""
	}
	if payload.Error == "" {
		payload.Error = http.StatusText(response.StatusCode)
	}
	return &APIError{
		StatusCode:    response.StatusCode,
		Message:       payload.Error,
		Code:          payload.Code,
		ContactURL:    payload.ContactURL,
		RequestID:     response.Header.Get("X-Request-ID"),
		RetryAfter:    response.Header.Get("Retry-After"),
		Header:        response.Header.Clone(),
		Body:          string(body),
		BodyTruncated: truncated,
		cause:         cause,
	}
}
