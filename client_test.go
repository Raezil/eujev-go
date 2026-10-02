package eujev_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eujev "github.com/Bevel/eujev-go"
)

// This fixture is the successful response example from the public OpenAPI schema.
const choiceResponse = `{
	"model":"jeff-1.0.0",
	"answers":{"team":{"type":"choice","choice":"billing","confidence":0.9,"probabilities":{"billing":0.95,"support":0.05}}},
	"usage":{"input_tokens":64,"output_tokens":0},
	"meta":{"request_id":"8febf2e9-1643-4446-a7cf-7fc3bcf2b391","mode":"live","latency_ms":180,"cost_eur":"0.000002368"}
}`

func refundRequest() eujev.DecisionRequest {
	return eujev.DecisionRequest{
		State: "I was charged twice. Can I get a refund?",
		Questions: map[string]eujev.Question{
			"team": eujev.ChoiceQuestion{
				Instructions: "Which team should handle this?",
				Criteria: map[string]any{
					"billing": "Payments, invoices, and refunds",
					"support": "Technical issues and bugs",
				},
			},
		},
	}
}

func newClient(t *testing.T, options ...eujev.Option) *eujev.Client {
	t.Helper()
	client, err := eujev.NewClient("test-api-key", options...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func assertJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Errorf("invalid JSON %q: %v", got, err)
		return
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Errorf("invalid fixture JSON: %v", err)
		return
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("JSON mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

func TestDecideMatchesCurl(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		for name, want := range map[string]string{
			"Authorization": "Bearer test-api-key",
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"User-Agent":    "eujev-go",
		} {
			if got := r.Header.Get(name); got != want {
				t.Errorf("%s = %q, want %q", name, got, want)
			}
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		assertJSON(t, body, `{
			"model":"jeff-latest",
			"state":"I was charged twice. Can I get a refund?",
			"questions":{"team":{"type":"choice","instructions":"Which team should handle this?","criteria":{"billing":"Payments, invoices, and refunds","support":"Technical issues and bugs"}}}
		}`)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "header-id")
		fmt.Fprint(w, choiceResponse)
	}))
	t.Cleanup(server.Close)
	request := refundRequest()
	response, err := newClient(t, eujev.WithBaseURL(server.URL+"/")).Decide(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if request.Model != "" {
		t.Fatal("Decide modified the request model")
	}
	answer := response.Answers["team"]
	if answer.Type != eujev.QuestionTypeChoice || answer.Choice != "billing" ||
		answer.Confidence == nil || *answer.Confidence != 0.9 || answer.Probabilities["billing"] != 0.95 {
		t.Errorf("unexpected choice answer: %+v", answer)
	}
	if answer.Noul != nil || answer.Score != nil {
		t.Error("absent numeric fields should remain nil")
	}
	if response.Model != "jeff-1.0.0" || response.Usage.InputTokens != 64 || response.Usage.OutputTokens != 0 ||
		response.Meta.CostEUR != "0.000002368" || response.Meta.Mode != "live" || response.Meta.LatencyMS != 180 ||
		response.Meta.RequestID != "8febf2e9-1643-4446-a7cf-7fc3bcf2b391" {
		t.Errorf("unexpected response: %+v", response)
	}
}

func TestDecideStructuredStateAndOtherAnswers(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/v1/systemone" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		assertJSON(t, body, `{
			"model":"jeff-1.0.0","state":{"message":"Hello","history":[]},
			"questions":{
				"urgent":{"type":"noul","instructions":"Is it urgent?"},
				"severity":{"type":"score","instructions":"How severe?","criteria":["low","medium","high"]}
			}
		}`)
		w.Header().Set("X-Request-ID", "fallback-id")
		fmt.Fprint(w, `{
			"model":"jeff-1.0.0",
			"answers":{
				"urgent":{"type":"noul","noul":0},
				"severity":{"type":"score","score":0,"confidence":0,"probabilities":{"0":1,"1":0,"2":0},"legend":{"0":"low","1":"medium","2":"high"}}
			},
			"usage":{"input_tokens":30,"output_tokens":0},"meta":{"mode":"live","latency_ms":10}
		}`)
	}))
	t.Cleanup(server.Close)
	response, err := newClient(t, eujev.WithBaseURL(server.URL+"/proxy///")).Decide(context.Background(), eujev.DecisionRequest{
		Model: "jeff-1.0.0",
		State: map[string]any{"message": "Hello", "history": []string{}},
		Questions: map[string]eujev.Question{
			"urgent": eujev.NoulQuestion{Instructions: "Is it urgent?"},
			"severity": eujev.ScoreQuestion{
				Instructions: "How severe?", Criteria: []any{"low", "medium", "high"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	noul, score := response.Answers["urgent"], response.Answers["severity"]
	if noul.Type != eujev.QuestionTypeNoul || noul.Noul == nil || *noul.Noul != 0 || noul.Score != nil {
		t.Errorf("unexpected noul: %+v", noul)
	}
	if score.Type != eujev.QuestionTypeScore || score.Score == nil || *score.Score != 0 ||
		score.Confidence == nil || *score.Confidence != 0 || score.Legend["2"] != "high" || score.Probabilities["0"] != 1 {
		t.Errorf("unexpected score: %+v", score)
	}
	if response.Meta.RequestID != "fallback-id" {
		t.Errorf("request ID = %q", response.Meta.RequestID)
	}
	encoded, err := json.Marshal(noul)
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, encoded, `{"type":"noul","noul":0}`)
}

func TestAPIErrors(t *testing.T) {
	t.Parallel()
	for _, status := range []int{400, 401, 402, 413, 415, 422, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			const body = `{"error":"Request rejected","code":"example_code","contact_url":"https://example.com/contact"}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("X-Request-ID", "error-id")
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(status)
				fmt.Fprint(w, body)
			}))
			t.Cleanup(server.Close)
			response, err := newClient(t, eujev.WithBaseURL(server.URL)).Decide(context.Background(), refundRequest())
			var apiErr *eujev.APIError
			if response != nil || !errors.As(err, &apiErr) {
				t.Fatalf("response = %+v, error = %v; want APIError", response, err)
			}
			if apiErr.StatusCode != status || apiErr.Message != "Request rejected" || apiErr.Code != "example_code" ||
				apiErr.ContactURL != "https://example.com/contact" || apiErr.RequestID != "error-id" || apiErr.RetryAfter != "30" ||
				apiErr.Header.Get("Retry-After") != "30" || apiErr.Body != body || apiErr.BodyTruncated || apiErr.Unwrap() != nil {
				t.Errorf("unexpected APIError: %+v", apiErr)
			}
			if !strings.Contains(err.Error(), "example_code") || !strings.Contains(err.Error(), "error-id") {
				t.Errorf("error lacks diagnostic context: %v", err)
			}
			if calls.Load() != 1 {
				t.Errorf("made %d requests, want exactly 1", calls.Load())
			}
		})
	}
}

