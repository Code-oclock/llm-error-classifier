package usecase_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
	"github.com/Code-oclock/llm-error-classifier/internal/usecase"
)

func TestParseClassification(t *testing.T) {
	t.Parallel()

	longSummary := strings.Repeat("s", 600)
	longAction := strings.Repeat("a", 600)

	tests := []struct {
		name    string
		input   string
		want    entity.Classification
		wantErr bool
	}{
		{
			name:  "plain JSON",
			input: `{"category": "runtime_error", "severity": "high", "summary": "NPE in main", "suggested_action": "Add null check"}`,
			want: entity.Classification{
				Category:        entity.CategoryRuntime,
				Severity:        entity.SeverityHigh,
				Summary:         "NPE in main",
				SuggestedAction: "Add null check",
			},
		},
		{
			name:  "fenced JSON",
			input: "```json\n" + `{"category": "network_error", "severity": "medium", "summary": "DNS timeout", "suggested_action": "Check DNS"}` + "\n```",
			want: entity.Classification{
				Category:        entity.CategoryNetwork,
				Severity:        entity.SeverityMedium,
				Summary:         "DNS timeout",
				SuggestedAction: "Check DNS",
			},
		},
		{
			name:  "prose around JSON",
			input: "Here is your classification:\n" + `{"category": "syntax_error", "severity": "low", "summary": "Missing semicolon", "suggested_action": "Add semicolon"}` + "\nHope this helps!",
			want: entity.Classification{
				Category:        entity.CategorySyntax,
				Severity:        entity.SeverityLow,
				Summary:         "Missing semicolon",
				SuggestedAction: "Add semicolon",
			},
		},
		{
			name:  "nested braces inside strings",
			input: `{"category": "database_error", "severity": "high", "summary": "SQL error in {users} table with {id}", "suggested_action": "Run migration {2026}"}`,
			want: entity.Classification{
				Category:        entity.CategoryDatabase,
				Severity:        entity.SeverityHigh,
				Summary:         "SQL error in {users} table with {id}",
				SuggestedAction: "Run migration {2026}",
			},
		},
		{
			name:  "escaped quotes and braces inside string",
			input: `{"category": "config_error", "severity": "medium", "summary": "Key \"{port}\" is missing", "suggested_action": "Add \"{port}\": 8080"}`,
			want: entity.Classification{
				Category:        entity.CategoryConfig,
				Severity:        entity.SeverityMedium,
				Summary:         `Key "{port}" is missing`,
				SuggestedAction: `Add "{port}": 8080`,
			},
		},
		{
			name:  "normalisation of category and severity",
			input: `{"category": "  RUNTIME-ERROR  ", "severity": "  CRITICAL  ", "summary": "  Panic  ", "suggested_action": "  Restart  "}`,
			want: entity.Classification{
				Category:        entity.CategoryRuntime,
				Severity:        entity.SeverityCritical,
				Summary:         "Panic",
				SuggestedAction: "Restart",
			},
		},
		{
			name:  "category with spaces",
			input: `{"category": "filesystem error", "severity": "low", "summary": "File not found", "suggested_action": "Create file"}`,
			want: entity.Classification{
				Category:        entity.CategoryFilesystem,
				Severity:        entity.SeverityLow,
				Summary:         "File not found",
				SuggestedAction: "Create file",
			},
		},
		{
			name:  "cut fields at 500 runes",
			input: `{"category": "timeout", "severity": "low", "summary": "` + longSummary + `", "suggested_action": "` + longAction + `"}`,
			want: entity.Classification{
				Category:        entity.CategoryTimeout,
				Severity:        entity.SeverityLow,
				Summary:         strings.Repeat("s", 500),
				SuggestedAction: strings.Repeat("a", 500),
			},
		},
		{
			name:  "unknown fields allowed",
			input: `{"category": "auth_error", "severity": "high", "summary": "Token expired", "suggested_action": "Login again", "confidence": 0.95}`,
			want: entity.Classification{
				Category:        entity.CategoryAuth,
				Severity:        entity.SeverityHigh,
				Summary:         "Token expired",
				SuggestedAction: "Login again",
			},
		},
		{
			name:    "no JSON at all",
			input:   "This is raw text without any braces.",
			wantErr: true,
		},
		{
			name:    "empty object",
			input:   "{}",
			wantErr: true,
		},
		{
			name:    "unclosed JSON",
			input:   `{"category": "syntax_error"`,
			wantErr: true,
		},
		{
			name:    "unknown category",
			input:   `{"category": "hardware_failure", "severity": "high", "summary": "Burned CPU", "suggested_action": "Replace CPU"}`,
			wantErr: true,
		},
		{
			name:    "unknown severity",
			input:   `{"category": "syntax_error", "severity": "fatal", "summary": "Syntax error", "suggested_action": "Fix syntax"}`,
			wantErr: true,
		},
		{
			name:    "empty summary",
			input:   `{"category": "syntax_error", "severity": "low", "summary": "   ", "suggested_action": "Fix syntax"}`,
			wantErr: true,
		},
		{
			name:    "empty suggested_action",
			input:   `{"category": "syntax_error", "severity": "low", "summary": "Syntax error", "suggested_action": "   "}`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := usecase.ParseClassification(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tc.input)
				}

				if !errors.Is(err, entity.ErrInvalidModelOutput) {
					t.Fatalf("expected error wrapping ErrInvalidModelOutput, got %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
