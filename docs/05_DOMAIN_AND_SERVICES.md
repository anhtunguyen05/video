# Domain and Service Boundaries

## 1. Core domain concepts

Recommended core entities:

### User

Owns videos.

### Video

Represents the logical uploaded media item.

Possible fields:

```text
id
owner_id
title
status
source_object_key
duration_ms
width
height
source_codec
source_size_bytes
created_at
updated_at
```

### Rendition

Represents one generated output quality.

```text
id
video_id
quality
width
height
codec
object_prefix
status
```

### ProcessingJob

Represents processing work.

```text
id
video_id
job_type
status
attempt
max_attempts
started_at
finished_at
error_code
error_message
```

### Asset

Represents generated media artifacts.

Examples:

```text
SOURCE
THUMBNAIL
HLS_MASTER
HLS_VARIANT
RENDITION
```

---

## 2. Final service boundaries

A reasonable final microservice target:

### Auth Service

Responsibilities:

- identity
- authentication
- authorization primitives
- access/session/token lifecycle

Does not:

- process videos
- own media metadata

---

### Media Service

Responsibilities:

- logical video resource
- title
- ownership
- video status
- metadata
- rendition catalog
- user-facing media queries

This should become the source of truth for the logical media resource.

---

### Upload Service

Responsibilities:

- create upload sessions
- issue presigned upload authorization
- validate upload completion
- multipart upload lifecycle
- upload constraints

It should not transcode media.

---

### Processing Service

Responsibilities:

- processing pipeline state
- orchestration
- job creation
- retry policy
- idempotency
- stage transitions
- dispatch work

It should not become a general user CRUD service.

---

### Processing Workers

Workers execute heavy jobs.

Examples:

```text
metadata worker
thumbnail worker
transcoding worker
packaging worker
```

These may be:

- separate process types
- or one worker binary with job handlers

Do not create independent network services for every job type unless there is a real need.

---

### Notification Service

Responsibilities:

- processing completed notifications
- processing failed notifications
- optional email
- optional SSE/WebSocket integration

This can remain inside the main application until there is a reason to extract it.

---

## 3. What should not become a microservice

Avoid:

```text
thumbnail-service
360p-service
720p-service
1080p-service
ffmpeg-service
database-service
redis-service
```

These are implementation details or infrastructure, not necessarily business capabilities.

---

## 4. Suggested modular boundaries before extraction

Before microservices:

```text
internal/
├── auth/
├── media/
├── upload/
├── processing/
├── notification/
└── platform/
```

Each module should own:

```text
domain types
application use cases
interfaces/ports
adapters
```

A module should not directly mutate another module's database internals.

---

## 5. Service extraction criteria

Extract a module when at least one strong reason exists.

Examples:

### Processing

Strong candidate because:

- CPU-heavy
- long-running
- independent scale
- crash isolation
- worker-oriented deployment

### Upload

Possible extraction because:

- large file lifecycle
- specialized storage concerns
- different traffic profile

### Notification

Possible extraction when:

- many delivery channels
- independent retry rules
- high fan-out

### Auth

Possible extraction when:

- multiple products/services share identity
- independent security lifecycle

---

## 6. Service ownership rule

A service should own its state.

Other services interact through:

```text
HTTP/gRPC contract
or
events/messages
```

Avoid:

```text
service A directly writing service B tables
```