func TestNonJSONAPIErrors(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"<html>Bad gateway</html>", "", `{"error":123}`, `{"error":"partial"`, "null"} {
		t.Run(body, func(t *testing.T) {
			client := newClient(t, eujev.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 502, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}))
			_, err := client.Decide(context.Background(), refundRequest())
			var apiErr *eujev.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != 502 || apiErr.Message != "Bad Gateway" || apiErr.Body != body {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestInvalidSuccessResponses(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", "null", "[]", `{"model":42}`, "not JSON", choiceResponse + `{}`} {
		t.Run(body, func(t *testing.T) {
			client := newClient(t, eujev.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}))
			if response, err := client.Decide(context.Background(), refundRequest()); err == nil || response != nil {
				t.Fatalf("response = %+v, error = %v; want a decode error", response, err)
			}
		})
	}
}

func TestDefaultEndpointAndClientCopy(t *testing.T) {
	t.Parallel()
	custom := &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://jev.bevel.software/v1/systemone" {
			t.Errorf("URL = %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(choiceResponse))}, nil
	})}
	client := newClient(t, eujev.WithHTTPClient(custom))
	if custom.CheckRedirect != nil {
		t.Fatal("SDK changed caller's HTTP client")
	}
	custom.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("the SDK should hold a copy")
	})
	if _, err := client.Decide(context.Background(), refundRequest()); err != nil {
		t.Fatal(err)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	t.Parallel()
	var redirected atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			redirected.Store(true)
			fmt.Fprint(w, choiceResponse)
			return
		}
		http.Redirect(w, r, "/other", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	custom := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		t.Error("custom redirect hook should be overridden in the SDK's copy")
		return nil
	}}
	_, err := newClient(t, eujev.WithBaseURL(server.URL), eujev.WithHTTPClient(custom)).Decide(context.Background(), refundRequest())
	var apiErr *eujev.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 307 || redirected.Load() {
		t.Fatalf("redirect followed or not surfaced: %v, followed=%v", err, redirected.Load())
	}
}

