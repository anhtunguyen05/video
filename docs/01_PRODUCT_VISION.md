# Product Vision

## 1. Problem

Raw videos are often unsuitable for direct web delivery.

They can be:

- too large
- encoded using inconvenient codecs
- too high resolution for a viewer's connection
- missing thumbnails
- difficult to stream efficiently
- expensive to process synchronously inside an HTTP request

The platform converts an uploaded source file into web-friendly media assets through an asynchronous processing pipeline.

---

## 2. Product statement

Build a mini video processing platform where users upload a source video and receive a processed, streamable result that can be played efficiently in a browser.

The product is not intended to compete with YouTube, Vimeo, Cloudflare Stream, or Mux.

It is a technical learning platform focused on:

- media processing
- asynchronous architecture
- workers
- events
- distributed systems
- infrastructure
- service decomposition

---

## 3. Primary user

Initial user:

```text
Individual user
```

The first version does not need:

- teams
- organizations
- subscriptions
- billing
- social sharing
- comments
- recommendations
- public discovery

These features distract from the project's technical objective.

---

## 4. Core user journey

```text
User opens dashboard
        ↓
Uploads video
        ↓
Upload completes
        ↓
Processing starts
        ↓
Thumbnail generated
        ↓
Video transcoded
        ↓
HLS generated
        ↓
Video becomes READY
        ↓
User plays processed video
```

---

## 5. Product principles

### Processing is asynchronous

A request must never wait for full video transcoding.

Bad:

```text
POST /videos
→ wait 8 minutes
→ response
```

Good:

```text
POST /videos
→ create video
→ enqueue work
→ return quickly
```

### Source files are immutable

The original upload should be treated as immutable.

Generated outputs can be reproduced from the source.

### Processing is observable

Every important stage should have a status and timestamps.

### Failures are expected

The system must assume:

- FFmpeg can fail
- workers can crash
- messages can be delivered more than once
- storage calls can fail
- clients can disconnect

### Architecture evolves with evidence

Do not create a service simply because a noun exists.

Create an independent service when there is a meaningful reason such as:

- independent scaling
- ownership boundary
- deployment independence
- distinct reliability needs
- distinct workload
- security boundary

---

## 6. Product states

Recommended video states:

```text
CREATED
UPLOADING
UPLOADED
QUEUED
PROCESSING
PACKAGING
READY
FAILED
DELETING
DELETED
```

Recommended processing job states:

```text
PENDING
RUNNING
SUCCEEDED
FAILED
RETRY_SCHEDULED
CANCELLED
```

---

## 7. Non-goals

The following are explicitly outside the first major version:

- live streaming
- DRM
- video editing
- collaborative editing
- AI highlight extraction
- automatic subtitles
- recommendation algorithms
- content moderation platform
- monetization
- creator analytics
- global multi-region delivery

They can become later extensions.
