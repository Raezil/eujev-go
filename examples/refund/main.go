package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	eujev "github.com/Raezil/go-eujev"
)

func main() {
	client, err := eujev.NewClient(os.Getenv("EU_JEV_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := client.Decide(ctx, eujev.DecisionRequest{
		Model: eujev.DefaultModel,
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
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		log.Fatal(err)
	}
}
