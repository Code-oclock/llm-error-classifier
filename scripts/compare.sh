#!/bin/sh
# Compare models: run the evaluation per model against a running Ollama
# container and record its CPU and RAM with `docker stats`.
#
# usage: scripts/compare.sh "qwen3.5:2b phi4-mini" [ollama-container]
set -eu

MODELS=${1:?usage: compare.sh "model1 model2" [container]}
CONTAINER=${2:-ollama}
BACKEND=${LLM_BASE_URL:-http://localhost:11434/v1}
PORT=${COMPARE_PORT:-18080}
OUT=eval/results
mkdir -p "$OUT" bin
go build -o bin/classifier ./cmd/classifier
go build -o bin/evaluate ./cmd/evaluate

for model in $MODELS; do
	label=$(echo "$model" | tr '/: ' '___')
	echo "== $model"
	LLM_MODEL=$model LLM_BASE_URL=$BACKEND LISTEN_ADDR=:$PORT MAX_CONCURRENT=1 LOG_LEVEL=warn bin/classifier >"$OUT/$label.server.log" 2>&1 &
	server=$!
	until curl -sf "http://localhost:$PORT/readyz" >/dev/null; do sleep 1; done

	(while :; do docker stats --no-stream --format '{{.CPUPerc}},{{.MemUsage}}' "$CONTAINER" >>"$OUT/$label.stats.csv"; sleep 2; done) &
	sampler=$!
	: >"$OUT/$label.stats.csv"

	bin/evaluate -url "http://localhost:$PORT" -dataset eval/dataset.jsonl -label "$label" -out "$OUT" >/dev/null

	kill "$sampler" "$server" 2>/dev/null || true
	wait 2>/dev/null || true
	echo "   report: $OUT/$label.md  docker stats: $OUT/$label.stats.csv"
done
