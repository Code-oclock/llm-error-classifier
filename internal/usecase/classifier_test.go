package usecase_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
	"github.com/Code-oclock/llm-error-classifier/internal/usecase"
)

type fakeLLM struct {
	mu       sync.Mutex
	requests []usecase.LLMRequest
	complete func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error)
}

func (f *fakeLLM) Complete(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	complete := f.complete
	f.mu.Unlock()

	if complete != nil {
		return complete(ctx, req)
	}

	return usecase.LLMResponse{}, nil
}

func (f *fakeLLM) Ping(ctx context.Context) error {
	return nil
}

func (f *fakeLLM) Requests() []usecase.LLMRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	copied := make([]usecase.LLMRequest, len(f.requests))
	copy(copied, f.requests)

	return copied
}

type fakeObserver struct {
	mu       sync.Mutex
	events   []usecase.Event
	inFlight int
	deltas   []int
}

func (f *fakeObserver) Observe(e usecase.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.events = append(f.events, e)
}

func (f *fakeObserver) InFlight(delta int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.inFlight += delta
	f.deltas = append(f.deltas, delta)
}

func (f *fakeObserver) Events() []usecase.Event {
	f.mu.Lock()
	defer f.mu.Unlock()

	copied := make([]usecase.Event, len(f.events))
	copy(copied, f.events)

	return copied
}

func (f *fakeObserver) InFlightCurrent() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.inFlight
}

func (f *fakeObserver) Deltas() []int {
	f.mu.Lock()
	defer f.mu.Unlock()

	copied := make([]int, len(f.deltas))
	copy(copied, f.deltas)

	return copied
}

func TestClassifier_HappyPath(t *testing.T) {
	t.Parallel()

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			return usecase.LLMResponse{
				Content:          `{"category": "runtime_error", "severity": "high", "summary": "Null pointer", "suggested_action": "Check reference"}`,
				PromptTokens:     10,
				CompletionTokens: 5,
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{Language: "English"})

	got, err := c.Classify(context.Background(), "NullPointerException at line 10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := entity.Classification{
		Category:        entity.CategoryRuntime,
		Severity:        entity.SeverityHigh,
		Summary:         "Null pointer",
		SuggestedAction: "Check reference",
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	reqs := llm.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	if reqs[0].Temperature != 0.0 {
		t.Fatalf("expected temperature 0.0, got %f", reqs[0].Temperature)
	}

	if !strings.Contains(reqs[0].User, "NullPointerException at line 10") {
		t.Fatalf("expected error text in prompt, got %q", reqs[0].User)
	}

	events := obs.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Outcome != usecase.OutcomeOK {
		t.Fatalf("expected outcome OK, got %q", events[0].Outcome)
	}

	if events[0].Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", events[0].Attempts)
	}

	if events[0].PromptTokens != 10 || events[0].CompletionTokens != 5 {
		t.Fatalf("unexpected tokens: %+v", events[0])
	}
}

func TestClassifier_FencedJSON(t *testing.T) {
	t.Parallel()

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			return usecase.LLMResponse{
				Content: "```json\n" + `{"category": "syntax_error", "severity": "low", "summary": "Bad syntax", "suggested_action": "Fix"}` + "\n```",
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{})

	got, err := c.Classify(context.Background(), "unexpected token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Category != entity.CategorySyntax {
		t.Fatalf("expected syntax_error, got %s", got.Category)
	}
}

func TestClassifier_ProseAroundJSON(t *testing.T) {
	t.Parallel()

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			return usecase.LLMResponse{
				Content: "Here is your triage:\n" + `{"category": "network_error", "severity": "medium", "summary": "Unreachable", "suggested_action": "Retry"}` + "\nHope that helps!",
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{})

	got, err := c.Classify(context.Background(), "connection refused")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Category != entity.CategoryNetwork {
		t.Fatalf("expected network_error, got %s", got.Category)
	}
}

func TestClassifier_RetryThenSuccess(t *testing.T) {
	t.Parallel()

	var attemptCount int

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			attemptCount++
			if attemptCount == 1 {
				return usecase.LLMResponse{
					Content:          `{"category": "unknown_category_value", "severity": "low", "summary": "Something", "suggested_action": "Fix"}`,
					PromptTokens:     8,
					CompletionTokens: 4,
				}, nil
			}

			return usecase.LLMResponse{
				Content:          `{"category": "runtime_error", "severity": "high", "summary": "Panic", "suggested_action": "Check code"}`,
				PromptTokens:     10,
				CompletionTokens: 6,
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{MaxAttempts: 2})

	got, err := c.Classify(context.Background(), "fatal panic occurred")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Category != entity.CategoryRuntime {
		t.Fatalf("expected runtime_error, got %s", got.Category)
	}

	reqs := llm.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}

	if reqs[0].Temperature != 0.0 {
		t.Fatalf("expected temperature 0.0 on attempt 1, got %f", reqs[0].Temperature)
	}

	if reqs[1].Temperature != 0.3 {
		t.Fatalf("expected temperature 0.3 on attempt 2, got %f", reqs[1].Temperature)
	}

	events := obs.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Outcome != usecase.OutcomeOK {
		t.Fatalf("expected outcome OK, got %q", events[0].Outcome)
	}

	if events[0].Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", events[0].Attempts)
	}

	if events[0].PromptTokens != 18 || events[0].CompletionTokens != 10 {
		t.Fatalf("unexpected token sums: %+v", events[0])
	}
}

