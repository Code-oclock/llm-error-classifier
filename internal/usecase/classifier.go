package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
)

const (
	defaultLanguage      = "English"
	defaultMaxInputChars = 4000
	defaultMaxAttempts   = 2
	defaultMaxConcurrent = 1
	defaultQueueWait     = 30 * time.Second
)

// Config controls classifier execution parameters.
type Config struct {
	Language      string
	MaxInputChars int
	MaxAttempts   int
	MaxConcurrent int
	QueueWait     time.Duration
}

// Classifier coordinates model completion, parsing, and observation.
type Classifier struct {
	llm LLM
	obs Observer
	cfg Config
	sem chan struct{}
}

// NewClassifier constructs a Classifier, setting defaults for empty values.
func NewClassifier(llm LLM, obs Observer, cfg Config) *Classifier {
	if cfg.Language == "" {
		cfg.Language = defaultLanguage
	}

	if cfg.MaxInputChars <= 0 {
		cfg.MaxInputChars = defaultMaxInputChars
	}

	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}

	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = defaultMaxConcurrent
	}

	if cfg.QueueWait <= 0 {
		cfg.QueueWait = defaultQueueWait
	}

	if obs == nil {
		obs = NopObserver{}
	}

	return &Classifier{
		llm: llm,
		obs: obs,
		cfg: cfg,
		sem: make(chan struct{}, cfg.MaxConcurrent),
	}
}

// Classify classifies an error message using the configured LLM.
func (c *Classifier) Classify(ctx context.Context, text string) (entity.Classification, error) {
	start := time.Now()

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		c.obs.Observe(Event{
			Outcome:  OutcomeBadInput,
			Duration: time.Since(start),
			Attempts: 0,
		})

		return entity.Classification{}, entity.ErrEmptyInput
	}

	input := truncate(trimmed, c.cfg.MaxInputChars)

	if err := ctx.Err(); err != nil {
		c.obs.Observe(Event{
			Outcome:  OutcomeTimeout,
			Duration: time.Since(start),
			Attempts: 0,
		})

		return entity.Classification{}, fmt.Errorf("%w: %w", entity.ErrModelTimeout, err)
	}

	timer := time.NewTimer(c.cfg.QueueWait)
	defer timer.Stop()

	select {
	case c.sem <- struct{}{}:
	case <-timer.C:
		c.obs.Observe(Event{
			Outcome:  OutcomeBusy,
			Duration: time.Since(start),
			Attempts: 0,
		})

		return entity.Classification{}, entity.ErrBusy
	case <-ctx.Done():
		c.obs.Observe(Event{
			Outcome:  OutcomeTimeout,
			Duration: time.Since(start),
			Attempts: 0,
		})

		return entity.Classification{}, fmt.Errorf("%w: %w", entity.ErrModelTimeout, ctx.Err())
	}

	c.obs.InFlight(1)
	defer func() {
		<-c.sem
		c.obs.InFlight(-1)
	}()

	userPrompt := "Classify this error message:\n\n" + input
	req := LLMRequest{
		System: SystemPrompt(c.cfg.Language),
		User:   userPrompt,
		Schema: Schema(),
	}

	var (
		totalPromptTokens     int
		totalCompletionTokens int
		lastErr               error
	)

	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		if attempt == 1 {
			req.Temperature = 0.0
		} else {
			req.Temperature = 0.3
		}

		resp, err := c.llm.Complete(ctx, req)
		if err != nil {
			totalPromptTokens += resp.PromptTokens
			totalCompletionTokens += resp.CompletionTokens

			outcome, finalErr := classifyBackendError(ctx, err)

			c.obs.Observe(Event{
				Outcome:          outcome,
				Duration:         time.Since(start),
				Attempts:         attempt,
				PromptTokens:     totalPromptTokens,
				CompletionTokens: totalCompletionTokens,
			})

			return entity.Classification{}, finalErr
		}

		totalPromptTokens += resp.PromptTokens
		totalCompletionTokens += resp.CompletionTokens

		classification, parseErr := parseClassification(resp.Content)
		if parseErr == nil {
			c.obs.Observe(Event{
				Outcome:          OutcomeOK,
				Duration:         time.Since(start),
				Attempts:         attempt,
				PromptTokens:     totalPromptTokens,
				CompletionTokens: totalCompletionTokens,
			})

			return classification, nil
		}

		lastErr = parseErr
	}

	if !errors.Is(lastErr, entity.ErrInvalidModelOutput) {
		lastErr = fmt.Errorf("%w: %w", entity.ErrInvalidModelOutput, lastErr)
	}

	c.obs.Observe(Event{
		Outcome:          OutcomeInvalidOutput,
		Duration:         time.Since(start),
		Attempts:         c.cfg.MaxAttempts,
		PromptTokens:     totalPromptTokens,
		CompletionTokens: totalCompletionTokens,
	})

	return entity.Classification{}, lastErr
}

// truncate truncates text to maxChars runes keeping the head and tail.
func truncate(text string, maxChars int) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}

	headLen := maxChars * 7 / 10
	tailLen := maxChars - headLen
	head := string(runes[:headLen])
	tail := string(runes[len(runes)-tailLen:])

	return head + "\n[... truncated ...]\n" + tail
}

// classifyBackendError maps a backend failure to a metrics outcome and a domain error.
func classifyBackendError(ctx context.Context, err error) (Outcome, error) {
	switch {
	case errors.Is(err, entity.ErrModelTimeout):
		return OutcomeTimeout, err
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), ctx.Err() != nil:
		ctxErr := ctx.Err()
		if ctxErr == nil {
			ctxErr = err
		}

		return OutcomeTimeout, fmt.Errorf("%w: %w", entity.ErrModelTimeout, ctxErr)
	case errors.Is(err, entity.ErrModelUnavailable):
		return OutcomeBackendError, err
	default:
		return OutcomeBackendError, fmt.Errorf("%w: %w", entity.ErrModelUnavailable, err)
	}
}
