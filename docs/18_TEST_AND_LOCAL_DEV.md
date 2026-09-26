# Testing and Local Development

## 1. Dependencies

Bắt buộc:
```text
PostgreSQL
RabbitMQ
MinIO
FFmpeg
```

Sau này:
```text
Redis
Prometheus
Grafana
OpenTelemetry Collector
Tempo / Jaeger
```

## 2. Local development model

Khuyến nghị:
```text
dependencies chạy Docker
application chạy host
```

Docker:
- PostgreSQL
- RabbitMQ
- MinIO
- Worker with bundled FFmpeg/ffprobe (`make worker-docker`)

Host:
- Next.js
- Go API
- Go Worker (optional when the Docker worker is running)

## 3. Environment

```text
APP_ENV=local
API_PORT=8080

DATABASE_URL=postgres://video:video@localhost:5432/video?sslmode=disable

RABBITMQ_URL=amqp://guest:guest@localhost:5672/

S3_ENDPOINT=http://localhost:9000
S3_REGION=us-east-1
S3_ACCESS_KEY=minio
S3_SECRET_KEY=minio123
S3_BUCKET=video-platform
S3_USE_PATH_STYLE=true

FFPROBE_PATH=ffprobe
PROCESSING_TEMP_DIR=

SESSION_SECRET=change-me
LOG_LEVEL=debug
```

## 4. Start local

```bash
docker compose up -d
```

```bash
make migrate-up
```

API:
```bash
cd services/api
go run ./cmd/api
```

Worker:
```bash
cd services/worker
go run ./cmd/worker
```

Docker worker with bundled FFmpeg:
```bash
make worker-docker
```

When using the Docker worker, Docker service endpoints are used for
PostgreSQL, RabbitMQ, and MinIO. The host `FFPROBE_PATH` requirement does
not apply.

Web:
```bash
cd apps/web
npm install
npm run dev
```

## 5. Makefile

```text
make dev-infra-up
make dev-infra-down

make api
make worker
make web

make migrate-up
make migrate-down

make test
make test-unit
make test-integration

make lint
make fmt
make build
```

## 6. Media fixtures

```text
testdata/
├── video-360p-3s.mp4
├── video-720p-5s.mp4
├── video-1080p-5s.mp4
├── video-portrait-5s.mp4
├── video-no-audio-5s.mp4
├── invalid-media.bin
└── empty-file.mp4
```

Giữ fixture ngắn để CI nhanh.

## 7. Unit tests

### State transition
```text
CREATED → UPLOADING allowed
READY → PROCESSING không allowed nếu không reprocess
DELETED → READY impossible
```

### Rendition planning
```text
720p source
→ 360p + 720p
```

### Retry
```text
storage timeout
→ retryable

invalid media
→ non-retryable
```

### Operation key
Cùng video/stage/version phải tạo cùng logical identity.

## 8. Integration tests

### PostgreSQL
- repository
- unique constraints
- transactions
- state rules

### RabbitMQ
- publish
- consume
- ack
- retry route
- DLQ

### MinIO
- presigned upload
- object exists
- output upload
- delete object

### FFmpeg
- ffprobe valid fixture
- invalid fixture
- thumbnail generation and aspect-ratio/max-edge rules
- transcode
- HLS

## 9. E2E-001 Happy path

```text
create video
↓
create upload
↓
upload fixture
↓
complete
↓
worker consume
↓
metadata
↓
thumbnail
↓
renditions
↓
HLS
↓
READY
```

For the Milestone 5 slice, the acceptance point is `thumbnail` followed by
`READY`; rendition and HLS steps remain future scope.

## 10. E2E-002 Invalid media

```text
upload invalid-media.bin
↓
ffprobe fail
↓
non-retryable
↓
FAILED
```

## 11. E2E-003 Duplicate message

```text
publish same processing job twice
↓
consumer handles both
↓
one logical result
```

Assert:
```text
one logical job
one rendition per variant
one READY transition
```

## 12. E2E-004 Worker crash

```text
worker nhận job
↓
kill worker
↓
restart worker
↓
job eventually retry/completes
```

## 13. E2E-005 Temporary storage failure

```text
output upload fail temporarily
↓
retry
↓
success
```

## 14. E2E-006 Delete

```text
READY
↓
DELETE
↓
DELETING
↓
cleanup objects
↓
DELETED
```

## 15. Race-condition tests

Case:
```text
two workers cùng nhận logical stage
```

Expected:
```text
DB invariant bảo vệ duplicate logical result
```

Case:
```text
processing.completed v1 tới sau khi reprocess v2 bắt đầu
```

Expected:
```text
v1 không overwrite v2
```

## 16. API error tests

Bao gồm:
```text
401 unauthenticated
404 unauthorized ownership
400 validation
409 invalid state
413 file too large
415 unsupported media type
```

## 17. Frontend tests

Tập trung:
- upload flow
- processing status
- failed state
- READY player

Không cần snapshot test quá nhiều.

## 18. CI

```text
lint
↓
unit tests
↓
integration tests
↓
build Go
↓
build Next.js
```

Sau này:
```text
E2E media processing
Docker image build
security scanning
```

## 19. Definition before merge

Feature PR nên có:
```text
implementation
tests
migration nếu cần
contract update nếu API/event đổi
docs update nếu behavior architecture đổi
```

## 20. Debug order

Khi processing lỗi:
```text
1. video status
2. processing job status
3. correlation_id
4. worker logs
5. RabbitMQ queue
6. source object
7. FFmpeg/ffprobe error
8. generated objects
9. final DB state
```