func TestClassifier_InvalidOnAllAttempts(t *testing.T) {
	t.Parallel()

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			return usecase.LLMResponse{
				Content:          "{}",
				PromptTokens:     5,
				CompletionTokens: 2,
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{MaxAttempts: 2})

	_, err := c.Classify(context.Background(), "some error")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, entity.ErrInvalidModelOutput) {
		t.Fatalf("expected error wrapping ErrInvalidModelOutput, got %v", err)
	}

	events := obs.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Outcome != usecase.OutcomeInvalidOutput {
		t.Fatalf("expected outcome invalid_output, got %q", events[0].Outcome)
	}

	if events[0].Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", events[0].Attempts)
	}

	if events[0].PromptTokens != 10 || events[0].CompletionTokens != 4 {
		t.Fatalf("unexpected tokens: %+v", events[0])
	}
}

func TestClassifier_BackendTimeoutNoRetry(t *testing.T) {
	t.Parallel()

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			return usecase.LLMResponse{}, entity.ErrModelTimeout
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{MaxAttempts: 2})

	_, err := c.Classify(context.Background(), "error message")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, entity.ErrModelTimeout) {
		t.Fatalf("expected ErrModelTimeout, got %v", err)
	}

	if len(llm.Requests()) != 1 {
		t.Fatalf("expected 1 call without retry, got %d", len(llm.Requests()))
	}

	events := obs.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Outcome != usecase.OutcomeTimeout {
		t.Fatalf("expected outcome timeout, got %q", events[0].Outcome)
	}

	if events[0].Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", events[0].Attempts)
	}
}

func TestClassifier_UnknownErrorWrapped(t *testing.T) {
	t.Parallel()

	origErr := errors.New("socket closed")
	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			return usecase.LLMResponse{}, origErr
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{MaxAttempts: 2})

	_, err := c.Classify(context.Background(), "error message")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, entity.ErrModelUnavailable) {
		t.Fatalf("expected error wrapping ErrModelUnavailable, got %v", err)
	}

	if !errors.Is(err, origErr) {
		t.Fatalf("expected error wrapping original error, got %v", err)
	}

	if len(llm.Requests()) != 1 {
		t.Fatalf("expected 1 call without retry, got %d", len(llm.Requests()))
	}

	events := obs.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Outcome != usecase.OutcomeBackendError {
		t.Fatalf("expected outcome backend_error, got %q", events[0].Outcome)
	}
}

func TestClassifier_EmptyInput(t *testing.T) {
	t.Parallel()

	llm := &fakeLLM{}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{})

	for _, input := range []string{"", "   \n\t  "} {
		_, err := c.Classify(context.Background(), input)
		if !errors.Is(err, entity.ErrEmptyInput) {
			t.Fatalf("expected ErrEmptyInput for %q, got %v", input, err)
		}
	}

	if len(llm.Requests()) != 0 {
		t.Fatalf("expected no model calls, got %d", len(llm.Requests()))
	}

	events := obs.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	for _, e := range events {
		if e.Outcome != usecase.OutcomeBadInput {
			t.Fatalf("expected outcome bad_input, got %q", e.Outcome)
		}

		if e.Attempts != 0 {
			t.Fatalf("expected 0 attempts, got %d", e.Attempts)
		}
	}
}

