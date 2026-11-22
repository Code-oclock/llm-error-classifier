// Package usecase holds the classification logic and the ports it needs.
package usecase

import (
	"context"
	"encoding/json"
	"time"
)

// LLMRequest is one chat completion request to the model backend.
type LLMRequest struct {
	System      string
	User        string
	Schema      json.RawMessage // JSON schema the reply must follow
	Temperature float64
}

type LLMResponse struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
}

// LLM is the model backend. Implementations return entity.ErrModelTimeout
// or entity.ErrModelUnavailable (wrapped) for transport-level failures.
type LLM interface {
	Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)
	// Ping reports whether the backend is reachable and the model is present.
	Ping(ctx context.Context) error
}

// Outcome labels the result of one Classify call for metrics.
type Outcome string

const (
	OutcomeOK            Outcome = "ok"
	OutcomeInvalidOutput Outcome = "invalid_output"
	OutcomeBackendError  Outcome = "backend_error"
	OutcomeTimeout       Outcome = "timeout"
	OutcomeBusy          Outcome = "busy"
	OutcomeBadInput      Outcome = "bad_input"
)

// Event describes one finished Classify call.
type Event struct {
	Outcome          Outcome
	Duration         time.Duration
	Attempts         int // model calls made
	PromptTokens     int
	CompletionTokens int
}

// Observer receives one Event per Classify call and tracks in-flight calls.
type Observer interface {
	Observe(Event)
	InFlight(delta int)
}

type NopObserver struct{}

func (NopObserver) Observe(Event) {}
func (NopObserver) InFlight(int)  {}
