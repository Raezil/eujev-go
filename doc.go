// Package eujev provides a client for the eu/jev System One API.
//
// Create a Client with NewClient and call Decide with a context and a
// DecisionRequest. Questions can be ChoiceQuestion, NoulQuestion, or
// ScoreQuestion values. A Client can be shared by concurrent goroutines.
//
// The client uses only the Go standard library. API failures are returned as
// *APIError; transport errors preserve their cause for errors.Is and errors.As.
package eujev
