package openai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/adapter/openai"
	"github.com/Code-oclock/llm-error-classifier/internal/entity"
	"github.com/Code-oclock/llm-error-classifier/internal/usecase"
)

func TestClient_Complete_RequestShape(t *testing.T) {
	t.Run("full request with auth and schema", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}

			if r.URL.Path != "/chat/completions" {
				t.Errorf("expected /chat/completions, got %s", r.URL.Path)
			}

			if auth := r.Header.Get("Authorization"); auth != "Bearer secret-key" {
				t.Errorf("expected Bearer secret-key, got %s", auth)
			}

			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected application/json, got %s", ct)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}

			var reqData map[string]any
			if err := json.Unmarshal(body, &reqData); err != nil {
				t.Fatalf("failed to unmarshal request: %v", err)
			}

			if reqData["model"] != "test-model" {
				t.Errorf("expected model test-model, got %v", reqData["model"])
			}

			if reqData["temperature"] != 0.5 {
				t.Errorf("expected temperature 0.5, got %v", reqData["temperature"])
			}

			if reqData["max_tokens"] != float64(250) {
				t.Errorf("expected max_tokens 250, got %v", reqData["max_tokens"])
			}

			if reqData["stream"] != false {
				t.Errorf("expected stream false, got %v", reqData["stream"])
			}

			if reqData["reasoning_effort"] != "low" {
				t.Errorf("expected reasoning_effort low, got %v", reqData["reasoning_effort"])
			}

			rf, ok := reqData["response_format"].(map[string]any)
			if !ok {
				t.Fatalf("missing response_format: %v", reqData)
			}

			if rf["type"] != "json_schema" {
				t.Errorf("expected type json_schema, got %v", rf["type"])
			}

			js, ok := rf["json_schema"].(map[string]any)
			if !ok {
				t.Fatalf("missing json_schema: %v", rf)
			}

			if js["name"] != "error_classification" {
				t.Errorf("expected schema name error_classification, got %v", js["name"])
			}

			if js["strict"] != true {
				t.Errorf("expected strict true, got %v", js["strict"])
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		}))
		defer ts.Close()

		client := openai.New(openai.Config{
			BaseURL:         ts.URL,
			Model:           "test-model",
			APIKey:          "secret-key",
			ReasoningEffort: "low",
			MaxTokens:       250,
			Timeout:         5 * time.Second,
		})

		req := usecase.LLMRequest{
			System:      "sys-prompt",
			User:        "user-prompt",
			Schema:      json.RawMessage(`{"type":"object"}`),
			Temperature: 0.5,
		}

		resp, err := client.Complete(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Content != "ok" {
			t.Errorf("expected content ok, got %q", resp.Content)
		}
	})

	t.Run("omitted when empty", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if auth := r.Header.Get("Authorization"); auth != "" {
				t.Errorf("expected empty Authorization header, got %s", auth)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}

			var reqData map[string]any
			if err := json.Unmarshal(body, &reqData); err != nil {
				t.Fatalf("failed to unmarshal request: %v", err)
			}

			if _, ok := reqData["response_format"]; ok {
				t.Errorf("response_format should be omitted when schema is empty")
			}

			if _, ok := reqData["reasoning_effort"]; ok {
				t.Errorf("reasoning_effort should be omitted when empty")
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		}))
		defer ts.Close()

		client := openai.New(openai.Config{
			BaseURL: ts.URL,
			Model:   "test-model",
			Timeout: 5 * time.Second,
		})

		req := usecase.LLMRequest{
			System: "sys-prompt",
			User:   "user-prompt",
		}

		_, err := client.Complete(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestClient_Complete_SuccessWithUsage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "summary test"}}],
			"usage": {"prompt_tokens": 42, "completion_tokens": 17}
		}`))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	})

	resp, err := client.Complete(context.Background(), usecase.LLMRequest{User: "err"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "summary test" {
		t.Errorf("expected summary test, got %q", resp.Content)
	}

	if resp.PromptTokens != 42 {
		t.Errorf("expected 42 prompt tokens, got %d", resp.PromptTokens)
	}

	if resp.CompletionTokens != 17 {
		t.Errorf("expected 17 completion tokens, got %d", resp.CompletionTokens)
	}
}

func TestClient_Complete_SuccessWithoutUsage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "summary without usage"}}]
		}`))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	})

	resp, err := client.Complete(context.Background(), usecase.LLMRequest{User: "err"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "summary without usage" {
		t.Errorf("expected summary without usage, got %q", resp.Content)
	}

	if resp.PromptTokens != 0 || resp.CompletionTokens != 0 {
		t.Errorf("expected zero token counts, got prompt=%d completion=%d", resp.PromptTokens, resp.CompletionTokens)
	}
}

func TestClient_Complete_EmptyChoices(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices": []}`))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	})

	_, err := client.Complete(context.Background(), usecase.LLMRequest{User: "err"})
	if err == nil {
		t.Fatal("expected error for empty choices, got nil")
	}

	if !errors.Is(err, entity.ErrModelUnavailable) {
		t.Errorf("expected ErrModelUnavailable, got %v", err)
	}
}

func TestClient_Complete_NonJSON200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Not JSON</body></html>"))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	})

	_, err := client.Complete(context.Background(), usecase.LLMRequest{User: "err"})
	if err == nil {
		t.Fatal("expected error for non-JSON 200, got nil")
	}

	if !errors.Is(err, entity.ErrModelUnavailable) {
		t.Errorf("expected ErrModelUnavailable, got %v", err)
	}
}

