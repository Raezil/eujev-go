package eujev_test

import (
	"encoding/json"
	"testing"

	eujev "github.com/Raezil/eujev-go"
)

func TestQuestionWireFormats(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		question eujev.Question
		want     string
	}{
		{
			name: "choice with structured and null descriptions",
			question: eujev.ChoiceQuestion{
				Instructions: map[string]any{"task": "Route this message"},
				Criteria:     map[string]any{"billing": nil, "support": []string{"technical", "bugs"}},
			},
			want: `{"type":"choice","instructions":{"task":"Route this message"},"criteria":{"billing":null,"support":["technical","bugs"]}}`,
		},
		{
			name:     "noul with null instructions and omitted criteria",
			question: eujev.NoulQuestion{},
			want:     `{"type":"noul","instructions":null}`,
		},
		{
			name: "noul with criteria",
			question: &eujev.NoulQuestion{
				Instructions: "Does this require a refund?",
				Criteria:     map[string]any{"true": "Refund needed", "false": nil},
			},
			want: `{"type":"noul","instructions":"Does this require a refund?","criteria":{"true":"Refund needed","false":null}}`,
		},
		{
			name: "score preserves ordered structured levels",
			question: eujev.ScoreQuestion{
				Instructions: []string{"Rate the urgency"},
				Criteria:     []any{"low", map[string]any{"label": "high"}, nil},
			},
			want: `{"type":"score","instructions":["Rate the urgency"],"criteria":["low",{"label":"high"},null]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.question)
			if err != nil {
				t.Fatal(err)
			}
			assertJSON(t, data, tc.want)
		})
	}
}

func TestQuestionEncodingFailure(t *testing.T) {
	t.Parallel()
	for _, question := range []eujev.Question{
		eujev.ChoiceQuestion{Instructions: make(chan int)},
		eujev.NoulQuestion{Criteria: map[string]any{"true": func() {}}},
		eujev.ScoreQuestion{Criteria: []any{make(chan int)}},
	} {
		if _, err := json.Marshal(question); err == nil {
			t.Errorf("accepted unsupported JSON value in %T", question)
		}
	}
}
