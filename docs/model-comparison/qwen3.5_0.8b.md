# Evaluation: qwen3.5_0.8b

## Headline Metrics

| Metric | Value |
|---|---|
| Total samples | 40 |
| Valid responses | 40 (100.0%) |
| Category accuracy | 82.5% |
| Severity exact accuracy | 40.0% |
| Severity within-one accuracy | 80.0% |
| Latency (mean) | 17.98s |
| Latency (p50) | 17.81s |
| Latency (p95) | 21.72s |
| Latency (max) | 25.20s |

## Category Recall

| Category | Recall |
|---|---|
| auth_error | 75.0% |
| config_error | 100.0% |
| database_error | 100.0% |
| dependency_error | 100.0% |
| filesystem_error | 100.0% |
| network_error | 100.0% |
| resource_exhausted | 25.0% |
| runtime_error | 100.0% |
| syntax_error | 100.0% |
| timeout | 66.7% |
| unknown | 33.3% |

## Mismatches

| ID | Expected | Got |
|---|---|---|
| syntax-go-paren | syntax_error / medium | syntax_error / low |
| syntax-rust-type | syntax_error / medium | syntax_error / low |
| runtime-java-npe | runtime_error / high | runtime_error / medium |
| runtime-python-trace | runtime_error / medium | runtime_error / high |
| network-tls-badcert | network_error / medium | network_error / high |
| timeout-sql-query | timeout / medium | database_error / medium |
| database-postgres-deadlock | database_error / high | database_error / critical |
| database-pool-exhausted | database_error / critical | database_error / medium |
| database-migration-failed | database_error / high | database_error / medium |
| auth-jwt-expired | auth_error / low | runtime_error / high |
| auth-postgres-password | auth_error / critical | auth_error / high |
| auth-k8s-forbidden | auth_error / medium | auth_error / critical |
| auth-github-token | auth_error / medium | auth_error / critical |
| config-env-missing | config_error / high | config_error / medium |
| config-port-invalid | config_error / high | config_error / medium |
| config-flag-unknown | config_error / low | config_error / medium |
| dependency-npm-missing | dependency_error / high | dependency_error / medium |
| resource-oom-killer | resource_exhausted / critical | runtime_error / high |
| resource-disk-enospc | resource_exhausted / critical | filesystem_error / low |
| resource-open-files | resource_exhausted / high | resource_exhausted / medium |
| resource-rate-limited | resource_exhausted / medium | network_error / high |
| filesystem-enoent | filesystem_error / high | filesystem_error / low |
| filesystem-eacces | filesystem_error / medium | filesystem_error / low |
| filesystem-readonly | filesystem_error / high | filesystem_error / low |
| unknown-random-text | unknown / low | runtime_error / low |
| unknown-info-startup | unknown / low | runtime_error / low |
| unknown-prompt-injection | unknown / low | unknown / critical |
