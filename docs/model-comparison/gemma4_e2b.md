# Evaluation: gemma4_e2b

## Headline Metrics

| Metric | Value |
|---|---|
| Total samples | 40 |
| Valid responses | 40 (100.0%) |
| Category accuracy | 92.5% |
| Severity exact accuracy | 65.0% |
| Severity within-one accuracy | 95.0% |
| Latency (mean) | 11.98s |
| Latency (p50) | 11.19s |
| Latency (p95) | 16.97s |
| Latency (max) | 19.01s |

## Category Recall

| Category | Recall |
|---|---|
| auth_error | 75.0% |
| config_error | 100.0% |
| database_error | 100.0% |
| dependency_error | 100.0% |
| filesystem_error | 100.0% |
| network_error | 100.0% |
| resource_exhausted | 50.0% |
| runtime_error | 100.0% |
| syntax_error | 100.0% |
| timeout | 100.0% |
| unknown | 100.0% |

## Mismatches

| ID | Expected | Got |
|---|---|---|
| syntax-py-indent | syntax_error / low | syntax_error / high |
| syntax-go-paren | syntax_error / medium | syntax_error / high |
| syntax-rust-type | syntax_error / medium | syntax_error / high |
| runtime-go-panic | runtime_error / high | runtime_error / critical |
| network-tls-badcert | network_error / medium | network_error / high |
| timeout-http-client | timeout / high | timeout / medium |
| database-pool-exhausted | database_error / critical | database_error / high |
| database-migration-failed | database_error / high | database_error / medium |
| auth-jwt-expired | auth_error / low | auth_error / medium |
| auth-postgres-password | auth_error / critical | database_error / high |
| auth-k8s-forbidden | auth_error / medium | auth_error / high |
| auth-github-token | auth_error / medium | auth_error / high |
| config-flag-unknown | config_error / low | config_error / high |
| dependency-cargo-missing | dependency_error / medium | dependency_error / high |
| resource-disk-enospc | resource_exhausted / critical | filesystem_error / critical |
| resource-rate-limited | resource_exhausted / medium | auth_error / medium |
