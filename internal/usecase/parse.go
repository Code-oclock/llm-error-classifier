package usecase

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
)

const maxFieldRunes = 500

// ParseClassification parses and normalises model output into a Classification.
func ParseClassification(content string) (entity.Classification, error) {
	return parseClassification(content)
}

// parseClassification parses model output into an entity.Classification.
func parseClassification(content string) (entity.Classification, error) {
	jsonStr, err := findBalancedJSON(content)
	if err != nil {
		return entity.Classification{}, err
	}

	var c entity.Classification
	if err := json.Unmarshal([]byte(jsonStr), &c); err != nil {
		return entity.Classification{}, fmt.Errorf("%w: %w", entity.ErrInvalidModelOutput, err)
	}

	cat := strings.ToLower(strings.TrimSpace(string(c.Category)))
	cat = strings.ReplaceAll(cat, "-", "_")
	cat = strings.ReplaceAll(cat, " ", "_")
	for strings.Contains(cat, "__") {
		cat = strings.ReplaceAll(cat, "__", "_")
	}

	c.Category = entity.Category(cat)

	sev := strings.ToLower(strings.TrimSpace(string(c.Severity)))
	c.Severity = entity.Severity(sev)

	c.Summary = cutRunes(strings.TrimSpace(c.Summary), maxFieldRunes)
	c.SuggestedAction = cutRunes(strings.TrimSpace(c.SuggestedAction), maxFieldRunes)

	if err := c.Validate(); err != nil {
		return entity.Classification{}, err
	}

	return c, nil
}

// findBalancedJSON locates the first balanced top-level JSON object.
func findBalancedJSON(s string) (string, error) {
	inString := false
	escaped := false
	depth := 0
	start := -1

	for i := 0; i < len(s); i++ {
		b := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}

			if b == '\\' {
				escaped = true
				continue
			}

			if b == '"' {
				inString = false
			}

			continue
		}

		switch b {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}

			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					return s[start : i+1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("%w: no balanced JSON object found", entity.ErrInvalidModelOutput)
}

// cutRunes cuts s to limit runes.
func cutRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) > limit {
		return string(runes[:limit])
	}

	return s
}
