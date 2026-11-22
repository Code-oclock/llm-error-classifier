// Package openai implements an LLM adapter for OpenAI-compatible APIs.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
	"github.com/Code-oclock/llm-error-classifier/internal/usecase"
)

const (
	defaultTimeout   = 120 * time.Second
	defaultMaxTokens = 400
	maxBodyBytes     = 4 * 1024 * 1024
	maxErrorBytes    = 200
)

// Config holds configuration for the OpenAI adapter.
type Config struct {
	BaseURL         string
	Model           string
	APIKey          string
	Timeout         time.Duration
	ReasoningEffort string
	MaxTokens       int
	HTTPClient      *http.Client
}

// Client implements usecase.LLM for OpenAI-compatible backends.
type Client struct {
	baseURL         string
	model           string
	apiKey          string
	reasoningEffort string
	maxTokens       int
	httpClient      *http.Client
}

var _ usecase.LLM = (*Client)(nil)

// New creates an OpenAI-compatible LLM client.
func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		var transport http.RoundTripper
		if tr, ok := http.DefaultTransport.(*http.Transport); ok {
			cloned := tr.Clone()
			cloned.Proxy = http.ProxyFromEnvironment
			transport = cloned
		} else {
			transport = &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			}
		}

		httpClient = &http.Client{
			Timeout:   timeout,
			Transport: transport,
		}
	}

	return &Client{
		baseURL:         strings.TrimRight(cfg.BaseURL, "/"),
		model:           cfg.Model,
		apiKey:          cfg.APIKey,
		reasoningEffort: cfg.ReasoningEffort,
		maxTokens:       maxTokens,
		httpClient:      httpClient,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type jsonSchemaObject struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type responseFormat struct {
	Type       string           `json:"type"`
	JSONSchema jsonSchemaObject `json:"json_schema"`
}

type chatCompletionRequest struct {
	Model           string          `json:"model"`
	Messages        []chatMessage   `json:"messages"`
	Temperature     float64         `json:"temperature"`
	MaxTokens       int             `json:"max_tokens"`
	Stream          bool            `json:"stream"`
	ResponseFormat  *responseFormat `json:"response_format,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// Complete sends a chat completion request to the backend.
func (c *Client) Complete(ctx context.Context, req usecase.LLMRequest) (usecase.LLMResponse, error) {
	reqBody := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		},
		Temperature: req.Temperature,
		MaxTokens:   c.maxTokens,
		Stream:      false,
	}

	if len(req.Schema) > 0 {
		reqBody.ResponseFormat = &responseFormat{
			Type: "json_schema",
			JSONSchema: jsonSchemaObject{
				Name:   "error_classification",
				Strict: true,
				Schema: req.Schema,
			},
		}
	}

	if c.reasoningEffort != "" {
		reqBody.ReasoningEffort = c.reasoningEffort
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return usecase.LLMResponse{}, fmt.Errorf("%w: failed to marshal request: %w", entity.ErrModelUnavailable, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return usecase.LLMResponse{}, fmt.Errorf("%w: failed to create request: %w", entity.ErrModelUnavailable, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return usecase.LLMResponse{}, mapNetworkError(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return usecase.LLMResponse{}, mapStatusError(resp)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return usecase.LLMResponse{}, fmt.Errorf("%w: failed to read response body: %w", entity.ErrModelUnavailable, err)
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return usecase.LLMResponse{}, fmt.Errorf("%w: failed to decode response JSON: %w", entity.ErrModelUnavailable, err)
	}

	if len(chatResp.Choices) == 0 {
		return usecase.LLMResponse{}, fmt.Errorf("%w: empty choices in response", entity.ErrModelUnavailable)
	}

	res := usecase.LLMResponse{
		Content: chatResp.Choices[0].Message.Content,
	}

	if chatResp.Usage != nil {
		res.PromptTokens = chatResp.Usage.PromptTokens
		res.CompletionTokens = chatResp.Usage.CompletionTokens
	}

	return res, nil
}

// Ping checks if the backend is reachable and the configured model exists.
func (c *Client) Ping(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", http.NoBody)
	if err != nil {
		return fmt.Errorf("%w: failed to create request: %w", entity.ErrModelUnavailable, err)
	}

	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return mapNetworkError(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return mapStatusError(resp)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: failed to read models response body: %w", entity.ErrModelUnavailable, err)
	}

	var models modelsResponse
	if err := json.Unmarshal(body, &models); err != nil {
		return fmt.Errorf("%w: failed to decode models response JSON: %w", entity.ErrModelUnavailable, err)
	}

	hasTag := strings.Contains(c.model, ":")
	for _, m := range models.Data {
		if m.ID == c.model {
			return nil
		}

		if !hasTag && m.ID == c.model+":latest" {
			return nil
		}
	}

	return fmt.Errorf("%w: model %s not found on backend", entity.ErrModelUnavailable, c.model)
}

func mapNetworkError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		cause := ctx.Err()
		if cause == nil {
			cause = err
		}

		return fmt.Errorf("%w: %w", entity.ErrModelTimeout, cause)
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", entity.ErrModelTimeout, err)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w: %w", entity.ErrModelTimeout, err)
	}

	return fmt.Errorf("%w: %w", entity.ErrModelUnavailable, err)
}

func mapStatusError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
	if err != nil {
		return fmt.Errorf("%w: HTTP %d: failed to read response body: %w", entity.ErrModelUnavailable, resp.StatusCode, err)
	}

	snippet := strings.TrimSpace(string(body))

	if resp.StatusCode == http.StatusNotFound {
		if snippet != "" {
			return fmt.Errorf("%w: HTTP 404 (model not found): %s", entity.ErrModelUnavailable, snippet)
		}

		return fmt.Errorf("%w: HTTP 404 (model not found)", entity.ErrModelUnavailable)
	}

	if snippet != "" {
		return fmt.Errorf("%w: HTTP %d: %s", entity.ErrModelUnavailable, resp.StatusCode, snippet)
	}

	return fmt.Errorf("%w: HTTP %d", entity.ErrModelUnavailable, resp.StatusCode)
}
