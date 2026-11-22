package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/adapter/metrics"
	"github.com/Code-oclock/llm-error-classifier/internal/usecase"
)

func TestObserverExposesMetrics(t *testing.T) {
	o := metrics.New()
	o.SetModel("m1")
	o.InFlight(1)
	o.Observe(usecase.Event{Outcome: usecase.OutcomeOK, Duration: 2 * time.Second, Attempts: 1, PromptTokens: 10, CompletionTokens: 20})
	o.Observe(usecase.Event{Outcome: usecase.OutcomeBusy, Duration: time.Second})

	rec := httptest.NewRecorder()
	o.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), "GET", "/metrics", http.NoBody))

	body := rec.Body.String()
	for _, want := range []string{
		`classifier_requests_total{outcome="ok"} 1`,
		`classifier_requests_total{outcome="busy"} 1`,
		`classifier_requests_total{outcome="timeout"} 0`,
		`classifier_in_flight 1`,
		`classifier_prompt_tokens_total 10`,
		`classifier_completion_tokens_total 20`,
		`classifier_model_info{model="m1"} 1`,
		`classifier_tokens_per_second_count 1`,
		`classifier_llm_attempts_count 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics lack %q", want)
		}
	}
}