func TestClassifier_Truncation(t *testing.T) {
	t.Parallel()

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			return usecase.LLMResponse{
				Content: `{"category": "runtime_error", "severity": "low", "summary": "err", "suggested_action": "fix"}`,
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{MaxInputChars: 20})

	input := "0123456789abcdefghijklmnopqrstuvwxyz"
	_, err := c.Classify(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqs := llm.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// 20 runes: 70% is 14 runes, 30% is 6 runes.
	wantHead := "0123456789abcd"
	wantTail := "uvwxyz"
	expectedPart := wantHead + "\n[... truncated ...]\n" + wantTail

	if !strings.Contains(reqs[0].User, expectedPart) {
		t.Fatalf("expected prompt to contain %q, got %q", expectedPart, reqs[0].User)
	}
}

func TestClassifier_Concurrency(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	firstStarted := make(chan struct{})

	var once sync.Once

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			once.Do(func() {
				close(firstStarted)
			})
			<-block

			return usecase.LLMResponse{
				Content: `{"category": "runtime_error", "severity": "low", "summary": "ok", "suggested_action": "none"}`,
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{
		MaxConcurrent: 1,
		QueueWait:     20 * time.Millisecond,
	})

	firstDone := make(chan struct{})

	go func() {
		defer close(firstDone)

		_, _ = c.Classify(context.Background(), "first error")
	}()

	<-firstStarted

	_, err := c.Classify(context.Background(), "second error")
	if !errors.Is(err, entity.ErrBusy) {
		t.Fatalf("expected ErrBusy for second call, got %v", err)
	}

	close(block)
	<-firstDone

	if current := obs.InFlightCurrent(); current != 0 {
		t.Fatalf("expected inFlight to balance to 0, got %d", current)
	}

	deltas := obs.Deltas()
	if len(deltas) != 2 || deltas[0] != 1 || deltas[1] != -1 {
		t.Fatalf("expected deltas [1, -1], got %+v", deltas)
	}

	events := obs.Events()
	var foundBusy bool

	for _, e := range events {
		if e.Outcome == usecase.OutcomeBusy {
			foundBusy = true
			if e.Attempts != 0 {
				t.Fatalf("expected attempts=0 for busy outcome, got %d", e.Attempts)
			}
		}
	}

	if !foundBusy {
		t.Fatal("expected busy outcome event")
	}
}

func TestClassifier_ContextCanceledWhileWaitingForSlot(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	firstStarted := make(chan struct{})

	var once sync.Once

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			once.Do(func() {
				close(firstStarted)
			})
			<-block

			return usecase.LLMResponse{
				Content: `{"category": "runtime_error", "severity": "low", "summary": "ok", "suggested_action": "none"}`,
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{
		MaxConcurrent: 1,
		QueueWait:     5 * time.Second,
	})

	firstDone := make(chan struct{})

	go func() {
		defer close(firstDone)

		_, _ = c.Classify(context.Background(), "first error")
	}()

	<-firstStarted

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Classify(ctx, "second error")
	if !errors.Is(err, entity.ErrModelTimeout) {
		t.Fatalf("expected ErrModelTimeout, got %v", err)
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled wrapped, got %v", err)
	}

	close(block)
	<-firstDone

	if current := obs.InFlightCurrent(); current != 0 {
		t.Fatalf("expected inFlight to balance to 0, got %d", current)
	}
}

func TestClassifier_CancelWhileQueuedFreesSlot(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	firstStarted := make(chan struct{})

	var once sync.Once

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			once.Do(func() {
				close(firstStarted)
				<-block
			})

			return usecase.LLMResponse{
				Content: `{"category": "runtime_error", "severity": "low", "summary": "ok", "suggested_action": "none"}`,
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{MaxConcurrent: 1, QueueWait: 10 * time.Second})

	firstDone := make(chan struct{})

	go func() {
		defer close(firstDone)

		_, _ = c.Classify(context.Background(), "first error")
	}()

	<-firstStarted

	// The second request is already waiting for the only slot when its client goes away.
	ctx, cancel := context.WithCancel(context.Background())
	secondErr := make(chan error, 1)

	go func() {
		_, err := c.Classify(ctx, "second error")
		secondErr <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-secondErr:
		if !errors.Is(err, entity.ErrModelTimeout) || !errors.Is(err, context.Canceled) {
			t.Fatalf("expected ErrModelTimeout wrapping context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued request did not return after its context was cancelled")
	}

	close(block)
	<-firstDone

	// The cancelled request must not have kept the slot: the next one goes through.
	if _, err := c.Classify(context.Background(), "third error"); err != nil {
		t.Fatalf("slot leaked after cancellation: %v", err)
	}

	if current := obs.InFlightCurrent(); current != 0 {
		t.Fatalf("expected inFlight to balance to 0, got %d", current)
	}
}

func TestClassifier_TokenSums(t *testing.T) {
	t.Parallel()

	var attemptCount int

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			attemptCount++
			if attemptCount == 1 {
				return usecase.LLMResponse{
					Content:          "{}",
					PromptTokens:     15,
					CompletionTokens: 5,
				}, nil
			}

			return usecase.LLMResponse{
				Content:          `{"category": "syntax_error", "severity": "low", "summary": "err", "suggested_action": "fix"}`,
				PromptTokens:     20,
				CompletionTokens: 10,
			}, nil
		},
	}
	obs := &fakeObserver{}

	c := usecase.NewClassifier(llm, obs, usecase.Config{MaxAttempts: 2})

	_, err := c.Classify(context.Background(), "syntax error")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := obs.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].PromptTokens != 35 || events[0].CompletionTokens != 15 {
		t.Fatalf("expected prompt=35, completion=15, got prompt=%d, completion=%d", events[0].PromptTokens, events[0].CompletionTokens)
	}
}

func TestNewClassifier_Defaults(t *testing.T) {
	t.Parallel()

	llm := &fakeLLM{
		complete: func(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
			return usecase.LLMResponse{
				Content: `{"category": "runtime_error", "severity": "low", "summary": "ok", "suggested_action": "none"}`,
			}, nil
		},
	}

	c := usecase.NewClassifier(llm, nil, usecase.Config{})

	_, err := c.Classify(context.Background(), "runtime error")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqs := llm.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	if reqs[0].System != usecase.SystemPrompt("English") {
		t.Fatalf("expected English system prompt default, got %q", reqs[0].System)
	}
}
