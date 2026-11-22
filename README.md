# LLM error classifier

A small self-hosted service that turns a raw software error message or stack trace into structured JSON:

```bash
curl -s localhost:8080/classify -H 'Content-Type: application/json' \
  -d '{"error": "TypeError: Cannot read properties of undefined (reading '"'"'map'"'"')\n    at renderProducts (app.js:42)"}'
```

```json
{
  "category": "runtime_error",
  "severity": "medium",
  "summary": "renderProducts tries to call map on undefined, causing a runtime error.",
  "suggested_action": "Fix the undefined value before calling map on it in renderProducts."
}
```

An open-weights model runs **locally on CPU** in Ollama; nothing leaves the machine and no external AI API is used. The whole stack starts with `docker compose up -d` and is ready for a plain Linux VPS without a GPU.

- Go service (stdlib + Prometheus client), 19 MB image, ports-and-adapters layout
- Default model: **IBM Granite 4.2 3B** (Apache 2.0), chosen by a measured comparison of six models (below)
- The model is a setting: change `LLM_MODEL` in `.env`, run `docker compose up -d`, no rebuild
- Prometheus metrics and a Grafana dashboard (optional profile), concurrency limit, timeouts, retries on invalid model output
- 40-sample labelled evaluation set and a tool that scores any model

## Quick start

