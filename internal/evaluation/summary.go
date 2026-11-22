package evaluation

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/entity"
)

type Mismatch struct {
	ID               string          `json:"id"`
	ExpectedCategory entity.Category `json:"expected_category"`
	ExpectedSeverity entity.Severity `json:"expected_severity"`
	GotCategory      entity.Category `json:"got_category,omitempty"`
	GotSeverity      entity.Severity `json:"got_severity,omitempty"`
	Expected         string          `json:"expected"`
	Got              string          `json:"got"`
	Error            string          `json:"error,omitempty"`
}

type Summary struct {
	Total                     int                         `json:"total"`
	Valid                     int                         `json:"valid"`
	Failed                    int                         `json:"failed"`
	ValidRate                 float64                     `json:"valid_rate"`
	CategoryAccuracy          float64                     `json:"category_accuracy"`
	SeverityExactAccuracy     float64                     `json:"severity_exact_accuracy"`
	SeverityWithinOneAccuracy float64                     `json:"severity_within_one_accuracy"`
	LatencyMean               time.Duration               `json:"latency_mean"`
	LatencyP50                time.Duration               `json:"latency_p50"`
	LatencyP95                time.Duration               `json:"latency_p95"`
	LatencyMax                time.Duration               `json:"latency_max"`
	PerCategoryRecall         map[entity.Category]float64 `json:"per_category_recall"`
	Mismatches                []Mismatch                  `json:"mismatches"`
}

func severityLevel(s entity.Severity) int {
	switch s {
	case entity.SeverityLow:
		return 0
	case entity.SeverityMedium:
		return 1
	case entity.SeverityHigh:
		return 2
	case entity.SeverityCritical:
		return 3
	default:
		return -1
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}

	idx := int(math.Ceil(float64(len(sorted))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}

	return sorted[idx]
}

// Summarize aggregates metrics from evaluation results.
func Summarize(results []Result) Summary {
	total := len(results)
	if total == 0 {
		return Summary{
			PerCategoryRecall: make(map[entity.Category]float64),
			Mismatches:        []Mismatch{},
		}
	}

	var (
		validCount            int
		categoryCorrectCount  int
		severityExactCount    int
		severityWithinCount   int
		successfulLatencies   []time.Duration
		mismatches            []Mismatch
		categoryTotalCounts   = make(map[entity.Category]int)
		categoryCorrectCounts = make(map[entity.Category]int)
	)

	for _, r := range results {
		categoryTotalCounts[r.Sample.Category]++

		isValid := r.Outcome.Err == nil &&
			r.Outcome.StatusCode == http.StatusOK &&
			r.Outcome.Classification.Validate() == nil

		expectedStr := fmt.Sprintf("%s / %s", r.Sample.Category, r.Sample.Severity)

		if !isValid {
			var gotStr string
			var errStr string
			switch {
			case r.Outcome.Err != nil:
				errStr = r.Outcome.Err.Error()
				gotStr = fmt.Sprintf("error: %v", r.Outcome.Err)
			case r.Outcome.StatusCode != http.StatusOK:
				errStr = fmt.Sprintf("http status %d", r.Outcome.StatusCode)
				gotStr = errStr
			default:
				valErr := r.Outcome.Classification.Validate()
				errStr = valErr.Error()
				gotStr = fmt.Sprintf("invalid: %v", valErr)
			}

			mismatches = append(mismatches, Mismatch{
				ID:               r.Sample.ID,
				ExpectedCategory: r.Sample.Category,
				ExpectedSeverity: r.Sample.Severity,
				GotCategory:      r.Outcome.Classification.Category,
				GotSeverity:      r.Outcome.Classification.Severity,
				Expected:         expectedStr,
				Got:              gotStr,
				Error:            errStr,
			})

			continue
		}

		validCount++
		successfulLatencies = append(successfulLatencies, r.Outcome.Latency)

		catMatch := r.Outcome.Classification.Category == r.Sample.Category
		if catMatch {
			categoryCorrectCount++
			categoryCorrectCounts[r.Sample.Category]++
		}

		sevMatch := r.Outcome.Classification.Severity == r.Sample.Severity
		if sevMatch {
			severityExactCount++
		}

		expLevel := severityLevel(r.Sample.Severity)
		gotLevel := severityLevel(r.Outcome.Classification.Severity)
		diff := expLevel - gotLevel
		if diff < 0 {
			diff = -diff
		}
		if diff <= 1 {
			severityWithinCount++
		}

		if !catMatch || !sevMatch {
			gotStr := fmt.Sprintf("%s / %s", r.Outcome.Classification.Category, r.Outcome.Classification.Severity)
			mismatches = append(mismatches, Mismatch{
				ID:               r.Sample.ID,
				ExpectedCategory: r.Sample.Category,
				ExpectedSeverity: r.Sample.Severity,
				GotCategory:      r.Outcome.Classification.Category,
				GotSeverity:      r.Outcome.Classification.Severity,
				Expected:         expectedStr,
				Got:              gotStr,
			})
		}
	}

	var meanLatency, p50Latency, p95Latency, maxLatency time.Duration
	if len(successfulLatencies) > 0 {
		slices.Sort(successfulLatencies)
		var sum time.Duration
		for _, l := range successfulLatencies {
			sum += l
		}
		meanLatency = sum / time.Duration(len(successfulLatencies))
		p50Latency = percentile(successfulLatencies, 0.50)
		p95Latency = percentile(successfulLatencies, 0.95)
		maxLatency = successfulLatencies[len(successfulLatencies)-1]
	}

	perCategoryRecall := make(map[entity.Category]float64, len(categoryTotalCounts))
	for cat, catTotal := range categoryTotalCounts {
		if catTotal > 0 {
			perCategoryRecall[cat] = float64(categoryCorrectCounts[cat]) / float64(catTotal)
		} else {
			perCategoryRecall[cat] = 0.0
		}
	}

	if mismatches == nil {
		mismatches = []Mismatch{}
	}

	return Summary{
		Total:                     total,
		Valid:                     validCount,
		Failed:                    total - validCount,
		ValidRate:                 float64(validCount) / float64(total),
		CategoryAccuracy:          float64(categoryCorrectCount) / float64(total),
		SeverityExactAccuracy:     float64(severityExactCount) / float64(total),
		SeverityWithinOneAccuracy: float64(severityWithinCount) / float64(total),
		LatencyMean:               meanLatency,
		LatencyP50:                p50Latency,
		LatencyP95:                p95Latency,
		LatencyMax:                maxLatency,
		PerCategoryRecall:         perCategoryRecall,
		Mismatches:                mismatches,
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}

	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
	}

	return fmt.Sprintf("%.2fs", d.Seconds())
}

