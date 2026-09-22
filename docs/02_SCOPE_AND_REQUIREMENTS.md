# Scope and Requirements

## 1. MVP scope

The MVP should support:

### User

- basic authentication
- authenticated dashboard
- ownership of uploaded videos

### Upload

- create upload
- upload source video
- validate content type and size
- confirm upload completion

### Video management

- list videos
- view video details
- delete video
- see current processing status

### Processing

- extract metadata
- generate thumbnail
- create selected renditions
- package HLS
- update status
- handle failed processing

### Playback

- browser playback using HLS
- display video title
- display thumbnail
- display duration
- show processing state when not ready

---

## 2. Suggested processing profile

Initial rendition targets:

```text
360p
720p
1080p
```

But output generation should respect source capability.

Example:

```text
source = 720p

generate:
✓ 360p
✓ 720p
✗ 1080p
```

---

## 3. Functional requirements

### FR-001 Create video record

The user can create a video upload session.

### FR-002 Upload source object

The source video is stored outside the application server filesystem.

Preferred target:

```text
MinIO during local development
S3-compatible storage in deployment
```

### FR-003 Confirm upload

The application verifies that the source object exists before marking the upload as complete.

### FR-004 Start processing

A completed upload produces a processing command/event.

### FR-005 Extract metadata

The processing worker extracts:

- duration
- width
- height
- codec
- frame rate when available
- source container
- file size

### FR-006 Generate thumbnail

The system generates at least one representative thumbnail.

### FR-007 Transcode renditions

The system generates supported renditions.

### FR-008 Package HLS

The system generates:

- variant playlists
- media segments
- master playlist

### FR-009 Track progress

The UI can display coarse-grained progress.

The first version does not require exact frame-level progress.

Suggested stages:

```text
UPLOAD_COMPLETE
METADATA
THUMBNAIL
TRANSCODING
PACKAGING
READY
```

### FR-010 Failure handling

Failures must be persisted and diagnosable.

### FR-011 Retry

Transient failures can be retried using bounded retry rules.

### FR-012 Delete

Deleting a video eventually removes:

- metadata
- generated assets
- source object according to retention policy

---

## 4. Non-functional requirements

### NFR-001 Asynchronous processing

Video transcoding must not execute inside a normal API request lifecycle.

### NFR-002 Idempotency

Reprocessing the same command should not accidentally produce duplicate logical state.

### NFR-003 At-least-once delivery awareness

Consumers must assume a broker can redeliver messages.

### NFR-004 Horizontal worker scaling

At least two worker replicas should be able to consume jobs safely.

### NFR-005 Observability

Every request and processing job should carry correlation identifiers.

### NFR-006 Graceful shutdown

Workers should:

- stop accepting new work
- finish or safely release current work
- close broker/database connections

### NFR-007 Security

Users must not be able to access another user's private videos through ID guessing.

### NFR-008 Reproducibility

The local development environment should be bootable using documented commands.

---

## 5. Stretch requirements

Optional after core completion:

- resumable multipart upload
- SSE/WebSocket progress updates
- dead-letter queue UI
- manual reprocess action
- signed playback URLs
- CDN integration
- worker autoscaling
- Kubernetes
- automatic subtitles
- waveform or preview clips
