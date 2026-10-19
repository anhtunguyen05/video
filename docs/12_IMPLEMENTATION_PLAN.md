# Implementation Plan

Mục tiêu của tài liệu này là chuyển architecture thành thứ tự code cụ thể.

Nguyên tắc: luôn ưu tiên vertical slice chạy được end-to-end.

## 1. Kiến trúc khởi đầu

```text
Next.js
   |
Go API
   |
   +---- PostgreSQL
   +---- MinIO
   +---- RabbitMQ
             |
             v
         Go Worker
             |
           FFmpeg
```

Redis chưa bắt buộc ở giai đoạn đầu.

## 2. Milestone 0 — Bootstrap

### Backend
- [ ] Khởi tạo Go module
- [ ] Tạo `services/api`
- [ ] Tạo `services/worker`
- [ ] Config loader
- [ ] Structured logger
- [ ] PostgreSQL connection
- [ ] Graceful shutdown
- [ ] `/health/live`
- [ ] `/health/ready`

### Frontend
- [ ] Khởi tạo Next.js + TypeScript
- [ ] App layout
- [ ] Dashboard shell
- [ ] API client cơ bản

### Infrastructure
- [ ] PostgreSQL
- [ ] RabbitMQ
- [ ] MinIO
- [ ] Docker Compose
- [ ] `.env.example`
- [ ] migration command

### Done when
`docker compose up -d` chạy dependencies và API health endpoint trả OK.

## 3. Milestone 1 — Video Resource

### API
```text
POST   /api/v1/videos
GET    /api/v1/videos
GET    /api/v1/videos/{videoId}
DELETE /api/v1/videos/{videoId}
```

### Database
```text
videos
```

### Frontend
```text
/videos
/videos/new
/videos/[videoId]
```

### Test
- create video
- get video
- list videos
- ownership
- video not found
- invalid state

### Done when
Tạo video từ UI và nhìn thấy nó trong dashboard.

## 4. Milestone 2 — Upload trực tiếp MinIO

### API
```text
POST /api/v1/videos/{videoId}/uploads
POST /api/v1/uploads/{uploadId}/complete
POST /api/v1/uploads/{uploadId}/abort
```

### Flow
```text
Browser
↓
create video
↓
request presigned URL
↓
upload trực tiếp MinIO
↓
confirm upload
↓
UPLOADED
```

### Database
```text
video_uploads
```

### Test
- valid upload
- missing object
- repeated complete
- unsupported media type
- file too large
- invalid owner

### Done when
File thật tồn tại trong MinIO và dashboard hiển thị `UPLOADED`.

## 5. Milestone 3 — RabbitMQ + Worker

Implementation choice: use the current API/worker boundary and consume
`video.uploaded.v1` directly in the worker. A separate processing orchestrator
and `processing.execute.v1` remain outside this milestone.

Sau upload complete:
```text
persist upload
↓
publish video.uploaded.v1
```

Worker:
```text
consume
↓
create processing job
↓
mark QUEUED
```

Database:
```text
processing_jobs
```

### Done when
Upload xong thì worker nhận được message thật.

## 6. Milestone 4 — ffprobe

Worker thực hiện `ffprobe` và lấy:
- duration
- width
- height
- codec
- container
- file size
- frame rate nếu có

### Done when
Trang video hiển thị metadata lấy từ file thật.

## 7. Milestone 5 — Thumbnail

```text
source
↓
FFmpeg
↓
thumbnail.jpg
↓
MinIO
```

Object key:
```text
users/{user_id}/videos/{video_id}/thumbnails/default.jpg
```

Implementation boundary:
- Generate one `image/jpeg` thumbnail per video processing version.
- Use an FFmpeg timestamp around 10% of the source duration, clamped to a
  valid timestamp for short videos.
- Preserve the source aspect ratio and cap the longest edge at 640 pixels.
- Persist the generated object as a `THUMBNAIL/default` asset. The asset
  identity is unique per video, asset type, and variant.
- Keep the object private in MinIO. The API returns a short-lived signed GET
  URL only after checking video ownership.
- Keep the existing sequential processing job. The video becomes `READY`
  only after metadata and thumbnail persistence succeed.
- Thumbnail generation failures are persisted as `FAILED`; retry policy,
  DLQ, outbox delivery, and worker scaling remain later milestones.

Database:
```text
assets
```

API/UI:
```text
Video.thumbnail.url
Video.thumbnail.expires_at
```

### Done when
Dashboard hiển thị thumbnail đã generate từ URL có chữ ký, video có asset
`THUMBNAIL/default`, và video chỉ chuyển sang `READY` sau khi asset tồn tại.

## 8. Milestone 6 — Rendition Planning

Target:
```text
360p
720p
1080p
```

Không upscale mặc định.

Ví dụ:
```text
source 720p
→ 360p
→ 720p
```

Database:
```text
renditions
```

### Done when
Hệ thống quyết định deterministic được cần generate quality nào.

## 9. Milestone 7 — Transcoding

