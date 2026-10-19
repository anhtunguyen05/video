# Events and Jobs

RabbitMQ dùng cho asynchronous processing.

Phân biệt:
```text
event = chuyện đã xảy ra
command/job = việc cần thực hiện
```

## 1. Message envelope

```json
{
  "message_id": "msg_123",
  "message_type": "video.uploaded.v1",
  "schema_version": 1,
  "occurred_at": "2026-09-23T02:00:00Z",
  "correlation_id": "cor_123",
  "causation_id": "req_123",
  "payload": {}
}
```

## 2. video.uploaded.v1

Producer:
```text
Upload application
```

Consumer:
```text
Processing orchestration
```

Payload:
```json
{
  "video_id": "vid_123",
  "owner_id": "usr_123",
  "source_object_key": "users/usr_123/videos/vid_123/source/source.mp4",
  "processing_version": 1
}
```

Consumer:
```text
validate
↓
create job if absent
↓
mark QUEUED
↓
ACK after DB commit
```

Milestone 9 still uses the one-hop `video.uploaded.v1` topology. PostgreSQL
schedules the sequential worker job; `processing.execute.v1` remains a later
service boundary.

## 3. processing.execute.v1

Producer:
```text
Processing orchestration
```

Consumer:
```text
Processing worker
```

Payload:
```json
{
  "video_id": "vid_123",
  "job_id": "job_123",
  "source_object_key": "users/usr_123/videos/vid_123/source/source.mp4",
  "processing_version": 1
}
```

## 4. processing.completed.v1

```json
{
  "video_id": "vid_123",
  "job_id": "job_123",
  "processing_version": 1,
  "outputs": {
    "thumbnail": true,
    "hls_master": true,
    "renditions": ["360p", "720p", "1080p"]
  }
}
```

Media state:
```text
PACKAGING
↓
READY
```

## 5. processing.failed.v1

```json
{
  "video_id": "vid_123",
  "job_id": "job_123",
  "attempt": 3,
  "error_code": "INVALID_MEDIA_STREAM",
  "retryable": false
}
```

## 6. Retry classification

Retryable:
```text
OBJECT_STORAGE_TIMEOUT
OBJECT_STORAGE_TEMPORARY_ERROR
BROKER_TEMPORARY_ERROR
PROCESS_INTERRUPTED
TEMPORARY_NETWORK_FAILURE
```

Non-retryable thường gặp:
```text
INVALID_MEDIA_STREAM
UNSUPPORTED_MEDIA
INVALID_PROCESSING_CONFIGURATION
```

Không dùng riêng FFmpeg exit code để quyết định retry.
Broker publish failures retry the outbox event; they do not consume a
processing-job attempt. MinIO timeouts and 5xx errors retry the job. MinIO
credential or bucket configuration errors are terminal.

## 7. Retry policy

```text
attempt 1 → wait 30s → attempt 2 → wait 2m → attempt 3
→ wait 10m → attempt 4 → FAILED + DLQ if still retryable
```

The worker polls due jobs every five seconds. A claim assigns a new `run_id`
and 90-second lease; a heartbeat renews it every 15 seconds. An expired lease
counts as `PROCESS_INTERRUPTED` and follows the same bounded schedule. A
non-retryable error transitions directly to `FAILED` without another attempt.
`next_retry_at` lives in PostgreSQL; RabbitMQ does not hot-requeue processing
failures.

## 8. Dead-letter queue

```text
processing.dead.v1
```

Giữ:
- original message
- failure reason
- attempt count
- last failure time

Không auto-loop DLQ.

The worker records invalid input and exhausted jobs in
`processing_dead_letters`, then publishes `processing.dead.v1` through an
outbox to the durable RabbitMQ queue of the same name. Redelivery of the same
invalid body or exhausted job does not create another logical dead letter.

## 9. Idempotency

Job identity:
```text
{video_id}:PROCESS:v{processing_version}
```

Stage operation identity:
```text
{video_id}:{stage}:v{processing_version}:{variant}
```

Ví dụ:
```text
vid_123:METADATA:v1:default
vid_123:THUMBNAIL:v1:default
vid_123:TRANSCODE:v1:360p
vid_123:HLS:v1:master
```

Ưu tiên DB unique constraint bảo vệ invariant.

Milestone 9 retries the whole pipeline, updating the same stage-operation row
for each successful rerun. Generated output paths include the `run_id`; only
the active run may update DB state, and a failed run cannot delete another
run's objects.

## 10. Ack policy

Không ACK trước safe completion point.

Sai:
```text
receive
ACK
start FFmpeg
crash
```

Đúng hơn:
```text
receive
validate and persist job
ACK
```

The PostgreSQL scheduler claims and executes the job separately. Publisher
confirms plus the upload outbox cover the DB-commit/broker-publish gap; duplicate
event delivery is expected and safe.

## 11. Stale event

Nếu reprocess v2 đang chạy thì completion của v1 không được overwrite v2.

Dùng:
```text
video_id
processing_version
run_id and unexpired lease
```

## 12. Outbox

Milestone 9:
```text
transaction
├── update DB
└── insert outbox
COMMIT
```

Publisher:
```text
read outbox
↓
publish RabbitMQ
↓
mark published
```

## 13. Queue topology

```text
exchange: video.events
queue: processing.video-uploaded.v1

exchange: processing.commands
queue: processing.execute.v1

exchange: processing.events
queue: media.processing-results.v1
```

Không tạo quá nhiều exchange/queue ở giai đoạn đầu.

## 14. Milestone 3 implementation boundary

Milestone 3 dùng topology một hop để giữ đúng boundary hiện tại của repository:

```text
API upload complete
      ↓
video.uploaded.v1
      ↓
processing.video-uploaded.v1
      ↓
Worker tạo processing_job = QUEUED
```

Worker và API cùng declare exchange, durable queue và binding để queue được
tạo ngay cả khi worker chưa khởi động trước lần upload đầu tiên.

`processing.execute.v1` là boundary dành cho bước orchestration tách riêng ở
milestone sau; Milestone 3 chưa thực hiện FFmpeg, ffprobe, retry, DLQ hay HLS.
