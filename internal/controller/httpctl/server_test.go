package httpctl_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Code-oclock/llm-error-classifier/internal/controller/httpctl"
	"github.com/Code-oclock/llm-error-classifier/internal/entity"
)

type fakeClassifier struct {
	got string
	res entity.Classification
	err error
}

func (f *fakeClassifier) Classify(_ context.Context, text string) (entity.Classification, error) {
	f.got = text

	return f.res, f.err
}

func newServer(t *testing.T, c *fakeClassifier, ready error) *httptest.Server {
	t.Helper()

	metrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "metrics") })
	h := httpctl.NewHandler(c, func(context.Context) error { return ready }, metrics, slog.New(slog.DiscardHandler))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return srv
}

func post(t *testing.T, url, contentType, body string) (code int, respBody string, header http.Header) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/classify", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, string(out), resp.Header
}

func TestClassifySuccess(t *testing.T) {
	c := &fakeClassifier{res: entity.Classification{
		Category: entity.CategoryRuntime, Severity: entity.SeverityMedium, Summary: "s", SuggestedAction: "a",
	}}
	srv := newServer(t, c, nil)

	status, body, _ := post(t, srv.URL, "application/json", `{"error":"TypeError: x"}`)
	if status != http.StatusOK || c.got != "TypeError: x" {
		t.Fatalf("status=%d got=%q body=%s", status, c.got, body)
	}

	for _, want := range []string{`"category":"runtime_error"`, `"severity":"medium"`, `"suggested_action":"a"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body %s lacks %s", body, want)
		}
	}
}

func TestClassifyInputForms(t *testing.T) {
	tests := []struct {
		name, contentType, body, want string
		status                        int
	}{
		{"plain text", "text/plain; charset=utf-8", "panic: boom", "panic: boom", http.StatusOK},
		{"message key", "application/json", `{"message":"boom"}`, "boom", http.StatusOK},
		{"bad json", "application/json", `{`, "", http.StatusBadRequest},
		{"too large", "text/plain", strings.Repeat("x", 300<<10), "", http.StatusRequestEntityTooLarge},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &fakeClassifier{}
			srv := newServer(t, c, nil)

			status, _, _ := post(t, srv.URL, tc.contentType, tc.body)
			if status != tc.status || c.got != tc.want {
				t.Fatalf("status=%d got=%q", status, c.got)
			}
		})
	}
}

func TestClassifyErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{entity.ErrEmptyInput, http.StatusBadRequest, "empty_input"},
		{entity.ErrBusy, http.StatusTooManyRequests, "busy"},
		{fmt.Errorf("x: %w", entity.ErrModelTimeout), http.StatusGatewayTimeout, "model_timeout"},
		{fmt.Errorf("x: %w", entity.ErrInvalidModelOutput), http.StatusBadGateway, "invalid_model_output"},
		{fmt.Errorf("x: %w", entity.ErrModelUnavailable), http.StatusBadGateway, "model_unavailable"},
		{errors.New("secret detail"), http.StatusInternalServerError, "internal"},
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			srv := newServer(t, &fakeClassifier{err: tc.err}, nil)

			status, body, header := post(t, srv.URL, "text/plain", "boom")
			if status != tc.status || !strings.Contains(body, `"code":"`+tc.code+`"`) {
				t.Fatalf("status=%d body=%s", status, body)
			}

			if tc.status == http.StatusTooManyRequests && header.Get("Retry-After") == "" {
				t.Error("missing Retry-After")
			}

			if strings.Contains(body, "secret detail") {
				t.Error("internal error leaked")
			}
		})
	}
}

func TestProbesAndMetrics(t *testing.T) {
	get := func(url string) (int, string) {
		t.Helper()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		b, _ := io.ReadAll(resp.Body)

		return resp.StatusCode, string(b)
	}

	ok := newServer(t, &fakeClassifier{}, nil)
	if s, _ := get(ok.URL + "/healthz"); s != http.StatusOK {
		t.Errorf("healthz %d", s)
	}

	if s, _ := get(ok.URL + "/readyz"); s != http.StatusOK {
		t.Errorf("readyz %d", s)
	}

	if s, b := get(ok.URL + "/metrics"); s != http.StatusOK || b != "metrics" {
		t.Errorf("metrics %d %s", s, b)
	}

	down := newServer(t, &fakeClassifier{}, errors.New("model missing"))
	if s, b := get(down.URL + "/readyz"); s != http.StatusServiceUnavailable || !strings.Contains(b, "model missing") {
		t.Errorf("readyz down %d %s", s, b)
	}
}
