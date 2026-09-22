# Technical Stack

## 1. Frontend

### Next.js

Recommended usage:

- App Router
- TypeScript
- server components where useful
- client components only for interactive areas
- authenticated dashboard
- upload UI
- processing status UI
- HLS player integration

Suggested frontend responsibilities:

```text
auth UI
dashboard
upload initiation
direct-to-object-storage upload
status polling or SSE
video detail page
HLS playback
```

Avoid embedding backend domain rules in the frontend.

---

## 2. Backend

### Go

Recommended style:

- standard library where practical
- lightweight router such as `chi`
- explicit dependency injection
- `context.Context`
- structured logging
- graceful shutdown
- clear package boundaries

Avoid choosing a heavy framework merely to imitate Laravel/Spring conventions.

Suggested backend layout:

```text
cmd/
  api/
  worker/

internal/
  auth/
  media/
  upload/
  processing/
  notification/
  platform/
```

---

## 3. Database

### PostgreSQL

Use for:

- users
- video metadata
- processing jobs
- output records
- status history
- idempotency records where needed

Do not store large media blobs in PostgreSQL.

---

## 4. Message broker

### RabbitMQ

Recommended initial broker because the project benefits from:

- work queues
- acknowledgements
- retries
- routing
- dead-letter queues
- predictable job-processing semantics

NATS can be evaluated later if the project shifts toward broader event-driven messaging.

Kafka is not required for this project.

---

## 5. Redis

Use only when there is a concrete need.

Possible uses:

- distributed lock
- ephemeral progress
- rate limiting
- short-lived cache

Do not introduce Redis merely because microservice diagrams often contain Redis.

---

## 6. Object storage

### MinIO locally

Use for:

- original source video
- thumbnails
- generated MP4/renditions
- HLS playlists
- HLS segments

Production-compatible alternative:

```text
Amazon S3
or another S3-compatible provider
```

---

## 7. Media processing

### FFmpeg

Primary responsibilities:

- probe media
- transcode
- resize
- change codecs
- generate thumbnail
- package HLS

Use `ffprobe` for metadata extraction.

The Go service should supervise FFmpeg rather than reimplement media codecs.

---

## 8. Playback

Use:

```text
HLS
```

Browser strategy:

- Safari can support HLS natively
- other browsers can use an HLS JavaScript player such as hls.js

The frontend should not download the full source file for normal playback.

---

## 9. Local infrastructure

### Docker Compose

Expected local services:

```text
nextjs
api
worker
postgres
rabbitmq
redis
minio
```

Optional later:

```text
prometheus
grafana
otel-collector
```

---

## 10. Observability

Recommended stack:

```text
OpenTelemetry
Prometheus
Grafana
structured JSON logs
```

Optional tracing backend:

```text
Jaeger
Tempo
```

---

## 11. Gateway / reverse proxy

Initial:

```text
Nginx or Traefik
```

Responsibilities:

- TLS termination
- routing
- basic request limits
- forwarding headers

Do not put business authorization logic in the reverse proxy.

---

## 12. Deployment progression

```text
Local:
Docker Compose

First hosted version:
single VM / VPS + Docker

Later:
managed containers or Kubernetes

Only after:
worker autoscaling
service autoscaling
```

Kubernetes is a learning extension, not an MVP requirement.

---

## 13. Recommended stack summary

```text
Frontend        Next.js + TypeScript
Backend         Go
Router          chi
Database        PostgreSQL
Broker          RabbitMQ
Cache/locks     Redis
Storage         MinIO / S3
Media           FFmpeg + ffprobe
Streaming       HLS
Containers      Docker / Docker Compose
Gateway         Nginx or Traefik
Observability   OpenTelemetry + Prometheus + Grafana
CI/CD           GitHub Actions
```
