# Evaluation: phi4-mini

## Headline Metrics

| Metric | Value |
|---|---|
| Total samples | 40 |
| Valid responses | 40 (100.0%) |
| Category accuracy | 90.0% |
| Severity exact accuracy | 42.5% |
| Severity within-one accuracy | 95.0% |
| Latency (mean) | 7.96s |
| Latency (p50) | 7.01s |
| Latency (p95) | 13.21s |
| Latency (max) | 14.54s |

## Category Recall

| Category | Recall |
|---|---|
| auth_error | 100.0% |
| config_error | 100.0% |
| database_error | 50.0% |
| dependency_error | 100.0% |
| filesystem_error | 100.0% |
| network_error | 100.0% |
| resource_exhausted | 100.0% |
| runtime_error | 100.0% |
| syntax_error | 100.0% |
| timeout | 33.3% |
| unknown | 100.0% |

## Mismatches

| ID | Expected | Got |
|---|---|---|
| syntax-py-indent | syntax_error / low | syntax_error / high |
| syntax-go-paren | syntax_error / medium | syntax_error / high |
| syntax-rust-type | syntax_error / medium | syntax_error / high |
| runtime-js-typeerror | runtime_error / medium | runtime_error / high |
| runtime-go-panic | runtime_error / high | runtime_error / critical |
| runtime-python-trace | runtime_error / medium | runtime_error / high |
| network-tls-badcert | network_error / medium | network_error / high |
| timeout-sql-query | timeout / medium | database_error / high |
| timeout-redis-lock | timeout / medium | resource_exhausted / high |
| database-mysql-duplicate | database_error / medium | database_error / high |
| database-pool-exhausted | database_error / critical | resource_exhausted / high |
| database-migration-failed | database_error / high | config_error / medium |
| auth-jwt-expired | auth_error / low | auth_error / high |
| auth-postgres-password | auth_error / critical | auth_error / high |
| auth-k8s-forbidden | auth_error / medium | auth_error / high |
| auth-github-token | auth_error / medium | auth_error / high |
| config-flag-unknown | config_error / low | config_error / medium |
| dependency-pip-conflict | dependency_error / medium | dependency_error / high |
| dependency-cargo-missing | dependency_error / medium | dependency_error / high |
| dependency-go-sum | dependency_error / medium | dependency_error / high |
| resource-oom-killer | resource_exhausted / critical | resource_exhausted / high |
| resource-disk-enospc | resource_exhausted / critical | resource_exhausted / high |
| resource-rate-limited | resource_exhausted / medium | resource_exhausted / high |
