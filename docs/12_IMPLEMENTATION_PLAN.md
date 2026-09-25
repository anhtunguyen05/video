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

### Done when
Dashboard hiển thị thumbnail đã generate.

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

Generate:
```text
hls/
├── master.m3u8
├── 360p/
├── 720p/
└── 1080p/
```

Frontend dùng HLS player.

### Done when
Video `READY` có thể phát trên browser bằng HLS.

## 11. Milestone 9 — Reliability

Thêm:
- idempotency
- bounded retry
- retryable/non-retryable errors
- DLQ
- safe final state transition

Operation key:
```text
{video_id}:{stage}:{processing_version}:{variant}
```

Ví dụ:
```text
vid_123:TRANSCODE:v1:720p
```

### Done when
Deliver cùng message 2 lần nhưng chỉ có 1 logical result.

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
