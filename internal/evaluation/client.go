package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
)

type Outcome struct {
	Classification entity.Classification `json:"classification"`
	StatusCode     int                   `json:"status_code"`
	Latency        time.Duration         `json:"latency"`
	Err            error                 `json:"error,omitempty"`
}

type classifyRequest struct {
	Error string `json:"error"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

// NewClient creates a Client calling the classifier service.
func NewClient(rawURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(rawURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// Classify sends a sample error message to POST /classify.
func (c *Client) Classify(ctx context.Context, sample Sample) Outcome {
	endpoint := c.baseURL
	if !strings.HasSuffix(endpoint, "/classify") {
		endpoint += "/classify"
	}

	body, err := json.Marshal(classifyRequest{Error: sample.Error})
	if err != nil {
		return Outcome{
			Err: fmt.Errorf("marshal request: %w", err),
		}
	}

	reqCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Outcome{
			Err: fmt.Errorf("create request: %w", err),
		}
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	latency := time.Since(start)

	if err != nil {
		return Outcome{
			Latency: latency,
			Err:     err,
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Outcome{
			StatusCode: resp.StatusCode,
			Latency:    latency,
			Err:        fmt.Errorf("read response body: %w", err),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return Outcome{
			StatusCode: resp.StatusCode,
			Latency:    latency,
			Err:        fmt.Errorf("server returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))),
		}
	}

	var classification entity.Classification
	if err := json.Unmarshal(respBody, &classification); err != nil {
		return Outcome{
			StatusCode: resp.StatusCode,
			Latency:    latency,
			Err:        fmt.Errorf("unmarshal classification: %w", err),
		}
	}

	if err := classification.Validate(); err != nil {
		return Outcome{
			Classification: classification,
			StatusCode:     resp.StatusCode,
			Latency:        latency,
			Err:            err,
		}
	}

	return Outcome{
		Classification: classification,
		StatusCode:     resp.StatusCode,
		Latency:        latency,
	}
}

// ClassifyText sends a raw error string and returns classification, HTTP status, and latency.
func (c *Client) ClassifyText(ctx context.Context, errorText string) (entity.Classification, int, time.Duration, error) {
	outcome := c.Classify(ctx, Sample{Error: errorText})

	return outcome.Classification, outcome.StatusCode, outcome.Latency, outcome.Err
}
