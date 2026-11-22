// Package main implements the CLI evaluation tool.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Code-oclock/llm-error-classifier/internal/evaluation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	urlFlag := flag.String("url", "http://localhost:8080", "base URL of the classifier service")
	datasetFlag := flag.String("dataset", "", "path to dataset JSONL file")
	concurrencyFlag := flag.Int("concurrency", 1, "number of concurrent requests")
	timeoutFlag := flag.Duration("timeout", 180*time.Second, "per-request timeout")
	labelFlag := flag.String("label", "", "name used in output, e.g. the model")
	outFlag := flag.String("out", "eval/results", "output directory for reports")
	warmupFlag := flag.Int("warmup", 1, "number of warmup requests before measuring")

	flag.Parse()

	if *datasetFlag == "" {
		return errors.New("-dataset is required")
	}

	label := *labelFlag
	if label == "" {
		label = "evaluation"
	}

	label = strings.NewReplacer("/", "_", ":", "-", " ", "_").Replace(label)

	f, err := os.Open(*datasetFlag)
	if err != nil {
		return fmt.Errorf("opening dataset: %w", err)
	}
	defer f.Close()

	samples, err := evaluation.Load(f)
	if err != nil {
		return fmt.Errorf("loading dataset: %w", err)
	}

	if len(samples) == 0 {
		return errors.New("dataset contains no samples")
	}

	client := evaluation.NewClient(*urlFlag, *timeoutFlag)
	ctx := context.Background()

	for i := 0; i < *warmupFlag && len(samples) > 0; i++ {
		_ = client.Classify(ctx, samples[0])
	}

	results := evaluation.Run(ctx, samples, client.Classify, *concurrencyFlag)
	summary := evaluation.Summarize(results)

	if err := os.MkdirAll(*outFlag, 0o750); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	jsonData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling summary to JSON: %w", err)
	}

	jsonPath := filepath.Join(*outFlag, label+".json")
	if err := os.WriteFile(jsonPath, jsonData, 0o600); err != nil {
		return fmt.Errorf("writing JSON report: %w", err)
	}

	mdReport := summary.Markdown(label)
	mdPath := filepath.Join(*outFlag, label+".md")
	if err := os.WriteFile(mdPath, []byte(mdReport), 0o600); err != nil {
		return fmt.Errorf("writing Markdown report: %w", err)
	}

	fmt.Print(mdReport)

	return nil
}
