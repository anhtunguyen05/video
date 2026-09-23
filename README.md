# Mini Video Processing Platform

A learning-oriented but production-minded video processing platform built with **Go** and **Next.js**.

The platform allows users to upload a source video, process it asynchronously, generate thumbnails and multiple renditions, package the result for web playback, and observe processing progress from the UI.

The project is intentionally designed to evolve from a small deployable system into a microservice-based architecture without forcing microservices on day one.

---

## 1. Project goals

This project should help demonstrate and practice:

- Go backend development
- Next.js frontend development
- asynchronous job processing
- FFmpeg-based media processing
- object storage
- message brokers
- retry and idempotency
- concurrency and worker scaling
- distributed system fundamentals
- observability
- containerization
- microservice evolution
- CI/CD
- production-oriented system design

---

## 2. Final user experience

A user should eventually be able to:

1. sign in
2. upload a video
3. see upload progress
4. see processing progress
5. wait while the video is processed asynchronously
6. receive a generated thumbnail
7. play the processed video in the browser
8. switch or automatically adapt playback quality
9. inspect processing status
10. retry failed processing when allowed
11. delete a video and its generated assets

Example lifecycle:

```text
UPLOAD
  ↓
UPLOADED
  ↓
QUEUED
  ↓
PROCESSING
  ↓
PACKAGING
  ↓
READY

or

PROCESSING
  ↓
FAILED
```

---

## 3. Target final outputs

For one uploaded video:

```text
video/
├── original/
│   └── source.mp4
├── thumbnails/
│   └── default.jpg
├── renditions/
│   ├── 360p/
│   ├── 720p/
│   └── 1080p/
└── hls/
    ├── master.m3u8
    ├── 360p/
    ├── 720p/
    └── 1080p/
```

The exact output depends on the source video. The system should not upscale blindly when the original resolution does not justify it.

---

## 4. Recommended document reading order

1. `01_PRODUCT_VISION.md`
2. `02_SCOPE_AND_REQUIREMENTS.md`
3. `03_SYSTEM_ARCHITECTURE.md`
4. `04_TECH_STACK.md`
5. `05_DOMAIN_AND_SERVICES.md`
6. `06_DATA_AND_EVENT_FLOW.md`
7. `07_API_AND_CONTRACTS.md`
8. `08_SECURITY_RELIABILITY_OBSERVABILITY.md`
9. `09_ROADMAP.md`
10. `10_DEFINITION_OF_DONE.md`
11. `11_ADR.md`

---

## 5. Recommended implementation strategy

Do not begin with five independent microservices.

Start with:

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

This already gives two independently scalable runtime units:

- API
- processing worker

Only extract more services after boundaries and operational needs become clear.

---

## 6. Final target architecture

A reasonable final learning architecture is:

```text
Next.js
   |
API Gateway
   |
   +---- Auth Service
   +---- Media Service
   +---- Upload Service
   +---- Processing Service
   +---- Notification Service

Infrastructure:
- PostgreSQL
- RabbitMQ
- Redis
- MinIO / S3
- FFmpeg
- OpenTelemetry
- Prometheus
- Grafana
```

Not every box needs to exist in the first release.

---

## 7. Definition of project success

The project is successful when:

- video upload works reliably
- processing is asynchronous
- processing survives worker restarts
- duplicate delivery does not create duplicate final outputs
- failed jobs have bounded retry behavior
- processed video is playable through HLS
- processing can run on multiple workers
- logs and traces allow a failed video job to be followed end-to-end
- services can be deployed independently in the final stage
- architecture decisions are documented rather than accidental
