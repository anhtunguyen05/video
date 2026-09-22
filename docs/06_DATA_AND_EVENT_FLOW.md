# Data and Event Flow

## 1. Upload flow

```text
Browser
  |
  | POST /uploads
  v
Upload API
  |
  | create upload record
  | create presigned URL
  v
Browser
  |
  | PUT source file
  v
MinIO
  |
  | upload complete
  v
Browser
  |
  | POST /uploads/{id}/complete
  v
Upload API
  |
  | verify object
  | publish event
  v
video.uploaded
```

---

## 2. Processing flow

```text
video.uploaded
      |
      v
Processing Service
      |
      v
processing.requested
      |
      v
Worker
      |
      +---- ffprobe
      |
      +---- thumbnail
      |
      +---- transcode
      |
      +---- HLS package
      |
      v
processing.completed
      |
      v
Media Service
      |
      v
VIDEO = READY
```

---

## 3. Failure flow

```text
processing.requested
      |
      v
worker
      |
      X transient failure
      |
      v
retry scheduled
      |
      v
processing.requested
```

After retry limit:

```text
processing.failed
      |
      +---- persist failure
      +---- mark pipeline failed
      +---- optionally notify user
      +---- route message to DLQ
```

---

## 4. Suggested event catalog

### video.uploaded

```json
{
  "event_id": "evt_...",
  "event_type": "video.uploaded",
  "occurred_at": "ISO-8601",
  "video_id": "vid_...",
  "owner_id": "usr_...",
  "source_object_key": "videos/.../source.mp4",
  "correlation_id": "..."
}
```

### processing.started

```json
{
  "event_id": "evt_...",
  "event_type": "processing.started",
  "video_id": "vid_...",
  "job_id": "job_...",
  "attempt": 1,
  "correlation_id": "..."
}
```

### processing.completed

```json
{
  "event_id": "evt_...",
  "event_type": "processing.completed",
  "video_id": "vid_...",
  "job_id": "job_...",
  "outputs": [
    "thumbnail",
    "hls"
  ],
  "correlation_id": "..."
}
```

### processing.failed

```json
{
  "event_id": "evt_...",
  "event_type": "processing.failed",
  "video_id": "vid_...",
  "job_id": "job_...",
  "attempt": 3,
  "error_code": "FFMPEG_EXIT_NON_ZERO",
  "correlation_id": "..."
}
```

---

## 5. Idempotency strategy

Assume duplicate message delivery.

Consumer logic should behave like:

```text
receive message
    ↓
check operation identity
    ↓
already completed?
 ┌──┴──┐
yes   no
 |     |
ack   execute
       |
       v
   persist result
       |
       v
      ack
```

Potential operation identity:

```text
video_id + processing_stage + processing_version
```

Example:

```text
vid_123 + TRANSCODE_720P + v1
```

---

## 6. Retry policy

Do not retry every failure.

### Retryable examples

- temporary storage timeout
- broker/network issue
- transient external dependency failure

### Non-retryable examples

- corrupt source file
- unsupported codec
- invalid media stream
- missing required input after verification

Suggested bounded backoff:

```text
attempt 1
+ 30 seconds
attempt 2
+ 2 minutes
attempt 3
+ 10 minutes
then fail
```

Exact values can change.

---

## 7. Outbox pattern

When the system becomes distributed, consider the transactional outbox pattern.

Problem:

```text
DB commit succeeded
event publish failed
```

Outbox approach:

```text
database transaction:
- update video
- insert outbox event

background publisher:
- reads outbox
- publishes message
- marks published
```

This is a useful later-stage reliability improvement.

---

## 8. Correlation

Carry:

```text
request_id
correlation_id
event_id
video_id
job_id
```

through logs and messages.

This allows an engineer to follow one video across API, broker and worker.