// Markdown formats the summary into a readable report with headline and mismatches tables.
func (s Summary) Markdown(model string) string {
	var b strings.Builder

	if model != "" {
		fmt.Fprintf(&b, "# Evaluation: %s\n\n", model)
	} else {
		b.WriteString("# Evaluation Summary\n\n")
	}

	b.WriteString("## Headline Metrics\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("|---|---|\n")
	fmt.Fprintf(&b, "| Total samples | %d |\n", s.Total)
	fmt.Fprintf(&b, "| Valid responses | %d (%.1f%%) |\n", s.Valid, s.ValidRate*100)
	fmt.Fprintf(&b, "| Category accuracy | %.1f%% |\n", s.CategoryAccuracy*100)
	fmt.Fprintf(&b, "| Severity exact accuracy | %.1f%% |\n", s.SeverityExactAccuracy*100)
	fmt.Fprintf(&b, "| Severity within-one accuracy | %.1f%% |\n", s.SeverityWithinOneAccuracy*100)
	fmt.Fprintf(&b, "| Latency (mean) | %s |\n", formatDuration(s.LatencyMean))
	fmt.Fprintf(&b, "| Latency (p50) | %s |\n", formatDuration(s.LatencyP50))
	fmt.Fprintf(&b, "| Latency (p95) | %s |\n", formatDuration(s.LatencyP95))
	fmt.Fprintf(&b, "| Latency (max) | %s |\n\n", formatDuration(s.LatencyMax))

	if len(s.PerCategoryRecall) > 0 {
		b.WriteString("## Category Recall\n\n")
		b.WriteString("| Category | Recall |\n")
		b.WriteString("|---|---|\n")

		cats := make([]string, 0, len(s.PerCategoryRecall))
		for c := range s.PerCategoryRecall {
			cats = append(cats, string(c))
		}
		sort.Strings(cats)

		for _, cat := range cats {
			rec := s.PerCategoryRecall[entity.Category(cat)]
			fmt.Fprintf(&b, "| %s | %.1f%% |\n", cat, rec*100)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Mismatches\n\n")
	if len(s.Mismatches) == 0 {
		b.WriteString("No mismatches found.\n")
	} else {
		b.WriteString("| ID | Expected | Got |\n")
		b.WriteString("|---|---|---|\n")
		for _, m := range s.Mismatches {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", m.ID, m.Expected, m.Got)
		}
	}

	return b.String()
}