func TestClientConfigurationErrors(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"", "  ", "Bearer key", "a\r\nb", "a\x7fb", "a\x01b", "klucz-ą"} {
		if _, err := eujev.NewClient(key); err == nil {
			t.Errorf("accepted invalid API key %q", key)
		}
	}
	for _, baseURL := range []string{"", "not-a-url", "//example.com", "ftp://example.com", "https://", "https://a:b@example.com", "https://example.com?q=1", "https://example.com?", "https://example.com#fragment", "https://example.com#", "http://[::1"} {
		if _, err := eujev.NewClient("key", eujev.WithBaseURL(baseURL)); err == nil {
			t.Errorf("accepted invalid base URL %q", baseURL)
		}
	}
	for _, option := range []eujev.Option{nil, eujev.WithHTTPClient(nil)} {
		if _, err := eujev.NewClient("key", option); err == nil {
			t.Error("accepted nil configuration")
		}
	}
	if _, err := eujev.NewClient(" key\n"); err != nil {
		t.Errorf("did not trim API key whitespace: %v", err)
	}
}

func TestEncodingAndContextErrors(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := newClient(t, eujev.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected network call")
	})}))
	request := refundRequest()
	request.State = make(chan int)
	_, err := client.Decide(context.Background(), request)
	var unsupported *json.UnsupportedTypeError
	if !errors.As(err, &unsupported) {
		t.Errorf("error = %v, want UnsupportedTypeError", err)
	}
	if _, err := client.Decide(nil, refundRequest()); err == nil {
		t.Error("accepted nil context")
	}
	if calls.Load() != 0 {
		t.Error("invalid requests reached the network")
	}
}

func TestCancellationAndTimeout(t *testing.T) {
	t.Parallel()
	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		client := newClient(t, eujev.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			close(started)
			<-r.Context().Done()
			return nil, r.Context().Err()
		})}))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := client.Decide(ctx, refundRequest())
			done <- err
		}()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("request did not start")
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("cancellation cause lost: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("request did not cancel")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		client := newClient(t, eujev.WithHTTPClient(&http.Client{
			Timeout: 10 * time.Millisecond,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}),
		}))
		_, err := client.Decide(context.Background(), refundRequest())
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("timeout cause lost: %v", err)
		}
	})
}

func TestResponseBodyLimitsAndClosure(t *testing.T) {
	t.Parallel()
	readFailure := errors.New("broken response stream")
	for _, tc := range []struct {
		name      string
		status    int
		reader    io.Reader
		wantErr   error
		wantAPI   bool
		truncated bool
	}{
		{"success", 200, strings.NewReader(choiceResponse), nil, false, false},
		{"API error", 500, strings.NewReader(`{"error":"failed"}`), nil, true, false},
		{"decode error", 200, strings.NewReader("invalid"), nil, false, false},
		{"read error", 200, errorReader{readFailure}, readFailure, false, false},
		{"API read error", 503, errorReader{readFailure}, readFailure, true, false},
		{"large success", 200, strings.NewReader(strings.Repeat("x", (4<<20)+1)), eujev.ErrResponseTooLarge, false, true},
		{"large error", 502, strings.NewReader(strings.Repeat("x", (4<<20)+1)), eujev.ErrResponseTooLarge, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackingBody{Reader: tc.reader}
			client := newClient(t, eujev.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: body}, nil
			})}))
			_, err := client.Decide(context.Background(), refundRequest())
			if !body.closed {
				t.Error("response body was not closed")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want cause %v", err, tc.wantErr)
			}
			var apiErr *eujev.APIError
			if errors.As(err, &apiErr) != tc.wantAPI {
				t.Fatalf("error = %v, want APIError=%v", err, tc.wantAPI)
			}
			if apiErr != nil && (apiErr.StatusCode != tc.status || apiErr.BodyTruncated != tc.truncated || len(apiErr.Body) > 4<<20) {
				t.Errorf("API error lost status or body limit: %+v", apiErr)
			}
			if tc.name == "success" && err != nil {
				t.Fatal(err)
			}
			if tc.name != "success" && err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestConcurrentDecisions(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, choiceResponse)
	}))
	t.Cleanup(server.Close)
	client := newClient(t, eujev.WithBaseURL(server.URL))
	request := refundRequest()
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			response, err := client.Decide(context.Background(), request)
			if err != nil {
				t.Error(err)
				return
			}
			if response.Answers["team"].Choice != "billing" {
				t.Error("unexpected answer")
			}
		}()
	}
	workers.Wait()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}