Requirements: Docker with Compose v2, 4 CPU cores, 8 GB RAM for the default limits (see [Resources](#resources)).

```bash
cp .env.example .env              # defaults work as is
docker compose up -d --build      # first start downloads the model (~2.2 GB)
docker compose logs -f model-init # watch the download
curl -s localhost:8080/readyz     # {"status":"ready"} once the model is present

# with Prometheus and Grafana
docker compose --profile observability up -d
# Grafana: http://localhost:3000 (dashboard "LLM error classifier"), Prometheus: http://localhost:9090
```

The first request after a start loads the model into RAM (about 35 s with the default model); later requests take about 7 s on a quiet 4-core machine.

## API

| Method and path | Purpose |
|---|---|
| `POST /classify` | Body `{"error": "<text>"}` (`application/json`) or the raw text with `Content-Type: text/plain`. Returns the four fields below. |
| `GET /healthz` | Liveness, always 200 while the process is up. |
| `GET /readyz` | 200 when the model backend answers and the configured model is present, otherwise 503. |
| `GET /metrics` | Prometheus metrics. |

Response fields: `category`, `severity`, `summary`, `suggested_action`.

Categories (`internal/entity/classification.go`): `syntax_error`, `runtime_error`, `network_error`, `timeout`, `database_error`, `auth_error`, `config_error`, `dependency_error`, `resource_exhausted`, `filesystem_error`, `unknown`.
Severities: `low` (warning or cosmetic), `medium` (one feature fails, workaround exists), `high` (important functionality down), `critical` (whole service down, data loss, security breach). The dictionaries and their definitions are inserted into the prompt and into the JSON schema from one place, so they cannot drift apart.

Errors are JSON `{"error": "<fixed message>", "code": "<code>"}`:

| Status | code | When |
|---|---|---|
| 400 | `empty_input`, `bad_request` | empty text, malformed JSON |
| 413 / 415 | `bad_request` | body over 256 KiB, unsupported content type |
| 429 | `busy` (with `Retry-After`) | no free model slot within `QUEUE_WAIT` |
| 502 | `invalid_model_output`, `model_unavailable` | the model kept returning an unusable answer, or the backend is down |
| 504 | `model_timeout` | the model did not answer within `LLM_TIMEOUT` |

Backend details never reach the client; they are logged at debug level only. The text of the error itself is not logged.

## Architecture

```
client ──HTTP──> classifier (Go) ──OpenAI-compatible API──> Ollama (CPU) ── volume ollama-models
                    │                                          ▲
                    ├─ /metrics ──> Prometheus ──> Grafana     └─ model-init: `ollama pull $LLM_MODEL`
                    └─ /healthz, /readyz
```

```
cmd/classifier         entry point (+ -healthcheck for Docker)
cmd/evaluate           evaluation tool
internal/entity        domain: categories, severities, Classification, errors (imports nothing)
internal/usecase       Classifier: input limits, concurrency slots, prompt, retry, validation; ports LLM and Observer
internal/adapter/openai   LLM port over /v1/chat/completions and /v1/models (Ollama, llama.cpp server, vLLM)
internal/adapter/metrics  Observer port as Prometheus metrics
internal/controller/httpctl  HTTP handlers and error mapping
internal/app, internal/config  composition root and environment settings
internal/evaluation    dataset loader, runner, scoring
```

`depguard` enforces that `usecase` and `entity` never import adapters or controllers.

How a request flows:

1. Empty input is rejected; very long input is cut to `MAX_INPUT_CHARS` keeping the head **and** the tail (stack traces carry the cause at both ends).
2. A semaphore of `MAX_CONCURRENT` slots protects the model. A call waits at most `QUEUE_WAIT` for a slot, then gets 429. CPU inference does not get faster with parallel calls, so the default is 1.
3. The prompt contains the dictionaries with definitions, two examples, and the instruction that the user text is untrusted data (a prompt-injection sample is part of the evaluation set). The model is called with `temperature 0`, a **JSON schema** in `response_format` (the schema enumerates the allowed categories and severities, so the output is constrained at decoding time) and `reasoning_effort: none` so reasoning models do not spend tokens on thinking.
4. The reply is parsed tolerantly (code fences and prose around the JSON are stripped, case and spacing are normalised, text fields are capped) and then validated against the dictionaries.
5. If the answer is still invalid, the call is repeated (`MAX_ATTEMPTS`, default 2, temperature 0.3 on retries) and then fails with 502. Backend errors and timeouts are **not** retried: a slow CPU model is better failed fast than queued twice.

## Model choice

Open-weights models of 0.8–4B parameters were compared on the same 40-sample set, on the same machine, with the Ollama container limited to **4 CPUs and 8 GB RAM** (the size of a typical entry-level VPS), one request at a time, Q4 quantisation as shipped by Ollama.

| Model | License | Disk | Peak RAM | Category acc. | Severity exact | Severity ±1 | Valid JSON | Latency mean / p95 |
|---|---|---|---|---|---|---|---|---|
| qwen3.5:0.8b | Apache 2.0 | 1.3 GB | 2.3 GiB | 82.5 % | 40.0 % | 80.0 % | 100 % | 18.0 s / 21.7 s |
| qwen3.5:2b | Apache 2.0 | 2.7 GB | 3.4 GiB | 90.0 % | 45.0 % | 90.0 % | 97.5 % | 13.8 s / 29.2 s |
| qwen3.5:4b | Apache 2.0 | 3.3 GB | 4.3 GiB | **97.5 %** | 45.0 % | 95.0 % | 100 % | 16.1 s / 26.8 s |
| **granite4.2:3b** | Apache 2.0 | 2.2 GB | **2.6 GiB** | 92.5 % | 57.5 % | **97.5 %** | 100 % | 11.0 s / 16.8 s |
| phi4-mini (3.8B) | MIT | 2.5 GB | 3.5 GiB | 90.0 % | 42.5 % | 95.0 % | 100 % | **8.0 s / 13.2 s** |
| gemma4:e2b | Apache 2.0 | ~7 GB | 5.2 GiB | 92.5 % | **65.0 %** | 95.0 % | 100 % | 12.0 s / 17.0 s |

Licenses were read from the Hugging Face model cards (`license:` tag) of the original repositories: `ibm-granite/granite-4.2-3b`, `Qwen/Qwen3.5-*`, `microsoft/Phi-4-mini-instruct`, `google/gemma-4-E2B-it`. Llama 3.2 was left out on purpose: the Llama Community License carries extra conditions (attribution, acceptable-use policy, a 700 M user clause). The full per-model reports with per-category recall and every mismatch are in [`docs/model-comparison/`](docs/model-comparison).

Why Granite 4.2 3B is the default:

- permissive Apache 2.0 license, open weights, no usage conditions beyond the license text;
- the smallest memory footprint of the accurate models (2.6 GiB peak, fits a 4 GB VPS with Ollama alone), and a 2.2 GB download;
- 100 % valid JSON, 92.5 % category accuracy, and the best severity-within-one-level score; its latency is the second best after phi4-mini;
- IBM positions the model for enterprise tasks with structured output, which is what this service needs.

Alternatives by priority: **qwen3.5:4b** if category accuracy matters most (97.5 %, about 1.3 GiB more RAM and slower), **phi4-mini** if speed matters most (MIT, fastest), **gemma4:e2b** for the best severity calibration if 8 GB of RAM and 7 GB of disk are available. Switching is one line in `.env`.

Caveats of this comparison: 40 samples means one sample is 2.5 points, so differences of a few points are noise. Severity is subjective, so "exact" accuracy is low for every model and "±1 level" is the more honest score. The labels were written by the author. Timings come from Docker Desktop on a Mac (the Linux VM there is slower than bare metal and the machine was not idle), so absolute numbers on a real VPS will differ (the default model was re-measured on a Linux host, see [the demonstration](#demonstration-on-a-clean-linux-host): mean 8.9 s instead of 11-13 s), but the ranking and the RAM figures are what to rely on.

Reproduce: `scripts/compare.sh "granite4.2:3b phi4-mini" <ollama-container>` runs the evaluation per model and records `docker stats`; `scripts/stats.sh <file>` summarises it.

## Resources

Measured on the default model (Granite 4.2 3B, Q4), Ollama limited to 4 CPUs / 8 GB:

| | |
|---|---|
| **Disk** | model 2.2 GB (volume `ollama-models`) + Ollama image 4.3 GB + classifier image 19 MB. Plan for **10 GB** including the OS layer cache and a second model for upgrades |
| **RAM** | idle Ollama ~90 MB; with the model loaded **2.6 GiB peak** under load; classifier process ~20 MB. Plan for **4 GB** minimum, 8 GB for comfort or larger models |
| **CPU** | inference uses all allotted cores: ~400 % of 4 cores while a request runs, idle otherwise. 2 cores work but roughly double the latency |
| **Latency** | first request after start (model load) 18–35 s; warm request **~7 s**, mean 9 s (Linux) to 11–13 s (Mac, Docker Desktop) over the 40-sample set (stack traces are slower: prompt processing grows with input length); p95 15–19 s |
| **Throughput** | one request at a time, about 5–8 requests per minute. Parallel requests do not help on CPU; they wait for a slot (`QUEUE_WAIT`) and then get 429 |

Series of sequential requests (the 40-sample set, one by one, against the Compose stack): 40/40 valid answers, no retries needed, memory flat at 2.6 GiB, CPU pinned at the 4-core limit. A burst of 6–8 simultaneous requests with the default single slot: one or two finish, the rest wait 30 s and are answered with `429 busy`, the process stays healthy and the metric `classifier_requests_total{outcome="busy"}` counts them (a run with two slots is visible as latency growth to ~50 s instead).

### Limitations

- Tiny models make mistakes: about 7 % of the evaluation set got a wrong category (typically the boundary cases: connection pool exhaustion read as a timeout, rate limiting read as a runtime error, an unknown CLI flag read as unknown). Severity is a judgement call even for people.
- Latency of several seconds per request is the price of CPU-only inference; this is a triage helper for logs and alerts, not an interactive low-latency API.
- The prompt is English-first; `CLASSIFY_LANGUAGE` switches the language of the two text fields, but classification quality for non-English stack traces was only spot-checked (3 samples).
- Input is limited by characters, not tokens (`MAX_INPUT_CHARS`, default 4000); dense CJK text can still exceed the model context. The default Ollama context is 4096 tokens.
- `tokens_per_second` in the dashboard includes time spent waiting for a slot, so it understates raw model speed under load.
- No authentication: bind the port to localhost (the default) or put a reverse proxy in front.

## Configuration

All settings are environment variables (see [`.env.example`](.env.example)); no secrets exist in this project.

| Variable | Default | Meaning |
|---|---|---|
| `LLM_MODEL` | `granite4.2:3b` (in `.env.example`; required) | Ollama model tag, pulled automatically by `model-init` |
| `LLM_BASE_URL` | `http://ollama:11434/v1` | any OpenAI-compatible backend |
| `LLM_API_KEY` | empty | bearer token for backends that need one |
| `LLM_REASONING_EFFORT` | `none` | passed through; stops reasoning models from thinking |
| `LLM_MAX_TOKENS`, `LLM_TIMEOUT` | `400`, `120s` | per model call |
| `MAX_CONCURRENT`, `QUEUE_WAIT` | `1`, `30s` | slots in front of the model and how long to wait for one |
| `MAX_ATTEMPTS`, `MAX_INPUT_CHARS` | `2`, `4000` | retries on invalid output; input cut length |
| `CLASSIFY_LANGUAGE` | `English` | language of `summary` and `suggested_action` |
| `OLLAMA_CPUS`, `OLLAMA_MEMORY` | `4`, `8g` | container limits |
| `OLLAMA_NUM_PARALLEL` | `1` | parallel requests inside Ollama; keep equal to `MAX_CONCURRENT` |
| `*_PORT` | 8080, 11434, 9090, 3000 | host ports, all bound to 127.0.0.1 |
| `BUILD_PROXY`, `HTTPS_PROXY` | empty | only when the host reaches the internet through a proxy |

Updating or swapping the model without touching the application: edit `LLM_MODEL`, run `docker compose up -d`. `model-init` pulls the new model into the persistent volume (a tag that is already present is not re-downloaded, so use a new tag to upgrade), the classifier restarts with the new name. Old models stay in the volume until you remove them (`docker compose exec ollama ollama rm <tag>`; if the host needs a proxy, add `-e HTTPS_PROXY= -e HTTP_PROXY=` to the `exec`, otherwise the CLI tries to reach localhost through it). Any other OpenAI-compatible server (llama.cpp `llama-server`, vLLM) can replace Ollama by setting `LLM_BASE_URL`.

## Deploying on a clean Linux VPS

Tested layout: Ubuntu 22.04/24.04 or Debian 12, 4 vCPU, 8 GB RAM (4 GB works with smaller models), 20 GB disk, **no GPU**.

```bash
# 1. Docker Engine and the Compose plugin (official convenience script)
curl -fsSL https://get.docker.com | sudo sh
sudo systemctl enable --now docker        # Docker, and with it the stack, starts on boot

# 2. The project
git clone https://github.com/Code-oclock/llm-error-classifier.git
cd llm-error-classifier
cp .env.example .env
$EDITOR .env                              # set OLLAMA_CPUS / OLLAMA_MEMORY to what the VPS has

# 3. Start (downloads the model once into the ollama-models volume)
sudo docker compose up -d --build
sudo docker compose logs -f model-init    # wait for the download to finish
curl -s localhost:8080/readyz

# 4. Optional dashboards
sudo docker compose --profile observability up -d
```

Notes:

- **Restart after reboot.** All long-running services use `restart: unless-stopped`; with `docker` enabled in systemd the stack comes back by itself. `model-init` is a one-shot job that finds the model already in the volume (`ollama show`) and exits in a second, so a restart works even without internet access.
- **Persistence.** Models live in the named volume `ollama-models`, so `docker compose down` / `up` and image rebuilds never download them again. Do not use `down -v` unless you want to.
- **No source changes** are needed between laptop and VPS: only `.env` differs (CPU/RAM limits, ports, model).
- **Exposure.** Ports are published on 127.0.0.1. To serve the API publicly, put nginx/Caddy with TLS and authentication in front, or change the port mapping deliberately.
- **Sizing.** Set `OLLAMA_CPUS` to the number of vCPUs and `OLLAMA_MEMORY` to RAM minus ~1 GB. For 4 GB RAM pick `qwen3.5:2b` or `granite4.2:3b`; for 2 vCPU expect roughly twice the latency.
- **Updates.** `git pull && docker compose up -d --build`.
- **Registry not reachable from the server.** If `registry.ollama.ai` is blocked, download the model on another machine and copy the volume: `docker run --rm --entrypoint tar -v <project>_ollama-models:/m:ro ollama/ollama:0.40.17 cf - -C /m . | ssh server 'cat > models.tar'`, then on the server create the volume and unpack the archive into it before `docker compose up -d`. This is exactly what was done for the demo below.

## Demonstration on a clean Linux host

There is no VPS at hand, so the deployment instructions were run literally on a separate Linux machine used as a stand-in: Ubuntu 24.04, kernel 6.8, 20 cores and 15 GB RAM (no GPU), Docker 29 with Compose v5. The Ollama container was limited to **4 CPUs and 8 GB** (the defaults of this project), so the numbers describe a 4-vCPU VPS.

What was done: `git clone`, `cp .env.example .env`, `docker compose --profile observability up -d --build`, with no source or configuration changes. The registry `registry.ollama.ai` is not reachable from that host, so the model volume was copied from the laptop (see the note in the deployment section); `model-init` found the model and exited, the stack was ready in 15 seconds.

| Measured on the Linux host (granite4.2:3b) | |
|---|---|
| First request after start (model loaded from disk) | 18 s |
| Warm request | 6.7-6.9 s |
| 40-sample evaluation, sequential | 40/40 valid, category accuracy **95.0 %**, severity exact 52.5 %, within one level 97.5 % |
| Latency over the 40 samples | mean 8.9 s, p50 8.3 s, p95 15.1 s, max 16.9 s |
| Ollama container | CPU ~394 % average (limit 400 %), RAM peak **2.9 GiB**, idle ~0 % CPU |
| Burst of 8 parallel requests (1 slot, `QUEUE_WAIT` 30 s) | 5 answered (about 6 s apart, no degradation), 3 rejected with `429 busy` after exactly 30 s |

The full report is in [`docs/model-comparison/vps-granite.md`](docs/model-comparison/vps-granite.md). These numbers are cleaner than the Mac measurements above (no virtualisation layer, no other load): the Mac figures are a pessimistic bound, and the ranking of models there is what to rely on. Differences between the two runs of the same model (92.5 % vs 95.0 % category accuracy) are the noise level of a 40-sample set.

## Monitoring

`docker compose --profile observability up -d` adds Prometheus (scrapes `classifier:8080/metrics` every 15 s, 15 days retention) and Grafana with the provisioned dashboard **LLM error classifier**: success ratio, model name, requests per minute by outcome (`ok`, `invalid_output`, `backend_error`, `timeout`, `busy`, `bad_input`), latency p50/p95, in-flight calls, tokens per second, prompt/completion tokens, model calls per request (above 1 means retries), process memory. Metrics (`classifier_*`): `requests_total{outcome}`, `request_duration_seconds`, `in_flight`, `llm_attempts`, `prompt_tokens_total`, `completion_tokens_total`, `tokens_per_second`, `model_info{model}`.

Container CPU and RAM of the model itself are best read from `docker stats`, or from cAdvisor if you run it; the dashboard shows only the classifier's own process.

![Grafana dashboard](docs/grafana-dashboard.png)

The screenshot is from the Linux demo host below, during the evaluation run. [An earlier screenshot](docs/grafana-dashboard-mac-burst.png) from the Mac shows the same dashboard after two deliberate bursts of parallel requests: the rejected `busy` requests pull the success ratio down.

## Evaluation

`eval/dataset.jsonl` has 40 labelled error messages: JavaScript, Python, Go, Java, PHP, Rust, SQL, nginx, Docker/Kubernetes, git, npm, systemd, TLS, OOM; five long stack traces, three non-English samples, one prompt-injection attempt, and every category covered by at least three examples (`unknown` by three). Labels use the root cause of the failure.

```bash
make eval                       # against http://localhost:8080
go run ./cmd/evaluate -url http://localhost:8080 -dataset eval/dataset.jsonl -label my-model
```

It sends the samples one by one (after one warm-up request so model loading does not distort the latency), then writes `eval/results/<label>.md` and `.json` with valid-response rate, category accuracy, severity exact and ±1 accuracy, latency mean/p50/p95/max, per-category recall and the list of mismatches.

## Development

```bash
make check      # gofumpt/gci formatting, golangci-lint (depguard, gosec, ...), go test -race
make hooks      # run format and lint before each commit
```

CI (GitHub Actions) runs the linter and the tests on every push. The unit tests use fake backends (`httptest`); no model is needed.
