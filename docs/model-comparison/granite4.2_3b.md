# Evaluation: granite4.2_3b

## Headline Metrics

| Metric | Value |
|---|---|
| Total samples | 40 |
| Valid responses | 40 (100.0%) |
| Category accuracy | 92.5% |
| Severity exact accuracy | 57.5% |
| Severity within-one accuracy | 97.5% |
| Latency (mean) | 11.00s |
| Latency (p50) | 9.61s |
| Latency (p95) | 16.82s |
| Latency (max) | 23.42s |

## Category Recall

| Category | Recall |
|---|---|
| auth_error | 100.0% |
| config_error | 66.7% |
| database_error | 75.0% |
| dependency_error | 100.0% |
| filesystem_error | 100.0% |
| network_error | 100.0% |
| resource_exhausted | 75.0% |
| runtime_error | 100.0% |
| syntax_error | 100.0% |
| timeout | 100.0% |
| unknown | 100.0% |

## Mismatches

| ID | Expected | Got |
|---|---|---|
| syntax-py-indent | syntax_error / low | syntax_error / medium |
| runtime-java-npe | runtime_error / high | runtime_error / medium |
| network-tls-badcert | network_error / medium | network_error / high |
| timeout-http-client | timeout / high | timeout / medium |
| database-pool-exhausted | database_error / critical | timeout / medium |
| database-migration-failed | database_error / high | database_error / medium |
| auth-jwt-expired | auth_error / low | auth_error / medium |
| auth-postgres-password | auth_error / critical | auth_error / high |
| config-env-missing | config_error / high | config_error / medium |
| config-port-invalid | config_error / high | config_error / medium |
| config-flag-unknown | config_error / low | unknown / low |
| dependency-npm-missing | dependency_error / high | dependency_error / medium |
| resource-oom-killer | resource_exhausted / critical | resource_exhausted / high |
| resource-disk-enospc | resource_exhausted / critical | resource_exhausted / high |
| resource-open-files | resource_exhausted / high | resource_exhausted / medium |
| resource-rate-limited | resource_exhausted / medium | runtime_error / medium |
| filesystem-enoent | filesystem_error / high | filesystem_error / medium |
| filesystem-readonly | filesystem_error / high | filesystem_error / medium |
| unknown-prompt-injection | unknown / low | unknown / medium |
