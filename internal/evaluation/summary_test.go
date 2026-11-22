package evaluation_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
	"github.com/Code-oclock/llm-error-classifier/internal/evaluation"
)

func validOutcome(cat entity.Category, sev entity.Severity, latency time.Duration) evaluation.Outcome {
	return evaluation.Outcome{
		Classification: entity.Classification{
			Category:        cat,
			Severity:        sev,
			Summary:         "Test summary",
			SuggestedAction: "Test action",
		},
		StatusCode: http.StatusOK,
		Latency:    latency,
	}
}

func TestSummarize_AccuracyAndWithinOne(t *testing.T) {
	results := []evaluation.Result{
		// 1: Exact match on category and severity.
		{
			Sample: evaluation.Sample{
				ID:       "s1",
				Category: entity.CategorySyntax,
				Severity: entity.SeverityLow,
			},
			Outcome: validOutcome(entity.CategorySyntax, entity.SeverityLow, 10*time.Millisecond),
		},
		// 2: Category match, severity off by 1 (medium vs low).
		{
			Sample: evaluation.Sample{
				ID:       "s2",
				Category: entity.CategoryRuntime,
				Severity: entity.SeverityMedium,
			},
			Outcome: validOutcome(entity.CategoryRuntime, entity.SeverityLow, 20*time.Millisecond),
		},
		// 3: Category match, severity off by 2 (critical vs low).
		{
			Sample: evaluation.Sample{
				ID:       "s3",
				Category: entity.CategoryDatabase,
				Severity: entity.SeverityCritical,
			},
			Outcome: validOutcome(entity.CategoryDatabase, entity.SeverityLow, 30*time.Millisecond),
		},
		// 4: Category mismatch, severity exact match (high vs high).
		{
			Sample: evaluation.Sample{
				ID:       "s4",
				Category: entity.CategoryNetwork,
				Severity: entity.SeverityHigh,
			},
			Outcome: validOutcome(entity.CategoryAuth, entity.SeverityHigh, 40*time.Millisecond),
		},
		// 5: Failed request (counts as wrong for everything).
		{
			Sample: evaluation.Sample{
				ID:       "s5",
				Category: entity.CategoryConfig,
				Severity: entity.SeverityLow,
			},
			Outcome: evaluation.Outcome{
				StatusCode: http.StatusInternalServerError,
				Err:        errors.New("backend error"),
			},
		},
	}

	summary := evaluation.Summarize(results)

	if summary.Total != 5 {
		t.Errorf("expected total 5, got %d", summary.Total)
	}
	if summary.Valid != 4 {
		t.Errorf("expected valid 4, got %d", summary.Valid)
	}
	if summary.Failed != 1 {
		t.Errorf("expected failed 1, got %d", summary.Failed)
	}

	const eps = 0.001
	if expected := 4.0 / 5.0; summary.ValidRate < expected-eps || summary.ValidRate > expected+eps {
		t.Errorf("expected valid rate %f, got %f", expected, summary.ValidRate)
	}

	// Category correct: s1, s2, s3 (3 out of 5).
	if expected := 3.0 / 5.0; summary.CategoryAccuracy < expected-eps || summary.CategoryAccuracy > expected+eps {
		t.Errorf("expected category accuracy %f, got %f", expected, summary.CategoryAccuracy)
	}

	// Severity exact correct: s1, s4 (2 out of 5).
	if expected := 2.0 / 5.0; summary.SeverityExactAccuracy < expected-eps || summary.SeverityExactAccuracy > expected+eps {
		t.Errorf("expected severity exact accuracy %f, got %f", expected, summary.SeverityExactAccuracy)
	}

	// Severity within one level: s1 (diff 0), s2 (diff 1), s4 (diff 0) -> 3 out of 5.
	if expected := 3.0 / 5.0; summary.SeverityWithinOneAccuracy < expected-eps || summary.SeverityWithinOneAccuracy > expected+eps {
		t.Errorf("expected severity within-one accuracy %f, got %f", expected, summary.SeverityWithinOneAccuracy)
	}

	// Recall for syntax: 1/1 = 1.0.
	if r, ok := summary.PerCategoryRecall[entity.CategorySyntax]; !ok || r < 1.0-eps {
		t.Errorf("expected recall 1.0 for syntax, got %f (ok=%v)", r, ok)
	}
	// Recall for network: 0/1 = 0.0.
	if r, ok := summary.PerCategoryRecall[entity.CategoryNetwork]; !ok || r > eps {
		t.Errorf("expected recall 0.0 for network, got %f (ok=%v)", r, ok)
	}

	// Mismatches: s2 (sev mismatch), s3 (sev mismatch), s4 (cat mismatch), s5 (error).
	if len(summary.Mismatches) != 4 {
		t.Errorf("expected 4 mismatches, got %d", len(summary.Mismatches))
	}
}

func TestSummarize_Percentiles(t *testing.T) {
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}

	var results []evaluation.Result
	for i, l := range latencies {
		results = append(results, evaluation.Result{
			Sample: evaluation.Sample{
				ID:       string(rune('a' + i)),
				Category: entity.CategorySyntax,
				Severity: entity.SeverityLow,
			},
			Outcome: validOutcome(entity.CategorySyntax, entity.SeverityLow, l),
		})
	}

	summary := evaluation.Summarize(results)

	if summary.LatencyMean != 55*time.Millisecond {
		t.Errorf("expected mean 55ms, got %v", summary.LatencyMean)
	}
	if summary.LatencyP50 != 50*time.Millisecond {
		t.Errorf("expected p50 50ms, got %v", summary.LatencyP50)
	}
	if summary.LatencyP95 != 100*time.Millisecond {
		t.Errorf("expected p95 100ms, got %v", summary.LatencyP95)
	}
	if summary.LatencyMax != 100*time.Millisecond {
		t.Errorf("expected max 100ms, got %v", summary.LatencyMax)
	}
}

func TestSummary_MarkdownAndJSON(t *testing.T) {
	results := []evaluation.Result{
		{
			Sample: evaluation.Sample{
				ID:       "err-1",
				Category: entity.CategorySyntax,
				Severity: entity.SeverityLow,
			},
			Outcome: validOutcome(entity.CategoryRuntime, entity.SeverityHigh, 15*time.Millisecond),
		},
	}

	summary := evaluation.Summarize(results)
	md := summary.Markdown("test-model")

	if !strings.Contains(md, "# Evaluation: test-model") {
		t.Errorf("expected markdown to contain header with model name, got: %s", md)
	}
	if !strings.Contains(md, "| Metric | Value |") {
		t.Errorf("expected markdown to contain headline table, got: %s", md)
	}
	if !strings.Contains(md, "| ID | Expected | Got |") {
		t.Errorf("expected markdown to contain mismatches table, got: %s", md)
	}
	if !strings.Contains(md, "err-1") {
		t.Errorf("expected markdown to mention mismatch err-1, got: %s", md)
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal summary to JSON: %v", err)
	}

	var parsed evaluation.Summary
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal summary JSON: %v", err)
	}

	if parsed.Total != 1 || parsed.Valid != 1 {
		t.Errorf("unexpected parsed summary: %+v", parsed)
	}
}
