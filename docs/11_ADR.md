# Architecture Decision Records

This file contains initial decisions. Split each ADR into its own file later if desired.

---

# ADR-001 — Use Go for backend services

## Status

Accepted

## Context

The project focuses on:

- asynchronous workers
- concurrency
- network services
- process control
- distributed systems
- infrastructure

## Decision

Use Go for backend APIs and workers.

## Consequences

Positive:

- simple deployment artifact
- strong concurrency primitives
- good fit for worker processes
- explicit context/cancellation
- good operational characteristics

Tradeoff:

- less framework convention than Laravel/Spring
- more architecture decisions must be made explicitly

---

# ADR-002 — Use Next.js for frontend

## Status

Accepted

## Decision

Use Next.js with TypeScript.

## Consequences

The project gains:

- modern React ecosystem
- routing
- server/client rendering options
- good dashboard development experience

The frontend remains separate from media processing concerns.

---

# ADR-003 — Use RabbitMQ for asynchronous processing

## Status

Accepted

## Context

The main requirement is reliable background work rather than high-volume event analytics.

## Decision

Use RabbitMQ for processing commands/events.

## Consequences

The project can practice:

- acknowledgements
- retries
- routing
- dead-letter queues
- competing consumers

Kafka is intentionally not selected initially.

---

# ADR-004 — Use object storage for media

## Status

Accepted

## Decision

Use MinIO locally and keep the storage interface S3-compatible.

## Consequences

Application servers do not need to persist large video files on local disks.

---

# ADR-005 — Use FFmpeg instead of implementing media processing

## Status

Accepted

## Decision

Use FFmpeg/ffprobe as external media tooling supervised by Go workers.

## Consequences

The project focuses on distributed processing architecture rather than codec implementation.

---

# ADR-006 — Do not begin with full microservices

## Status

Accepted

## Context

Service boundaries are clearer after real workloads and failure modes exist.

## Decision

Begin with:

```text
Next.js
Go API
Go Worker
PostgreSQL
RabbitMQ
MinIO
```

Maintain modular boundaries so services can be extracted later.

## Consequences

Positive:

- faster vertical slice
- less premature operational complexity
- easier debugging
- architecture can evolve from evidence

Tradeoff:

- later extraction work is intentionally part of the learning process

---

# ADR-007 — Processing is the first extraction candidate

## Status

Accepted

## Context

Processing differs from normal API workloads because it is:

- CPU intensive
- long-running
- failure-prone
- horizontally scalable
- asynchronous

## Decision

If the architecture is decomposed, Processing is the first major independent service boundary.

---

# ADR-008 — Use HLS for final browser streaming

## Status

Accepted

## Decision

Package processed video as HLS.

## Consequences

The platform can demonstrate:

- segmented delivery
- multiple renditions
- adaptive playback concepts
- stream-oriented output rather than only static MP4 download
