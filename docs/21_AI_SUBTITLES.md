# Planned AI subtitles for uploaded video

**Status: implemented, acceptance incomplete.** Migrations 9–12, opt-in upload,
request intake, batch executor, result outbox and projection, status/track API,
and player track are present. Isolated tests cover PostgreSQL transactions,
duplicate intake, worker fencing with fake ASR, and result projection. A local
CPU INT8 smoke test ran the pinned model on synthetic English speech and produced
WebVTT, including through the worker's model child. This does not establish
Vietnamese quality, browser playback, broker outage recovery, or deletion and
cleanup acceptance. Apply migrations through 12 and provision the model
before starting the subtitle Compose profile; see `services/subtitles/README.md`.
The existing video API currently allows deletion only in `CREATED`, so deletion
of an uploaded or in-flight video and its subtitle objects is not implemented.

The in-progress upload request accepts optional boolean `generate_subtitles`
(default `false`). With `true`, successful completion creates a `PENDING`
Media API projection and `subtitle.requested.v1` outbox event atomically. The
event on `video.events` contains a stable `message_id`/`job_id`, `video_id`,
`owner_id`, `source_object_key`, `source_version`, and `language` (`vi`); it is
routed to the durable `subtitles.requested.v1` queue. The event may be delivered
more than once; the intake deduplicates by job ID. This is an
internal, incomplete contract, not a promise that subtitles can yet be used.

`GET /api/v1/videos/{videoId}/subtitles` returns the owner-scoped state or
`NOT_REQUESTED`, independently of video READY. When the projection is `READY`,
it includes an owner-authorized same-origin track path; the track endpoint
serves UTF-8 WebVTT with private, no-store caching. The intake quarantines invalid
messages in `subtitles.invalid.v1` only after a publisher confirm, and leaves a
delivery unacknowledged if database or quarantine publication fails. The worker
keeps status `PENDING` until its final event is projected; a live `PROCESSING`
update is not currently emitted.

## Goal and scope

Generate optional, timestamped Vietnamese captions for uploaded videos using a
locally hosted speech recognition model. The initial increment is batch
transcription of one audio track into one Vietnamese WebVTT track. It does not
train a model. Live captions, translation, speaker diarization, caption editing,
multiple tracks and accuracy guarantees are outside this increment.

The upload UI offers an explicit “Generate Vietnamese subtitles with AI”
option, off by default. Persist the owner's selection with the upload so it
survives refreshes and upload-completion retries. A completed upload creates a
subtitle job only when that selection is enabled. No subtitle job, inference,
or track is created for an upload where it is disabled. Enabling generation
later on the video detail page is a possible follow-up, not a committed part
of this first increment; viewing or toggling a generated track remains in scope.

The model runs after upload, never in a browser or on every playback. Caption
processing is independent from media processing: video READY continues to mean
the existing media assets are ready, even if subtitles are queued, fail, or are
unavailable. The UI explains subtitle status separately.

This is batch processing, not live listening during playback: the worker reads
the uploaded source and produces a complete, validated WebVTT file before it is
made available. An implementation may decode or infer audio in chunks; it does
not need to hold the whole video in memory. No partial captions are exposed.
Once ready, the browser uses cue timestamps to display captions during playback
and seeking without rerunning inference. Processing time and quality must be
measured; video playback need not wait for the track.

## Proposed flow and responsibilities

```text
Browser       Go API / database           RabbitMQ          Subtitle worker
   | upload + opt-in |                       |                       |
   |-------------->| confirm upload          |                       |
   |               |-- opt-in job request via outbox -------------->|
   |               |                  separate queue                 |
   |               |                         |---- deliver --------->|
   |               |                         |       read source from MinIO
   |               |                         |       ASR -> WebVTT -> MinIO
   |               |<------- durable result event / projection ------|
   |-- GET status ->|                                                |
   |<-- status / authorized track URL -------|                       |
   |-- GET completed WebVTT via authorized API or signed MinIO URL -->|
   |       browser renders cues during HLS playback                   |
```

This is a logical diagram, not a deployed contract. Implement reliable job
creation with a database transaction and outbox or equivalent recovery: an
opted-in committed upload must not silently lose its subtitle job if publishing
fails. Repeated upload completion must produce at most one logical request per
video/source version and caption configuration.
Do not add subtitle consumers to the existing video-processing queue: competing
consumers would divide messages between the two job types. Use a separate
durable queue and explicit routing, or an independently durable scheduler. A
versioned message carries job ID, video ID, source identity/version and
correlation ID; it does not contain audio, transcripts or credentials.

The proposed ownership boundary is: Media API owns the upload choice and the
owner-facing subtitle status/track projection; the subtitle service owns its
execution jobs, attempts and result production. It sends versioned result events
through its own transactional outbox; Media API consumes them idempotently and
updates its projection. Workers do not write Media API-owned tables. The
services may share a PostgreSQL instance locally while retaining table ownership.
Messages are at-least-once: deduplicate using stable event/job identities and
reject stale source versions and runs. Define the exact wire schema, projection
ordering, ownership, transaction boundaries and deletion protocol before code.
The M12 processing service currently shares data ownership, so this subtitle
boundary is a proposed design, not an existing integration.

