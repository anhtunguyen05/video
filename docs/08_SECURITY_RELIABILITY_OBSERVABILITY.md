# Security, Reliability and Observability

## 1. Security

### Authorization

Every user-owned video read/write must validate ownership.

Never rely on:

```text
"the user cannot guess this UUID"
```

UUIDs are identifiers, not authorization.

---

### Upload validation

Validate:

- allowed size
- expected media type
- object presence
- upload lifecycle
- filename handling

Do not trust the original filename as a storage path.

---

### Storage paths

Use generated object keys.

Example:

```text
users/{user_id}/videos/{video_id}/source/source.mp4
```

Avoid user-controlled traversal paths.

---

### Signed URLs

Private content should use time-limited access when appropriate.

Possible strategy:

- API verifies authorization
- API returns signed object/playback URL
- client accesses storage/CDN

---

### Secrets

Never commit:

- object storage secret keys
- JWT/session secrets
- database passwords
- broker credentials

Use environment variables or a secret manager.

---

## 2. Reliability

### Idempotent consumers

Broker consumers must tolerate redelivery.

### Bounded retry

Avoid infinite retry loops.

### Dead-letter queue

Messages that exceed retry policy should be inspectable.

### Graceful shutdown

Worker receives shutdown:

```text
stop receiving new job
finish current safe unit
ack/nack correctly
close resources
exit
```

### Job lease/heartbeat

For long-running work, consider persisted job ownership or heartbeat behavior so abandoned work can be recovered.

### Cleanup

Temporary files should be cleaned after:

- success
- failure
- cancellation

Cleanup itself should be retry-safe.

---

## 3. Concurrency

Multiple workers may process different videos concurrently.

The system must prevent unsafe duplicate logical processing.

Possible approaches:

- database uniqueness constraints
- compare-and-set state transitions
- idempotency records
- Redis lock only when truly necessary

Prefer database-enforced invariants over distributed locks when possible.

---

## 4. Logging

Use structured JSON logs.

Recommended fields:

```text
timestamp
level
service
request_id
correlation_id
video_id
job_id
event_id
operation
duration_ms
error_code
```

Avoid logging media contents or secrets.

---

## 5. Metrics

Useful metrics:

### API

```text
http_requests_total
http_request_duration_seconds
http_errors_total
```

### Processing

```text
processing_jobs_total
processing_jobs_failed_total
processing_duration_seconds
processing_retries_total
ffmpeg_failures_total
```

### Queue

```text
queue_depth
consumer_count
oldest_message_age
```

### Storage

```text
upload_failures_total
storage_operation_duration_seconds
```

---

## 6. Tracing

A trace should ideally show:

```text
HTTP upload completion
    ↓
event publication
    ↓
processing consume
    ↓
ffprobe
    ↓
transcode
    ↓
storage upload
    ↓
completion event
```

OpenTelemetry should propagate trace/correlation context through asynchronous messages.

---

## 7. Health endpoints

Each service should expose:

```text
/health/live
/health/ready
```

Liveness:

```text
process is alive
```

Readiness:

```text
process can serve work
```

Do not make liveness depend on every external dependency.

---

## 8. SLO thinking

For learning purposes, define simple targets.

Examples:

```text
API availability target: 99.9% in a production-style environment
job success target: > 99% for valid supported input
no lost acknowledged jobs
no cross-user media access
```

These are engineering targets, not guarantees.
