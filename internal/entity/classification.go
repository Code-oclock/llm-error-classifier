// Package entity holds the domain model of the error classifier.
package entity

import (
	"errors"
	"fmt"
	"strings"
)

type Category string

const (
	CategorySyntax            Category = "syntax_error"
	CategoryRuntime           Category = "runtime_error"
	CategoryNetwork           Category = "network_error"
	CategoryTimeout           Category = "timeout"
	CategoryDatabase          Category = "database_error"
	CategoryAuth              Category = "auth_error"
	CategoryConfig            Category = "config_error"
	CategoryDependency        Category = "dependency_error"
	CategoryResourceExhausted Category = "resource_exhausted"
	CategoryFilesystem        Category = "filesystem_error"
	CategoryUnknown           Category = "unknown"
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Definition is a dictionary value with the meaning shown to the model.
type Definition[T ~string] struct {
	Value       T
	Description string
}

// Categories is the full category dictionary, in prompt order.
func Categories() []Definition[Category] {
	return []Definition[Category]{
		{CategorySyntax, "code does not parse or compile: syntax errors, compiler errors, invalid markup or config syntax"},
		{CategoryRuntime, "bug in running code: TypeError, NullPointerException, undefined access, panics, assertion failures, unhandled exceptions, bad arguments"},
		{CategoryNetwork, "connection refused or reset, DNS failure, TLS errors, unreachable host, HTTP 5xx from an upstream service"},
		{CategoryTimeout, "an operation exceeded its deadline: request, query, lock or job timeout"},
		{CategoryDatabase, "database failures: SQL errors, constraint violations, deadlocks, migrations, connection pool exhausted"},
		{CategoryAuth, "authentication or authorization failure: bad credentials, expired token, 401, 403, permission denied for a user"},
		{CategoryConfig, "wrong or missing configuration: env vars, config files, invalid flags, bad port or address settings"},
		{CategoryDependency, "missing, incompatible or broken package, module, library or import"},
		{CategoryResourceExhausted, "out of memory, disk full, too many open files, CPU or quota limits, rate limiting"},
		{CategoryFilesystem, "file or directory problems: not found, permission denied on a path, read-only filesystem, IO errors"},
		{CategoryUnknown, "none of the above fits or the text is not an error message"},
	}
}

// Severities is the full severity dictionary, from least to most severe.
func Severities() []Definition[Severity] {
	return []Definition[Severity]{
		{SeverityLow, "warning or cosmetic issue, no real impact on users, can wait"},
		{SeverityMedium, "one feature or a part of requests fails, a workaround exists, fix within days"},
		{SeverityHigh, "important functionality is down or a service cannot start, fix soon"},
		{SeverityCritical, "whole service down, data loss or corruption, security breach, fix immediately"},
	}
}

func (c Category) Valid() bool {
	for _, d := range Categories() {
		if d.Value == c {
			return true
		}
	}

	return false
}

func (s Severity) Valid() bool {
	for _, d := range Severities() {
		if d.Value == s {
			return true
		}
	}

	return false
}

// Classification is the structured verdict for one error message.
type Classification struct {
	Category        Category `json:"category"`
	Severity        Severity `json:"severity"`
	Summary         string   `json:"summary"`
	SuggestedAction string   `json:"suggested_action"`
}

// Validate checks dictionary membership and non-empty text fields.
func (c Classification) Validate() error {
	var problems []string

	if !c.Category.Valid() {
		problems = append(problems, fmt.Sprintf("unknown category %q", c.Category))
	}

	if !c.Severity.Valid() {
		problems = append(problems, fmt.Sprintf("unknown severity %q", c.Severity))
	}

	if strings.TrimSpace(c.Summary) == "" {
		problems = append(problems, "empty summary")
	}

	if strings.TrimSpace(c.SuggestedAction) == "" {
		problems = append(problems, "empty suggested_action")
	}

	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalidModelOutput, strings.Join(problems, "; "))
	}

	return nil
}

var (
	ErrEmptyInput         = errors.New("error text is empty")
	ErrBusy               = errors.New("too many concurrent requests")
	ErrModelUnavailable   = errors.New("model backend unavailable")
	ErrModelTimeout       = errors.New("model backend timed out")
	ErrInvalidModelOutput = errors.New("model returned an invalid classification")
)