Bắt đầu tuần tự:
```text
360p
↓
720p
↓
1080p
```

Output:
```text
users/{user_id}/videos/{video_id}/renditions/{quality}/video.mp4
```

### Done when
Generated rendition có thể tải và phát được.

## 10. Milestone 8 — HLS

Package the MP4 renditions from Milestone 7 without re-encoding. Use MPEG-TS
segments with a six-second target and VOD playlists. Persist one `HLS_MASTER`
asset and one `HLS_VARIANT` asset per rendition; keep segment objects private
without creating a database row for each segment.

Object keys:
```text
users/{owner_id}/videos/{video_id}/hls/master.m3u8
users/{owner_id}/videos/{video_id}/hls/{quality}/index.m3u8
users/{owner_id}/videos/{video_id}/hls/{quality}/segment_00000.ts
```

`GET /videos/{videoId}/playback` returns a short-lived playback response whose
manifest URL is served by the authenticated API. The API rewrites variant
playlists and signs segment URLs after checking ownership. The web player uses
native HLS when supported and `hls.js` otherwise.

Only mark the video `READY` after the master playlist, every expected variant,
and all referenced segments have been uploaded and their playlist assets have
been persisted. Packaging failure marks the processing job/video `FAILED` and
best-effort deletes uploaded HLS objects. Retry and DLQ remain Milestone 9.

### Done when
Video `READY` có thể phát trên browser bằng HLS.

## 11. Milestone 9 — Reliability

Keep one sequential processing job per video and processing version. The
`video.uploaded.v1` consumer validates and inserts the job, then ACKs after the
database commit. A worker polls due jobs every five seconds. Each retry starts
the pipeline again; stage operations have unique logical keys but no separate
queues in this milestone.

Job key: `{video_id}:PROCESS:v{processing_version}`. Stage key:
`{video_id}:{stage}:v{processing_version}:{variant}` (use `default` for
single-result stages; for example,
`vid_123:TRANSCODE:v1:720p`). The job claim atomically increments `attempt`,
assigns a fresh `run_id`, and grants a 90-second lease. Heartbeats renew it every
15 seconds. A write from an expired run or old processing version is rejected.

Allow four total attempts with waits of 30 seconds, two minutes, and ten
minutes. Retryable dependency/transport failures schedule `next_retry_at` in
PostgreSQL; invalid media, missing source, invalid processing configuration,
and invalid FFmpeg output are terminal. A crashed run is retried after its lease
expires. Only a terminal error or exhausted attempt budget marks the video
`FAILED`. Job `SUCCEEDED` and video `READY` are committed together after all
assets for the active run are persisted.

Write generated objects under
`users/{owner_id}/videos/{video_id}/processing/v{version}/runs/{run_id}/...`.
Asset records point at the winning run. A failed run's unreferenced objects are
removed after 24 hours; objects from older READY videos keep their existing keys.

Upload completion commits its `video.uploaded.v1` outbox event in the same
database statement. The outbox publisher uses a stable message ID, mandatory
routing, and publisher confirms. Invalid messages and exhausted jobs create a
durable dead-letter record and an outbox event for `processing.dead.v1`. The DLQ
does not replay automatically. No reprocess endpoint or separate processing
service is added in this milestone.

### Done when
Duplicate delivery produces one job and one logical result. A worker crash,
temporary storage outage, and broker outage recover without leaving a job
stuck or losing an acknowledged event. Exhausted attempts produce one
inspectable dead letter. Validate these cases with PostgreSQL, RabbitMQ, MinIO,
and an actual uploaded MP4 before marking the milestone complete.

## 12. Milestone 10 — Multiple Workers

Chạy:
```text
worker-1
worker-2
worker-3
```

Test:
- jobs phân phối
- video khác nhau chạy song song
- worker crash
- worker graceful shutdown
- không duplicate final result

### Done when
Tăng worker làm tăng throughput mà state vẫn đúng.

## 13. Milestone 11 — Observability

Thêm:
```text
structured logs
request_id
correlation_id
video_id
job_id
event_id
```

Sau đó:
```text
OpenTelemetry
Prometheus
Grafana
```

### Done when
Có thể trace một processing job từ API → RabbitMQ → Worker → FFmpeg → Storage.

## 14. Milestone 12 — Tách Processing Service

Chỉ bắt đầu sau khi flow chính ổn.

```text
Next.js
   |
Media API
   |
RabbitMQ
   |
Processing Service
   |
Worker Pool
```

Yêu cầu:
- deploy độc lập
- scale độc lập
- contract rõ
- không write trực tiếp bảng của service khác

## 15. Thứ tự code khuyến nghị

```text
1. bootstrap
2. video CRUD
3. MinIO upload
4. RabbitMQ
5. ffprobe
6. thumbnail
7. rendition planning
8. transcoding
9. HLS
10. retry/idempotency
11. multiple workers
12. observability
13. service extraction
```

Không thêm Kubernetes trước khi core processing và reliability hoạt động.
