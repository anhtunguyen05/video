# Roadmap

## Phase 0 — Repository foundation

Goal: establish boundaries before feature work.

Deliverables:

- monorepo or clearly organized multi-app repo
- Next.js app
- Go API
- Go worker
- Docker Compose
- PostgreSQL
- RabbitMQ
- MinIO
- initial CI

Suggested repository:

```text
/
├── apps/
│   └── web/
├── services/
│   ├── api/
│   └── worker/
├── packages/
│   └── contracts/
├── deploy/
│   ├── docker/
│   └── compose/
├── docs/
└── .github/workflows/
```

---

## Phase 1 — Upload vertical slice

Goal:

```text
browser → storage → persisted video record
```

Deliver:

- create video
- presigned upload URL
- direct upload to MinIO
- upload complete endpoint
- video list/detail UI

Exit criteria:

A user can upload a source file and see it in the dashboard.

---

## Phase 2 — Basic processing

Goal:

```text
uploaded video → processed output
```

Deliver:

- processing queue
- worker consume
- ffprobe metadata
- thumbnail
- one initial transcode profile
- status persisted

Exit criteria:

A completed upload eventually becomes READY or FAILED.

---

## Phase 3 — Multi-rendition + HLS

Deliver:

- 360p
- 720p
- 1080p when source supports it
- HLS master playlist
- HLS playback in Next.js

Exit criteria:

Processed video can be streamed from the browser.

---

## Phase 4 — Reliability

Deliver:

- idempotency
- bounded retry
- DLQ
- cleanup
- safe worker shutdown
- duplicate delivery tests
- worker crash recovery tests

Exit criteria:

Restarting a worker does not corrupt logical processing state.

---

## Phase 5 — Concurrency and scaling

Deliver:

- multiple worker replicas
- concurrent processing
- resource limits
- worker concurrency configuration
- load test for queue behavior

Exit criteria:

Increasing worker replicas improves throughput without duplicate final state.

---

## Phase 6 — Observability

Deliver:

- structured logs
- request/correlation IDs
- OpenTelemetry
- metrics
- Grafana dashboard
- basic distributed trace

Exit criteria:

A failed job can be diagnosed from UI-visible IDs through logs/traces.

---

## Phase 7 — Service extraction

Start extracting only after the system works.

Recommended first extraction:

```text
processing-service
```

Possible later extractions:

```text
upload-service
notification-service
auth-service
```

Exit criteria:

At least one extracted service:

- owns its deployment
- owns its contract
- does not require direct table access to another service
- can be scaled independently

---

## Phase 8 — Production-style deployment

Deliver:

- reverse proxy
- HTTPS
- CI/CD
- migrations
- secret handling
- backups
- deployment documentation
- rolling restart strategy

Optional:

- Kubernetes
- Horizontal Pod Autoscaler
- managed PostgreSQL
- S3-compatible cloud storage

---

## Phase 9 — Extensions

Only after core project completion:

- subtitles
- AI chapters
- preview clips
- waveform
- CDN
- resumable upload
- live streaming
