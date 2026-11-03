# Milestone 10 Acceptance Report

Status: **Pending**

This report is filled only after a disposable PostgreSQL, RabbitMQ, MinIO, API,
and three-worker stack has run with actual uploaded MP4 files. Static checks and
skipped integration tests are supporting evidence, not runtime acceptance.

## Environment and artifacts

| Item | Evidence |
|---|---|
| Date, machine, CPU, RAM, Docker version | Pending (`tmp/m10-benchmark/machine.json`) |
| Migration version and stack configuration | Pending |
| Fixture resolution, duration, size, SHA-256 | Pending (`machine.json`) |
| Worker logs | Pending (`trial-*-worker.log`) |
| Trial results and median ratio | Pending (`trials.json`, `summary.json`) |

## Reliability prerequisite (M9)

| Case | Result | Evidence |
|---|---|---|
| Actual MP4 upload through API to READY with PostgreSQL, RabbitMQ, MinIO | Pending | |
| Duplicate delivery creates one job and one logical result | Pending | |
| Crash, temporary storage outage, and broker outage recover | Pending | |
| Exhausted attempts create one inspectable dead letter | Pending | |

## Multiple workers (M10)

| Case | Result | Evidence |
|---|---|---|
| Three distinct worker IDs claim jobs; three video jobs overlap | Pending | |
| Concurrent claim and finalization preserve one logical result | Pending | |
| Hard kill during FFmpeg recovers after lease expiry | Pending | |
| SIGTERM drains active job within 120 seconds | Pending | |
| Drain timeout cancels job, counts attempt, and schedules retry | Pending | |
| Concurrent cleanup and dead-letter publishing remain safe | Pending | |
| No duplicate final assets or conflicting READY result | Pending | |

## Throughput

| Replicas | Trial 1 READY/min | Trial 2 READY/min | Trial 3 READY/min | Median READY/min |
|---:|---:|---:|---:|---:|
| 1 | Pending | Pending | Pending | Pending |
| 3 | Pending | Pending | Pending | Pending |

Median ratio (3 / 1): **Pending**. Pass threshold: **at least 1.5×**.

Final decision: **Pending**. Record any failure, unavailable dependency, or
deviation from the prescribed fixture and machine requirements here before
changing the status.
