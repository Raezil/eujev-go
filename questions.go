package eujev

import "encoding/json"

// QuestionType identifies the kind of question or answer.
type QuestionType string

const (
	QuestionTypeChoice QuestionType = "choice"
	QuestionTypeNoul   QuestionType = "noul"
	QuestionTypeScore  QuestionType = "score"
)

// Question is a ChoiceQuestion, NoulQuestion, or ScoreQuestion.
// Their JSON encoders supply the correct type discriminator automatically.
type Question interface {
	json.Marshaler
	isQuestion()
}

// ChoiceQuestion asks the model to select one of two to ten named options.
// Instructions and criterion descriptions may be strings, JSON objects, arrays,
// or nil. A nil description uses the option name.
type ChoiceQuestion struct {
	Instructions any            `json:"instructions"`
	Criteria     map[string]any `json:"criteria"`
}

func (ChoiceQuestion) isQuestion() {}

// MarshalJSON includes the choice discriminator.
func (q ChoiceQuestion) MarshalJSON() ([]byte, error) {
	type fields ChoiceQuestion
	return json.Marshal(struct {
		Type QuestionType `json:"type"`
		fields
	}{QuestionTypeChoice, fields(q)})
}

// NoulQuestion asks for the probability that a condition is true.
// Criteria optionally describes the "true" and "false" outcomes. Instructions
// and descriptions may be strings, JSON objects, arrays, or nil.
type NoulQuestion struct {
	Instructions any            `json:"instructions"`
	Criteria     map[string]any `json:"criteria,omitempty"`
}

func (NoulQuestion) isQuestion() {}

// MarshalJSON includes the noul discriminator.
func (q NoulQuestion) MarshalJSON() ([]byte, error) {
	type fields NoulQuestion
	return json.Marshal(struct {
		Type QuestionType `json:"type"`
		fields
	}{QuestionTypeNoul, fields(q)})
}

// ScoreQuestion asks for an expected position on an ordered scale.
// Criteria contains two to ten levels, from low to high. Instructions and level
// descriptions may be strings, JSON objects, arrays, or nil.
type ScoreQuestion struct {
	Instructions any   `json:"instructions"`
	Criteria     []any `json:"criteria"`
}

func (ScoreQuestion) isQuestion() {}

// MarshalJSON includes the score discriminator.
func (q ScoreQuestion) MarshalJSON() ([]byte, error) {
	type fields ScoreQuestion
	return json.Marshal(struct {
		Type QuestionType `json:"type"`
		fields
	}{QuestionTypeScore, fields(q)})
}
