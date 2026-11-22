// Package metrics exposes classifier metrics in Prometheus format.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Code-oclock/llm-error-classifier/internal/usecase"
)

// Observer implements usecase.Observer on top of a private registry.
type Observer struct {
	registry   *prometheus.Registry
	requests   *prometheus.CounterVec
	duration   prometheus.Histogram
	inFlight   prometheus.Gauge
	attempts   prometheus.Histogram
	promptTok  prometheus.Counter
	complTok   prometheus.Counter
	tokensPerS prometheus.Histogram
	modelInfo  *prometheus.GaugeVec
}

var _ usecase.Observer = (*Observer)(nil)

func New() *Observer {
	o := &Observer{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "classifier_requests_total",
			Help: "Classify calls by outcome.",
		}, []string{"outcome"}),
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "classifier_request_duration_seconds",
			Help:    "Classify call duration, including queueing.",
			Buckets: []float64{0.5, 1, 2, 5, 10, 20, 40, 80, 160},
		}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "classifier_in_flight",
			Help: "Classify calls currently holding a model slot.",
		}),
		attempts: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "classifier_llm_attempts",
			Help:    "Model calls per classify request.",
			Buckets: []float64{1, 2, 3, 4},
		}),
		promptTok: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "classifier_prompt_tokens_total",
			Help: "Prompt tokens sent to the model.",
		}),
		complTok: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "classifier_completion_tokens_total",
			Help: "Completion tokens produced by the model.",
		}),
		tokensPerS: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "classifier_tokens_per_second",
			Help:    "Completion tokens per second of successful requests.",
			Buckets: []float64{1, 2, 5, 10, 20, 40, 80},
		}),
		modelInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "classifier_model_info",
			Help: "Configured model, always 1.",
		}, []string{"model"}),
	}

	o.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		o.requests, o.duration, o.inFlight, o.attempts,
		o.promptTok, o.complTok, o.tokensPerS, o.modelInfo,
	)

	for _, outcome := range []usecase.Outcome{
		usecase.OutcomeOK, usecase.OutcomeInvalidOutput, usecase.OutcomeBackendError,
		usecase.OutcomeTimeout, usecase.OutcomeBusy, usecase.OutcomeBadInput,
	} {
		o.requests.WithLabelValues(string(outcome))
	}

	return o
}

func (o *Observer) SetModel(name string) {
	o.modelInfo.Reset()
	o.modelInfo.WithLabelValues(name).Set(1)
}

func (o *Observer) Handler() http.Handler {
	return promhttp.HandlerFor(o.registry, promhttp.HandlerOpts{})
}

func (o *Observer) Observe(e usecase.Event) {
	o.requests.WithLabelValues(string(e.Outcome)).Inc()
	o.duration.Observe(e.Duration.Seconds())

	if e.Attempts > 0 {
		o.attempts.Observe(float64(e.Attempts))
	}

	o.promptTok.Add(float64(e.PromptTokens))
	o.complTok.Add(float64(e.CompletionTokens))

	if e.Outcome == usecase.OutcomeOK && e.CompletionTokens > 0 && e.Duration > 0 {
		o.tokensPerS.Observe(float64(e.CompletionTokens) / e.Duration.Seconds())
	}
}

func (o *Observer) InFlight(delta int) {
	o.inFlight.Add(float64(delta))
}
