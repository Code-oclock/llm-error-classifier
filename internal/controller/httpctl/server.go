// Package httpctl is the HTTP API of the classifier.
package httpctl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
)

const (
	maxBodyBytes = 256 << 10
	readyTimeout = 3 * time.Second
)

// Classifier is what the handler needs from the use case.
type Classifier interface {
	Classify(ctx context.Context, text string) (entity.Classification, error)
}

type server struct {
	classifier Classifier
	ready      func(context.Context) error
	log        *slog.Logger
}

// NewHandler wires the routes. ready checks the model backend; metrics
// serves /metrics.
func NewHandler(c Classifier, ready func(context.Context) error, metrics http.Handler, log *slog.Logger) http.Handler {
	s := &server{classifier: c, ready: ready, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /classify", s.classify)
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.Handle("GET /metrics", metrics)

	return mux
}

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: msg, Code: code})
}

func (s *server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()

	if err := s.ready(ctx); err != nil {
		s.log.Warn("not ready", "error", err)
		writeError(w, http.StatusServiceUnavailable, "not_ready", "model backend is not ready")

		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *server) classify(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	text, status, msg := readText(w, r)
	if status != 0 {
		writeError(w, status, "bad_request", msg)

		return
	}

	result, err := s.classifier.Classify(r.Context(), text)
	if err != nil {
		status, code := mapError(err)
		s.log.Warn("classify failed", "code", code, "chars", len(text), "duration", time.Since(start).Round(time.Millisecond))
		s.log.Debug("classify failure detail", "err", err)

		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "5")
		}

		writeError(w, status, code, publicMessages[code])

		return
	}

	s.log.Info("classified", "category", result.Category, "severity", result.Severity, "chars", len(text), "duration", time.Since(start).Round(time.Millisecond))
	writeJSON(w, http.StatusOK, result)
}

// readText extracts the error text; a non-zero status means a rejection.
func readText(w http.ResponseWriter, r *http.Request) (text string, status int, msg string) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return "", http.StatusRequestEntityTooLarge, "request body is too large"
		}

		return "", http.StatusBadRequest, "cannot read request body"
	}

	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))

	switch mediaType {
	case "text/plain":
		return string(body), 0, ""
	case "", "application/json":
	default:
		return "", http.StatusUnsupportedMediaType, "use application/json or text/plain"
	}

	var in struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &in); err != nil {
		return "", http.StatusBadRequest, `body must be JSON {"error": "<text>"} or text/plain`
	}

	text = in.Error
	if strings.TrimSpace(text) == "" {
		text = in.Message
	}

	return text, 0, ""
}

// publicMessages are the only texts clients see: backend details stay in logs.
var publicMessages = map[string]string{
	"empty_input":          "error text is empty",
	"busy":                 "too many concurrent requests, retry later",
	"model_timeout":        "the model did not answer in time",
	"invalid_model_output": "the model returned an unusable answer",
	"model_unavailable":    "the model backend is unavailable",
	"internal":             "internal error",
}

func mapError(err error) (status int, code string) {
	switch {
	case errors.Is(err, entity.ErrEmptyInput):
		return http.StatusBadRequest, "empty_input"
	case errors.Is(err, entity.ErrBusy):
		return http.StatusTooManyRequests, "busy"
	case errors.Is(err, entity.ErrModelTimeout):
		return http.StatusGatewayTimeout, "model_timeout"
	case errors.Is(err, entity.ErrInvalidModelOutput):
		return http.StatusBadGateway, "invalid_model_output"
	case errors.Is(err, entity.ErrModelUnavailable):
		return http.StatusBadGateway, "model_unavailable"
	default:
		return http.StatusInternalServerError, "internal"
	}
}
