package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/config"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := config.Load(env(map[string]string{"LLM_MODEL": "qwen3.5:2b"}))
	if err != nil {
		t.Fatal(err)
	}

	if c.ListenAddr != ":8080" || c.MaxConcurrent != 2 || c.LLMTimeout != 120*time.Second || c.LLMReasoningEffort != "none" {
		t.Errorf("unexpected defaults: %+v", c)
	}
}

func TestLoadOverrides(t *testing.T) {
	c, err := config.Load(env(map[string]string{
		"LLM_MODEL": "m", "MAX_CONCURRENT": "4", "QUEUE_WAIT": "5s", "CLASSIFY_LANGUAGE": "Russian",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if c.MaxConcurrent != 4 || c.QueueWait != 5*time.Second || c.Language != "Russian" {
		t.Errorf("unexpected: %+v", c)
	}
}

func TestLoadReportsAllProblems(t *testing.T) {
	_, err := config.Load(env(map[string]string{"MAX_ATTEMPTS": "0", "LLM_TIMEOUT": "soon"}))
	if err == nil {
		t.Fatal("expected an error")
	}

	for _, want := range []string{"LLM_MODEL", "MAX_ATTEMPTS", "LLM_TIMEOUT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %s", err, want)
		}
	}
}
