package evaluation_test

import (
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
	"github.com/Code-oclock/llm-error-classifier/internal/evaluation"
)

func TestDataset_Real(t *testing.T) {
	path := "../../eval/dataset.jsonl"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open dataset at %s: %v", path, err)
	}
	defer f.Close()

	samples, err := evaluation.Load(f)
	if err != nil {
		t.Fatalf("failed to load dataset: %v", err)
	}

	if len(samples) < 40 {
		t.Errorf("expected at least 40 samples, got %d", len(samples))
	}

	categoryCounts := make(map[entity.Category]int)
	for _, s := range samples {
		categoryCounts[s.Category]++
	}

	for _, d := range entity.Categories() {
		count := categoryCounts[d.Value]
		if d.Value == entity.CategoryUnknown {
			if count < 2 {
				t.Errorf("expected at least 2 samples for category %s, got %d", d.Value, count)
			}
		} else {
			if count < 3 {
				t.Errorf("expected at least 3 samples for category %s, got %d", d.Value, count)
			}
		}
	}

	// Verify at least 5 multi-line stack traces (10-20 lines).
	stackTraceCount := 0
	for _, s := range samples {
		lines := strings.Split(s.Error, "\n")
		if len(lines) >= 10 && len(lines) <= 20 {
			stackTraceCount++
		}
	}
	if stackTraceCount < 5 {
		t.Errorf("expected at least 5 examples with 10-20 lines stack traces, got %d", stackTraceCount)
	}

	// Verify at least 3 examples with non-English text (Russian, Chinese).
	nonEnglishCount := 0
	for _, s := range samples {
		hasCyrillic := false
		hasHan := false
		for _, r := range s.Error {
			if unicode.Is(unicode.Cyrillic, r) {
				hasCyrillic = true
			}
			if unicode.Is(unicode.Han, r) {
				hasHan = true
			}
		}
		if hasCyrillic || hasHan {
			nonEnglishCount++
		}
	}
	if nonEnglishCount < 3 {
		t.Errorf("expected at least 3 samples with non-English text, got %d", nonEnglishCount)
	}
}
