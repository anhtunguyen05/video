# Definition of Done

The project should not be considered fully complete just because a video can be transcoded once.

Use the following checklist.

---

## 1. Product completion

- [ ] user can authenticate
- [ ] user can upload a supported video
- [ ] upload progress is understandable
- [ ] uploaded video appears in dashboard
- [ ] processing status is visible
- [ ] thumbnail is generated
- [ ] media metadata is visible
- [ ] multiple renditions are generated when appropriate
- [ ] HLS output is generated
- [ ] processed video plays in browser
- [ ] failure state is understandable
- [ ] user can delete own video

---

## 2. Backend completion

- [ ] API does not run full transcoding synchronously
- [ ] broker-based processing exists
- [ ] worker handles graceful shutdown
- [ ] retry is bounded
- [ ] non-retryable failures are distinguished
- [ ] messages can be delivered twice without corrupting logical state
- [ ] processing state is persisted
- [ ] storage failures are handled
- [ ] temporary files are cleaned
- [ ] database migrations exist

---

## 3. Concurrency completion

- [ ] at least two worker instances can run simultaneously
- [ ] two workers do not both create conflicting final logical results
- [ ] uniqueness/invariant rules are enforced
- [ ] duplicate processing tests exist
- [ ] race-sensitive state transitions are tested

---

## 4. Security completion

- [ ] user A cannot access user B's video through API
- [ ] storage access is appropriately private
- [ ] secrets are not committed
- [ ] unsafe filenames do not affect object paths
- [ ] upload size/type policies exist
- [ ] sensitive errors are not exposed

---

## 5. Observability completion

- [ ] structured logs
- [ ] request ID
- [ ] correlation ID
- [ ] video ID appears in processing logs
- [ ] job ID appears in worker logs
- [ ] processing metrics
- [ ] queue metrics
- [ ] health endpoints
- [ ] at least one useful trace through async processing

---

## 6. Testing completion

### Unit

- [ ] rendition planning
- [ ] state transition rules
- [ ] retry classification
- [ ] validation
- [ ] idempotency logic

### Integration

- [ ] PostgreSQL repository
- [ ] RabbitMQ publish/consume
- [ ] MinIO upload/read
- [ ] FFmpeg adapter

### End-to-end

- [ ] upload → ready
- [ ] invalid video → failed
- [ ] worker restart scenario
- [ ] duplicate message scenario
- [ ] delete video scenario

---

## 7. Deployment completion

- [ ] Docker images build
- [ ] Docker Compose works from clean checkout
- [ ] CI runs tests
- [ ] migrations run safely
- [ ] environment variables documented
- [ ] production config documented
- [ ] deployment process documented

---

## 8. Microservice completion

Only check these after service extraction.

- [ ] service boundaries have written reasons
- [ ] extracted services deploy independently
- [ ] services do not write each other's tables
- [ ] contracts are documented
- [ ] messages are versioned
- [ ] failure between services is handled
- [ ] distributed tracing works across boundaries
- [ ] independent worker/service scaling is demonstrated

---

## 9. Portfolio completion

- [ ] architecture diagram
- [ ] README
- [ ] demo screenshots/GIF
- [ ] design decisions
- [ ] failure scenarios documented
- [ ] performance/scaling experiment
- [ ] clear explanation of why microservices were introduced
- [ ] explanation of what would be changed for real production use

The strongest portfolio story is not:

> "I used microservices."

It is:

> "I started with a smaller architecture, observed where processing had different scaling and reliability characteristics, then extracted that boundary and demonstrated the operational consequences."
