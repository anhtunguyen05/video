# System Architecture

## 1. Architecture strategy

The project should evolve through three architecture stages.

---

## Stage A — Modular application + worker

Recommended starting point:

```text
                   ┌───────────────┐
                   │    Next.js    │
                   └───────┬───────┘
                           │ HTTPS
                           v
                   ┌───────────────┐
                   │    Go API     │
                   └───────┬───────┘
                           │
            ┌──────────────┼───────────────┐
            │              │               │
            v              v               v
      PostgreSQL         MinIO          RabbitMQ
                                             │
                                             v
                                      ┌─────────────┐
                                      │ Go Worker   │
                                      └──────┬──────┘
                                             │
                                           FFmpeg
```

This stage is intentionally not "full microservices".

It already teaches:

- process separation
- async processing
- broker-based communication
- object storage
- worker lifecycle
- retry
- idempotency

---

## Stage B — Extract processing boundary

```text
Next.js
   |
API Gateway
   |
   +---- Media API
   |
   +---- Processing Service
            |
         RabbitMQ
            |
       Worker Pool
            |
          FFmpeg
```

Reasons to extract processing:

- CPU-heavy workload
- different scaling characteristics
- independent deployment
- different failure profile
- longer-running tasks

---

## Stage C — Target microservice architecture

```text
                         ┌───────────────┐
                         │    Next.js    │
                         └───────┬───────┘
                                 │
                                 v
                         ┌───────────────┐
                         │ API Gateway   │
                         └───────┬───────┘
                                 │
        ┌────────────────────────┼────────────────────────┐
        │                        │                        │
        v                        v                        v
  Auth Service              Media Service           Upload Service
                                  │                        │
                                  └─────────┬──────────────┘
                                            │
                                            v
                                       RabbitMQ
                                            │
                                            v
                                  Processing Service
                                            │
                                  ┌─────────┼─────────┐
                                  │         │         │
                                  v         v         v
                               Worker    Worker    Worker
                                  │         │         │
                                  └─────────┼─────────┘
                                            │
                                          FFmpeg
                                            │
                                            v
                                          MinIO
                                            │
                                            v
                                  Notification Service
```

---

## 2. Control plane vs data plane

A useful conceptual split:

### Control plane

Responsible for:

- users
- videos
- states
- permissions
- jobs
- metadata
- commands
- orchestration

Mostly Go services and PostgreSQL.

### Data/media plane

Responsible for:

- source video files
- generated video segments
- thumbnails
- transcode processes
- object storage transfer

Mostly MinIO/S3 + FFmpeg + workers.

This distinction keeps media bytes away from unnecessary application hops.

---

## 3. Upload strategy

Avoid proxying large video files through the Go API when possible.

Preferred flow:

```text
Browser
  |
  | 1. request upload authorization
  v
Go API
  |
  | 2. return presigned URL
  v
Browser
  |
  | 3. upload directly
  v
MinIO / S3
  |
  | 4. confirm upload
  v
Go API
```

Benefits:

- less API memory pressure
- less network duplication
- easier large-file handling
- storage scales independently

---

## 4. Processing orchestration

Recommended logical pipeline:

```text
UPLOAD_COMPLETED
      |
      v
EXTRACT_METADATA
      |
      v
GENERATE_THUMBNAIL
      |
      v
PLAN_RENDITIONS
      |
      +---- TRANSCODE_360P
      +---- TRANSCODE_720P
      +---- TRANSCODE_1080P
      |
      v
PACKAGE_HLS
      |
      v
VIDEO_READY
```

Do not require the first implementation to parallelize every rendition.

Start sequentially if needed; add parallelism after correctness.

---

## 5. Database ownership

Early stage:

```text
one PostgreSQL instance
one application database
clear module boundaries
```

Later microservice stage:

Prefer logical ownership.

Example:

```text
auth schema/database
media schema/database
processing schema/database
```

Do not allow arbitrary cross-service joins once services are independently deployed.

---

## 6. Communication style

Use synchronous communication for:

- user-facing reads
- immediate commands requiring a quick acknowledgement
- simple metadata lookup when justified

Use asynchronous communication for:

- processing
- thumbnail generation
- transcode
- packaging
- notifications
- cleanup

---

## 7. Consistency model

Do not pretend the whole distributed system is one ACID transaction.

Example:

```text
video row = PROCESSING
```

does not guarantee:

```text
all output files already exist
```

Final readiness should be derived from completed pipeline state.

The project should explicitly accept eventual consistency where appropriate.
