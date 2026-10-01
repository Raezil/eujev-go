# eujev-go

A Go SDK for the [eu/jev System One API](https://jev.bevel.software/docs),
using only the standard library. Supports choice, noul (yes/no probability),
and score questions, with typed responses and API errors.

Requires Go 1.22 or later. This workspace uses the local module path `eujev-go`
and package name `eujev`. Before publishing, change the module path in `go.mod`
and the `eujev-go` imports to your repository's public import path.

## Quick start

This sends the same request as the supplied curl example:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    eujev "eujev-go"
)

func main() {
    client, err := eujev.NewClient(os.Getenv("EU_JEV_API_KEY"))
    if err != nil {
        log.Fatal(err)
    }

    response, err := client.Decide(context.Background(), eujev.DecisionRequest{
        Model: eujev.DefaultModel, // jeff-latest; also used when Model is empty
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
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.Answers["team"].Choice)
    fmt.Println(response.Meta.RequestID, response.Meta.CostEUR)
}
```

Run the included example, which prints the full response:

```sh
export EU_JEV_API_KEY='your-api-key'
go run ./examples/refund
```

The SDK takes the key explicitly; the example reads it from the environment.
Running the example makes a live request subject to the service's billing.

To use this unpublished checkout in another local module:

```sh
go mod edit -require=eujev-go@v0.0.0
go mod edit -replace=eujev-go=/absolute/path/to/eujev-go
```

Then import `eujev "eujev-go"` and run `go mod tidy`.

## Other question types

```go
request := eujev.DecisionRequest{
    State: map[string]any{
        "message": "I was charged twice. Can I get a refund?",
        "customer_tier": "premium",
    },
    Questions: map[string]eujev.Question{
        "refund": eujev.NoulQuestion{
            Instructions: "Does this require a refund?",
            Criteria: map[string]any{ // optional for noul
                "true":  "A duplicate or incorrect charge",
                "false": "A valid charge",
            },
        },
        "urgency": eujev.ScoreQuestion{
            Instructions: "How urgently should the team respond?",
            Criteria: []any{"Routine", "Soon", "Immediately"},
        },
    },
}
response, err := client.Decide(ctx, request)
```

Question types add their JSON `type` field automatically. Instructions and
criterion descriptions accept strings, objects, arrays, and `nil`, matching the
API schema. State accepts a string, object, or array, including Go structs and
`json.RawMessage` values.

Answers are keyed by question name. Check `Answer.Type` to identify the shape:

| Type | Fields |
| --- | --- |
| `QuestionTypeChoice` | `Choice`, `Confidence`, `Probabilities` |
| `QuestionTypeNoul` | `Noul` (probability from 0 to 1) |
| `QuestionTypeScore` | `Score` (expected position from 0 to N−1), `Confidence`, `Probabilities`, `Legend` |

`Noul`, `Score`, and `Confidence` are `*float64`: nil means absent, and a pointer
to zero is a valid result. `Meta.CostEUR` remains a decimal string to preserve
the exact amount.

The service validates input constraints. Its schema currently specifies 1–8
questions, 2–10 choice options or score levels, and a 256 KiB request body limit.
See the [OpenAPI schema](https://jev.bevel.software/openapi.json) for the full
contract; this SDK was implemented against the schema retrieved on 2026-10-01.

## HTTP configuration and cancellation

```go
client, err := eujev.NewClient(apiKey,
    eujev.WithBaseURL("https://jev.bevel.software"), // service root, no /v1
    eujev.WithHTTPClient(&http.Client{Timeout: 20 * time.Second}),
)
// Handle err before using client.

ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Second)
defer cancel()
response, err := client.Decide(ctx, request)
```

The default HTTP timeout is 60 seconds. A custom HTTP client's timeout replaces
that default, including zero (no client timeout). Base URLs can include a proxy
path prefix. The SDK shallow-copies the supplied HTTP client and disables
redirects in the copy; redirect responses become `*APIError` values.

Clients can be shared across goroutines. Custom transports must support
concurrent use, and request maps and slices must not be mutated during a call.
Each response body is closed and limited to 4 MiB. Oversized responses expose
`ErrResponseTooLarge` through `errors.Is`.

## Errors

```go
response, err := client.Decide(ctx, request)
if err != nil {
    var apiErr *eujev.APIError
    switch {
    case errors.As(err, &apiErr):
        log.Printf("status=%d code=%s request_id=%s retry_after=%s: %s",
            apiErr.StatusCode, apiErr.Code, apiErr.RequestID,
            apiErr.RetryAfter, apiErr.Message)
    case errors.Is(err, context.Canceled):
        log.Print("request canceled")
    case errors.Is(err, context.DeadlineExceeded):
        log.Print("request timed out")
    default:
        log.Printf("request failed: %v", err)
    }
    return
}
```

`APIError` also exposes `ContactURL`, response `Header`, raw `Body`, and
`BodyTruncated`. Non-JSON errors retain their HTTP status and body. Successful
responses use `X-Request-ID` as a fallback when `meta.request_id` is missing.
Transport and response read errors preserve their underlying causes.

There are no automatic retries. The API documents `Retry-After` in seconds for
rate limiting; callers can use it to schedule retries for 429 or 503 responses.

## Development

```sh
go test -race ./...
go vet ./...
```

Tests use local HTTP servers and in-memory transports. They require no API key
and make no requests to the live service.