func TestClient_Complete_500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal crash"))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	})

	_, err := client.Complete(context.Background(), usecase.LLMRequest{User: "err"})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}

	if !errors.Is(err, entity.ErrModelUnavailable) {
		t.Errorf("expected ErrModelUnavailable, got %v", err)
	}

	if !strings.Contains(err.Error(), "internal crash") {
		t.Errorf("expected error message to contain internal crash, got %v", err)
	}
}

func TestClient_Complete_404WithMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`model "llama3" not found`))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		Model:   "llama3",
		Timeout: 5 * time.Second,
	})

	_, err := client.Complete(context.Background(), usecase.LLMRequest{User: "err"})
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}

	if !errors.Is(err, entity.ErrModelUnavailable) {
		t.Errorf("expected ErrModelUnavailable, got %v", err)
	}

	if !strings.Contains(err.Error(), `model "llama3" not found`) {
		t.Errorf("expected error to include body message, got %v", err)
	}
}

func TestClient_Complete_429(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limit exceeded"))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	})

	_, err := client.Complete(context.Background(), usecase.LLMRequest{User: "err"})
	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}

	if !errors.Is(err, entity.ErrModelUnavailable) {
		t.Errorf("expected ErrModelUnavailable, got %v", err)
	}

	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("expected error message to contain rate limit exceeded, got %v", err)
	}
}

func TestClient_Complete_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		Model:   "test-model",
		Timeout: 20 * time.Millisecond,
	})

	_, err := client.Complete(context.Background(), usecase.LLMRequest{User: "err"})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if !errors.Is(err, entity.ErrModelTimeout) {
		t.Errorf("expected ErrModelTimeout, got %v", err)
	}
}

func TestClient_Complete_ClosedServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	baseURL := ts.URL
	ts.Close()

	client := openai.New(openai.Config{
		BaseURL: baseURL,
		Model:   "test-model",
		Timeout: 2 * time.Second,
	})

	_, err := client.Complete(context.Background(), usecase.LLMRequest{User: "err"})
	if err == nil {
		t.Fatal("expected error for closed server, got nil")
	}

	if !errors.Is(err, entity.ErrModelUnavailable) {
		t.Errorf("expected ErrModelUnavailable, got %v", err)
	}
}

func TestClient_Complete_ContextCanceled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer ts.Close()

	client := openai.New(openai.Config{
		BaseURL: ts.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Complete(ctx, usecase.LLMRequest{User: "err"})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}

	if !errors.Is(err, entity.ErrModelTimeout) {
		t.Errorf("expected ErrModelTimeout, got %v", err)
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected wrapped context.Canceled, got %v", err)
	}
}

func TestClient_Ping(t *testing.T) {
	t.Run("found exact", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}

			if r.URL.Path != "/models" {
				t.Errorf("expected /models, got %s", r.URL.Path)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"llama3:8b"}]}`))
		}))
		defer ts.Close()

		client := openai.New(openai.Config{
			BaseURL: ts.URL,
			Model:   "llama3:8b",
			Timeout: 5 * time.Second,
		})

		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("tag-less match with latest", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"llama3:latest"}]}`))
		}))
		defer ts.Close()

		client := openai.New(openai.Config{
			BaseURL: ts.URL,
			Model:   "llama3",
			Timeout: 5 * time.Second,
		})

		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"mistral:latest"}]}`))
		}))
		defer ts.Close()

		client := openai.New(openai.Config{
			BaseURL: ts.URL,
			Model:   "llama3",
			Timeout: 5 * time.Second,
		})

		err := client.Ping(context.Background())
		if err == nil {
			t.Fatal("expected error for missing model, got nil")
		}

		if !errors.Is(err, entity.ErrModelUnavailable) {
			t.Errorf("expected ErrModelUnavailable, got %v", err)
		}

		if !strings.Contains(err.Error(), "model llama3 not found on backend") {
			t.Errorf("expected error message to contain 'model llama3 not found on backend', got %v", err)
		}
	})

	t.Run("tagged model does not match latest suffix", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"llama3:latest"}]}`))
		}))
		defer ts.Close()

		client := openai.New(openai.Config{
			BaseURL: ts.URL,
			Model:   "llama3:8b",
			Timeout: 5 * time.Second,
		})

		err := client.Ping(context.Background())
		if err == nil {
			t.Fatal("expected error when model has explicit tag, got nil")
		}

		if !errors.Is(err, entity.ErrModelUnavailable) {
			t.Errorf("expected ErrModelUnavailable, got %v", err)
		}
	})

	t.Run("500 response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("backend failure"))
		}))
		defer ts.Close()

		client := openai.New(openai.Config{
			BaseURL: ts.URL,
			Model:   "llama3",
			Timeout: 5 * time.Second,
		})

		err := client.Ping(context.Background())
		if err == nil {
			t.Fatal("expected error for 500, got nil")
		}

		if !errors.Is(err, entity.ErrModelUnavailable) {
			t.Errorf("expected ErrModelUnavailable, got %v", err)
		}
	})

	t.Run("auth header on ping", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if auth := r.Header.Get("Authorization"); auth != "Bearer ping-key" {
				t.Errorf("expected Bearer ping-key, got %s", auth)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
		}))
		defer ts.Close()

		client := openai.New(openai.Config{
			BaseURL: ts.URL,
			Model:   "test-model",
			APIKey:  "ping-key",
			Timeout: 5 * time.Second,
		})

		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestNew_Defaults(t *testing.T) {
	client := openai.New(openai.Config{
		BaseURL: "http://example.com/v1/",
		Model:   "model",
	})

	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
