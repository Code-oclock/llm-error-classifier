package app_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/app"
)

const fakeReply = `{"choices":[{"message":{"content":"{\"category\":\"network_error\",\"severity\":\"high\",\"summary\":\"Connection refused\",\"suggested_action\":\"Start the database\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`

func TestRunServesClassifyAndMetrics(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"test-model"}]}`)
		case "/v1/chat/completions":
			_, _ = io.WriteString(w, fakeReply)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	lis, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := lis.Addr().String()
	_ = lis.Close()

	env := map[string]string{"LISTEN_ADDR": addr, "LLM_BASE_URL": backend.URL + "/v1", "LLM_MODEL": "test-model"}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() {
		done <- app.Run(ctx, func(k string) string { return env[k] }, slog.New(slog.DiscardHandler))
	}()

	base := "http://" + addr
	waitReady(t, base)

	status, body := do(t, http.MethodPost, base+"/classify", `{"error":"ECONNREFUSED 127.0.0.1:5432"}`)
	if status != http.StatusOK || !strings.Contains(body, `"category":"network_error"`) {
		t.Fatalf("classify: %d %s", status, body)
	}

	if _, body := do(t, http.MethodGet, base+"/metrics", ""); !strings.Contains(body, `classifier_requests_total{outcome="ok"} 1`) {
		t.Errorf("metrics lack the ok counter:\n%s", body)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestRunRejectsBadConfig(t *testing.T) {
	err := app.Run(t.Context(), func(string) string { return "" }, slog.New(slog.DiscardHandler))
	if err == nil || !strings.Contains(err.Error(), "LLM_MODEL") {
		t.Fatalf("expected a config error, got %v", err)
	}
}

func waitReady(t *testing.T, base string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if status, _ := do(t, http.MethodGet, base+"/readyz", ""); status == http.StatusOK {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("server did not become ready")
}

// do returns status 0 when the connection fails, so callers can poll.
func do(t *testing.T, method, url, body string) (status int, respBody string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, string(out)
}
