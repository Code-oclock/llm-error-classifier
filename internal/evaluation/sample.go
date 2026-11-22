// Package evaluation provides dataset loading, client execution, and metrics calculation.
package evaluation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
)

type Sample struct {
	ID       string          `json:"id"`
	Error    string          `json:"error"`
	Category entity.Category `json:"category"`
	Severity entity.Severity `json:"severity"`
}

// Load parses samples from a JSONL reader with strict validation.
func Load(r io.Reader) ([]Sample, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	lineNum := 0
	seenIDs := make(map[string]struct{})
	var samples []Sample

	for scanner.Scan() {
		lineNum++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			return nil, fmt.Errorf("line %d: empty line", lineNum)
		}

		var s Sample
		if err := json.Unmarshal([]byte(text), &s); err != nil {
			return nil, fmt.Errorf("line %d: invalid json: %w", lineNum, err)
		}

		if s.ID == "" {
			return nil, fmt.Errorf("line %d: empty id", lineNum)
		}

		if _, exists := seenIDs[s.ID]; exists {
			return nil, fmt.Errorf("line %d: duplicate id %q", lineNum, s.ID)
		}
		seenIDs[s.ID] = struct{}{}

		if !s.Category.Valid() {
			return nil, fmt.Errorf("line %d: unknown category %q", lineNum, s.Category)
		}

		if !s.Severity.Valid() {
			return nil, fmt.Errorf("line %d: unknown severity %q", lineNum, s.Severity)
		}

		samples = append(samples, s)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
	}

	return samples, nil
}
