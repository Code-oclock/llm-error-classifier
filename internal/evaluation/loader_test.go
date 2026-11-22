package evaluation_test

import (
	"strings"
	"testing"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
	"github.com/Code-oclock/llm-error-classifier/internal/evaluation"
)

func TestLoad_Valid(t *testing.T) {
	data := `{"id":"s1","error":"err 1","category":"syntax_error","severity":"low"}
{"id":"s2","error":"err 2","category":"network_error","severity":"high"}
`
	samples, err := evaluation.Load(strings.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}

	if samples[0].ID != "s1" || samples[0].Category != entity.CategorySyntax || samples[0].Severity != entity.SeverityLow {
		t.Errorf("unexpected sample 0: %+v", samples[0])
	}
	if samples[1].ID != "s2" || samples[1].Category != entity.CategoryNetwork || samples[1].Severity != entity.SeverityHigh {
		t.Errorf("unexpected sample 1: %+v", samples[1])
	}
}

func TestLoad_BadCategory(t *testing.T) {
	data := `{"id":"s1","error":"err 1","category":"invalid_category","severity":"low"}`
	_, err := evaluation.Load(strings.NewReader(data))
	if err == nil {
		t.Fatal("expected error for bad category, got nil")
	}

	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("expected error to mention line 1, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown category") {
		t.Errorf("expected error to mention unknown category, got: %v", err)
	}
}

func TestLoad_BadSeverity(t *testing.T) {
	data := `{"id":"s1","error":"err 1","category":"syntax_error","severity":"invalid_sev"}`
	_, err := evaluation.Load(strings.NewReader(data))
	if err == nil {
		t.Fatal("expected error for bad severity, got nil")
	}

	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("expected error to mention line 1, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown severity") {
		t.Errorf("expected error to mention unknown severity, got: %v", err)
	}
}

func TestLoad_DuplicateID(t *testing.T) {
	data := `{"id":"dup-id","error":"err 1","category":"syntax_error","severity":"low"}
{"id":"dup-id","error":"err 2","category":"network_error","severity":"high"}
`
	_, err := evaluation.Load(strings.NewReader(data))
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}

	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("expected error to mention line 2, got: %v", err)
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("expected error to mention duplicate id, got: %v", err)
	}
}

func TestLoad_BadJSONLineNumber(t *testing.T) {
	data := `{"id":"s1","error":"err 1","category":"syntax_error","severity":"low"}
{"id":"s2", broken json here
`
	_, err := evaluation.Load(strings.NewReader(data))
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}

	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("expected error to mention line 2, got: %v", err)
	}
}
