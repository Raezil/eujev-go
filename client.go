package eujev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://jev.bevel.software"
	DefaultModel   = "jeff-latest"
	DefaultTimeout = 60 * time.Second

	maxResponseBytes = 4 << 20
)

// Client calls the eu/jev API. Create it with NewClient; the zero value is not
// usable. A Client is safe for concurrent use when its transport is, provided
// callers do not mutate request maps or slices while a request is in flight.
type Client struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

// Option configures a Client during construction.
type Option func(*Client) error

// WithBaseURL sets the service root, without /v1/systemone. An optional path
// prefix is preserved, for example https://example.com/proxy. The URL must use
// HTTP or HTTPS and must not contain credentials, a query, or a fragment.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) error {
		u, err := url.Parse(baseURL)
		if err != nil {
			return fmt.Errorf("eujev: invalid base URL: %w", err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
			u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(baseURL, "#") {
			return errors.New("eujev: base URL must be an HTTP(S) URL without credentials, a query, or a fragment")
		}
		c.endpoint = strings.TrimRight(u.String(), "/") + "/v1/systemone"
		return nil
	}
}

// WithHTTPClient supplies a custom timeout, transport, or cookie jar. The client
// is shallow-copied. Redirects are always disabled by the SDK, overriding
// CheckRedirect in the copy, so credentials and POST bodies stay at the endpoint.
// Custom transports must be safe for concurrent use.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) error {
		if httpClient == nil {
			return errors.New("eujev: HTTP client must not be nil")
		}
		c.httpClient = httpClient
		return nil
	}
}

// NewClient creates an authenticated client. apiKey is the token itself, without
// the "Bearer " prefix. Leading and trailing whitespace is trimmed.
func NewClient(apiKey string, options ...Option) (*Client, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("eujev: API key must not be empty")
	}
	for _, r := range apiKey {
		if r <= ' ' || r >= 0x7f {
			return nil, errors.New("eujev: API key must be an ASCII token without whitespace or control characters")
		}
	}
	c := &Client{
		apiKey:     apiKey,
		endpoint:   DefaultBaseURL + "/v1/systemone",
		httpClient: &http.Client{Timeout: DefaultTimeout},
	}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("eujev: client option must not be nil")
		}
		if err := option(c); err != nil {
			return nil, err
		}
	}
	httpClient := *c.httpClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	c.httpClient = &httpClient
	return c, nil
}

// Decide submits one request to POST /v1/systemone. It does not automatically
// retry. Use ctx to cancel the request or impose a deadline. The request is not
// modified; an empty Model is replaced with DefaultModel in the outgoing JSON.
func (c *Client) Decide(ctx context.Context, request DecisionRequest) (*DecisionResponse, error) {
	if request.Model == "" {
		request.Model = DefaultModel
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("eujev: encode request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("eujev: create request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("User-Agent", "eujev-go")

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("eujev: send request: %w", err)
	}
	defer httpResponse.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes+1))
	truncated := len(body) > maxResponseBytes
	if truncated {
		body = body[:maxResponseBytes]
		readErr = errors.Join(readErr, ErrResponseTooLarge)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return nil, newAPIError(httpResponse, body, truncated, readErr)
	}
	if readErr != nil {
		return nil, fmt.Errorf("eujev: read response: %w", readErr)
	}
	var response *DecisionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("eujev: decode response: %w", err)
	}
	if response == nil {
		return nil, errors.New("eujev: expected a decision response, got null")
	}
	if response.Meta.RequestID == "" {
		response.Meta.RequestID = httpResponse.Header.Get("X-Request-ID")
	}
	return response, nil
}
