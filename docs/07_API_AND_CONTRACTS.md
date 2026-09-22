# API and Contracts

## 1. API style

Use REST for the first version.

Reasons:

- easy to inspect
- easy to document
- appropriate for resource-oriented control APIs
- straightforward OpenAPI generation

Use asynchronous messages for processing commands/events.

---

## 2. Suggested public API

### Authentication

```text
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/me
```

Auth implementation may be simplified during the earliest technical prototype.

---

### Videos

```text
GET    /api/v1/videos
POST   /api/v1/videos
GET    /api/v1/videos/{videoId}
DELETE /api/v1/videos/{videoId}
POST   /api/v1/videos/{videoId}/reprocess
```

---

### Uploads

```text
POST /api/v1/videos/{videoId}/uploads
POST /api/v1/uploads/{uploadId}/complete
POST /api/v1/uploads/{uploadId}/abort
```

---

### Processing

User-facing read:

```text
GET /api/v1/videos/{videoId}/processing
```

Avoid exposing internal broker operations as public APIs.

---

## 3. Example create video response

```json
{
  "data": {
    "id": "vid_123",
    "title": "demo.mp4",
    "status": "CREATED",
    "created_at": "2026-09-22T10:00:00Z"
  }
}
```

---

## 4. Example processing status

```json
{
  "data": {
    "video_id": "vid_123",
    "status": "PROCESSING",
    "stage": "TRANSCODING",
    "progress": {
      "completed": 2,
      "total": 4
    }
  }
}
```

Do not promise exact percentage unless it can be measured reliably.

---

## 5. Error envelope

Recommended:

```json
{
  "error": {
    "code": "VIDEO_NOT_FOUND",
    "message": "Video was not found.",
    "request_id": "req_123"
  }
}
```

Do not leak:

- raw SQL
- internal object paths unnecessarily
- FFmpeg command lines containing secrets
- stack traces

---

## 6. API contracts

Maintain an OpenAPI specification.

Use it to:

- document endpoints
- validate contracts
- generate frontend types if desired
- support automated tests

---

## 7. Internal message contracts

Message contracts should be versioned.

Example routing key:

```text
video.uploaded.v1
processing.requested.v1
processing.completed.v1
processing.failed.v1
```

Alternative:

Keep event type stable and include:

```json
{
  "schema_version": 1
}
```

Choose one convention and document it.

---

## 8. Contract evolution rule

Prefer additive changes.

Safer:

```json
{
  "video_id": "...",
  "new_optional_field": "..."
}
```

Riskier:

```text
rename required field
change meaning of existing field
change identifier semantics
```

Consumers should tolerate unknown fields.

---

## 9. Internal service communication

Start with:

```text
HTTP/JSON + RabbitMQ
```

Do not introduce gRPC until there is a concrete benefit.

Possible later gRPC use:

- internal low-latency metadata calls
- strong typed contracts across many Go services

But gRPC is not required for this project's learning goals.
