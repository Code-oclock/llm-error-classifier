package evaluation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
	"github.com/Code-oclock/llm-error-classifier/internal/evaluation"
)

func TestRun_HTTPTestServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/classify" || r.Method != http.MethodPost {
			http.NotFound(w, r)

			return
		}

		var req struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		switch req.Error {
		case "trigger-fail":
			http.Error(w, "internal server error", http.StatusInternalServerError)
		case "trigger-invalid-output":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"category":"bad_category","severity":"low","summary":"s","suggested_action":"a"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			resp := entity.Classification{
				Category:        entity.CategorySyntax,
				Severity:        entity.SeverityLow,
				Summary:         "Syntax error in code",
				SuggestedAction: "Check parentheses",
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer ts.Close()

	client := evaluation.NewClient(ts.URL, 5*time.Second)

	samples := []evaluation.Sample{
		{ID: "s1", Error: "trigger-ok", Category: entity.CategorySyntax, Severity: entity.SeverityLow},
		{ID: "s2", Error: "trigger-fail", Category: entity.CategoryNetwork, Severity: entity.SeverityHigh},
		{ID: "s3", Error: "trigger-invalid-output", Category: entity.CategoryDatabase, Severity: entity.SeverityMedium},
	}

	ctx := context.Background()
	results := evaluation.Run(ctx, samples, client.Classify, 2)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Result 0: successful 200.
	if results[0].Sample.ID != "s1" {
		t.Errorf("expected result 0 to have id s1, got %s", results[0].Sample.ID)
	}
	if results[0].Outcome.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", results[0].Outcome.StatusCode)
	}
	if results[0].Outcome.Err != nil {
		t.Errorf("unexpected error on s1: %v", results[0].Outcome.Err)
	}
	if results[0].Outcome.Classification.Category != entity.CategorySyntax {
		t.Errorf("expected category syntax_error, got %s", results[0].Outcome.Classification.Category)
	}

	// Result 1: failing 500.
	if results[1].Sample.ID != "s2" {
		t.Errorf("expected result 1 to have id s2, got %s", results[1].Sample.ID)
	}
	if results[1].Outcome.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", results[1].Outcome.StatusCode)
	}
	if results[1].Outcome.Err == nil {
		t.Error("expected error on s2, got nil")
	}

	// Result 2: invalid classification output.
	if results[2].Sample.ID != "s3" {
		t.Errorf("expected result 2 to have id s3, got %s", results[2].Sample.ID)
	}
	if results[2].Outcome.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for s3, got %d", results[2].Outcome.StatusCode)
	}
	if results[2].Outcome.Err == nil {
		t.Error("expected validation error on s3, got nil")
	}
}