The worker resolves the private source, selects a supported audio stream,
decodes or extracts audio, runs local ASR, converts timed segments into UTF-8
WebVTT and uploads an immutable run-scoped result. Keep one loaded model per
worker process and initially process at most one subtitle job at a time. The
backend exposes an owner-authorized status and, when ready, either a short-lived
signed track URL or an authenticated same-origin endpoint. Browser access to a
signed URL requires appropriate CORS and expiry/refresh handling. The player
uses a caption track and supports enabling, disabling and seeking. Refresh the
status while it is pending so a user can see the track become available without
re-uploading.

Timing refinement: the worker enables voice activity detection and disables
conditioning on previous text to reduce silence hallucinations and feedback
loops. It groups word timestamps into short cues and splits at pauses. When
word times are missing or invalid it retains the segment as one cue instead of
inventing word alignment; such a cue can still be long. Repeated speech is not
discarded merely because its text matches earlier speech. These measures do not
correct recognition errors or establish synchronization. Existing tracks remain unchanged, and the
running worker must be rebuilt and the specific subtitle job regenerated before
the change appears in playback. Validate with a human-reviewed Vietnamese clip
and browser playback before considering the reported timing issue fixed.

## State, reliability and lifecycle

Proposed independent states: `PENDING`, `PROCESSING`, `READY`, `NO_AUDIO`,
`NO_SPEECH`, and `FAILED`. No audio is established from the media stream; no
speech means inference completed without recognized cues. Neither should invent
caption text. `FAILED` carries a safe reason for the owner, with diagnostic
details in restricted logs. Only `READY` exposes a validated track.

Persist job identity, video and owner association, source/version, state,
attempts, timestamps, detected language where useful, model identifier/version,
and a reference to the winning output. Exact schema, event and endpoint names
are implementation decisions, not existing contracts. Treat transcript text as
private content; avoid putting it in ordinary logs or queue messages.

Use idempotent job creation and bounded retries for transient broker, database
and storage errors. Define terminal handling for unsupported audio, corrupt
source or invalid configuration. Lease/heartbeat and a run identity must fence
stale workers: duplicate delivery or a crash must not publish conflicting final
results. Only mark a track READY after validating and persisting its object and
metadata. Clean failed and superseded run objects and temporary audio files.

Deletion must revoke future discovery and coordinate with in-flight jobs: check
the video lifecycle before committing a result, fence or cancel stale work, and
clean all subtitle outputs, including late writes. A previously issued direct
signed URL can remain usable until expiry or object deletion; choose a short
TTL and document this limit. Integrate subtitle cleanup with existing video
deletion and verify it rather than assuming it is immediate.

## Model and resources

Candidate: Python `faster-whisper` with multilingual `small` and INT8. Do not
use an English-only `.en` model for Vietnamese. Start with CPU INT8 for a
portable baseline or evaluate NVIDIA GPU INT8 where the CUDA/cuDNN stack and
container GPU access are compatible. Explicit language `vi` is the initial
Vietnamese use case; define behavior for other languages before claiming support.
Pin and document the model, runtime and dependencies, cache model files, and
provide a reproducible download/provisioning step. Runtime inference is local;
provisioning model weights may require network access.

The development machine observed on 2026-11-03 had Ryzen 5 5600H, about
15.4 GiB RAM and an NVIDIA RTX 3050 Laptop GPU with 4 GiB VRAM. These are a
reference, not a minimum requirement or a verified benchmark. The machine also
runs Docker, the web app and CPU-intensive FFmpeg workers. GPU VRAM, system RAM,
CPU, temporary disk and model cache are shared constraints; start with one ASR
job and measure while representative video jobs run. CPU-only fallback may be
slow. Avoid scheduling multiple model processes without measuring their memory.

Record model, precision, hardware, audio duration, language, wall time, peak
RAM/VRAM, CPU load and disk usage. Compare short and longer Vietnamese clips,
including noise and accents, against a human-reviewed reference. Measure error
and timing quality as well as throughput. The selected model and hardware are
not validated until those results meet project-defined targets.

## Implementation and acceptance

1. Define the opt-in persistence, job, durable routing, result projection, access
   and deletion contracts; add migrations and compatible versioned messages
   before introducing a consumer. Cover repeat completion and publisher outages.
2. Add a local worker with pinned model/runtime, resource limits, timeout and
   cleanup; generate and validate WebVTT from timed ASR segments.
3. Add owner-scoped status and track access, plus a player track selector and
   refresh behavior. Make clear that machine-generated captions can be wrong.
4. Test unit behavior for timestamp formatting, cue validation, transitions,
   retries and fencing. Integration-test DB/outbox, separate RabbitMQ routing,
   storage and expiry with deterministic fake ASR; do not require model
   downloads in ordinary CI.
5. In an opt-in local acceptance run, process a consented Vietnamese fixture
   with real ASR. Verify Vietnamese characters, cue timing, playback/seek and
   toggling in a browser, no-audio/no-speech cases, failure and restart, duplicate
   delivery, authorization, expired links and deletion during processing. Verify
   opt-out creates no job and opt-in produces one recoverable job despite retries.
   Record measured quality and resource use; a fake-model test alone does not
   prove real inference or browser playback.

Reference: [faster-whisper upstream documentation](https://github.com/SYSTRAN/faster-whisper)
and [WebVTT specification](https://www.w3.org/TR/webvtt1/).
