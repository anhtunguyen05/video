# Planned AI subtitles for uploaded video

**Status: proposed, not implemented.** This is a post-MVP extension. Existing
API, database, queue and startup commands do not provide this feature yet.

## Goal and scope

Generate optional, timestamped Vietnamese captions for uploaded videos using a
locally hosted speech recognition model. The initial increment is batch
transcription of one audio track into one Vietnamese WebVTT track. It does not
train a model. Live captions, translation, speaker diarization, caption editing,
multiple tracks and accuracy guarantees are outside this increment.

The model runs after upload, never in a browser or on every playback. Caption
processing is independent from media processing: video READY continues to mean
the existing media assets are ready, even if subtitles are queued, fail, or are
unavailable. The UI explains subtitle status separately.

## Proposed flow and responsibilities

```text
Browser       Go API / database           RabbitMQ          Subtitle worker
   | upload        |                         |                       |
   |-------------->| confirm upload          |                       |
   |               |-- durable subtitle job / outbox event -------->|
   |               |                  separate queue                 |
   |               |                         |---- deliver --------->|
   |               |                         |       read source from MinIO
   |               |                         |       ASR -> WebVTT -> MinIO
   |               |<------------------------ persist result/status --|
   |-- GET status ->|                                                |
   |<-- status / authorized track URL -------|                       |
   |--------------- GET completed WebVTT ---------------------> MinIO |
   |       browser renders cues during HLS playback                   |
```

This is a logical diagram, not a deployed contract. Implement reliable job
creation with a database transaction and outbox or equivalent recovery: a
committed upload must not silently lose its subtitle job if publishing fails.
Do not add subtitle consumers to the existing video-processing queue: competing
consumers would divide messages between the two job types. Use a separate
durable queue and explicit routing, or an independently durable scheduler. A
versioned message carries job ID, video ID, source identity/version and
correlation ID; it does not contain audio, transcripts or credentials. The exact
wire schema and ownership boundary must be defined before implementation.

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

1. Define the job, durable routing, access and deletion contracts; add migrations
   and compatible versioned messages before introducing a consumer.
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
   delivery, authorization, expired links and deletion during processing.
   Record measured quality and resource use; a fake-model test alone does not
   prove real inference or browser playback.

Reference: [faster-whisper upstream documentation](https://github.com/SYSTRAN/faster-whisper)
and [WebVTT specification](https://www.w3.org/TR/webvtt1/).
