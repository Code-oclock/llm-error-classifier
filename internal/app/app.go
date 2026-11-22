// Package app is the composition root: config, adapters, use case, server.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/adapter/metrics"
	"github.com/Code-oclock/llm-error-classifier/internal/adapter/openai"
	"github.com/Code-oclock/llm-error-classifier/internal/config"
	"github.com/Code-oclock/llm-error-classifier/internal/controller/httpctl"
	"github.com/Code-oclock/llm-error-classifier/internal/usecase"
)

const shutdownTimeout = 15 * time.Second

// Run serves until ctx is done, then shuts down gracefully.
func Run(ctx context.Context, getenv func(string) string, log *slog.Logger) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log.Info("starting",
		"listen", cfg.ListenAddr, "model", cfg.LLMModel, "backend", redact(cfg.LLMBaseURL),
		"max_concurrent", cfg.MaxConcurrent, "max_attempts", cfg.MaxAttempts,
		"llm_timeout", cfg.LLMTimeout, "language", cfg.Language)

	llm := openai.New(openai.Config{
		BaseURL:         cfg.LLMBaseURL,
		Model:           cfg.LLMModel,
		APIKey:          cfg.LLMAPIKey,
		Timeout:         cfg.LLMTimeout,
		ReasoningEffort: cfg.LLMReasoningEffort,
		MaxTokens:       cfg.LLMMaxTokens,
	})

	obs := metrics.New()
	obs.SetModel(cfg.LLMModel)

	classifier := usecase.NewClassifier(llm, obs, usecase.Config{
		Language:      cfg.Language,
		MaxInputChars: cfg.MaxInputChars,
		MaxAttempts:   cfg.MaxAttempts,
		MaxConcurrent: cfg.MaxConcurrent,
		QueueWait:     cfg.QueueWait,
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpctl.NewHandler(classifier, llm.Ping, obs.Handler(), log),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      cfg.LLMTimeout*time.Duration(cfg.MaxAttempts) + cfg.QueueWait + 40*time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)

	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

// redact hides credentials embedded in a URL before it is logged.
func redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid url>"
	}

	return u.Redacted()
}
