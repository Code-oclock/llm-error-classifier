// Package config reads the service settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr         string
	LLMBaseURL         string
	LLMModel           string
	LLMAPIKey          string
	LLMReasoningEffort string
	LLMMaxTokens       int
	LLMTimeout         time.Duration
	Language           string
	MaxInputChars      int
	MaxAttempts        int
	MaxConcurrent      int
	QueueWait          time.Duration
	LogLevel           string
}

// Load reads and validates the configuration; all problems are reported at once.
func Load(getenv func(string) string) (Config, error) {
	var errs []error

	str := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}

		return def
	}
	num := func(key string, def int) int {
		v := getenv(key)
		if v == "" {
			return def
		}

		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive integer, got %q", key, v))

			return def
		}

		return n
	}
	dur := func(key string, def time.Duration) time.Duration {
		v := getenv(key)
		if v == "" {
			return def
		}

		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive duration like 30s, got %q", key, v))

			return def
		}

		return d
	}

	c := Config{
		ListenAddr:         str("LISTEN_ADDR", ":8080"),
		LLMBaseURL:         str("LLM_BASE_URL", "http://ollama:11434/v1"),
		LLMModel:           getenv("LLM_MODEL"),
		LLMAPIKey:          getenv("LLM_API_KEY"),
		LLMReasoningEffort: str("LLM_REASONING_EFFORT", "none"),
		LLMMaxTokens:       num("LLM_MAX_TOKENS", 400),
		LLMTimeout:         dur("LLM_TIMEOUT", 120*time.Second),
		Language:           str("CLASSIFY_LANGUAGE", "English"),
		MaxInputChars:      num("MAX_INPUT_CHARS", 4000),
		MaxAttempts:        num("MAX_ATTEMPTS", 2),
		MaxConcurrent:      num("MAX_CONCURRENT", 1),
		QueueWait:          dur("QUEUE_WAIT", 30*time.Second),
		LogLevel:           str("LOG_LEVEL", "info"),
	}

	if c.LLMModel == "" {
		errs = append(errs, errors.New("LLM_MODEL is required"))
	}

	return c, errors.Join(errs...)
}
