package eujev

// DecisionRequest is the payload for POST /v1/systemone.
// The service validates question counts, lengths, and other input constraints.
type DecisionRequest struct {
	// Model defaults to DefaultModel when empty in a call to Decide.
	Model string `json:"model,omitempty"`
	// State is a nonempty string, JSON object, or array. Structs, maps, slices,
	// and json.RawMessage values can be used for structured state.
	State any `json:"state"`
	// Questions contains one to eight named questions.
	Questions map[string]Question `json:"questions"`
}

// DecisionResponse contains the model's answers and request accounting.
type DecisionResponse struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
	Meta    Metadata          `json:"meta"`
}

// Answer holds one of the three answer shapes. Check Type before reading the
// corresponding fields. Numeric pointers distinguish an absent field from zero.
type Answer struct {
	Type          QuestionType       `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}

// Usage reports the tokens counted by the service.
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// Metadata describes the server-side processing of a request.
type Metadata struct {
	RequestID string `json:"request_id"`
	Mode      string `json:"mode"`
	LatencyMS int64  `json:"latency_ms"`
	// CostEUR preserves the exact decimal charge without floating-point loss.
	CostEUR string `json:"cost_eur,omitempty"`
}
