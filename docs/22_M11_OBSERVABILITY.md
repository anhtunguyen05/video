# Milestone 11 observability

Status: implementation in progress; end-to-end acceptance is pending.

The API preserves a valid `X-Request-ID` or generates a UUID. The request ID
from the upload completion that commits the outbox event becomes its
`correlation_id`. Repeated completion calls can have a different request ID:
search by `video_id` to find the original event. Older events retain their
existing correlation ID and can lack trace context.

The outbox stores an optional W3C `traceparent`. The publisher, consumer and
processing runs use that persisted parent so retries remain connected to the
original request. Publication, consumption and processing are sibling spans;
the run contains ffprobe, FFmpeg and storage operation spans. A publish span
indicates a publish attempt, not necessarily durable confirmation. An operation
span's error status indicates an error from the corresponding operation; a run
ending without a Go error does not by itself establish that the job succeeded.
Follow the persisted job/video status for the outcome. Logs expose request,
correlation, event, video, job and run IDs where available. Avoid putting signed
URLs, media bytes or credentials into logs or trace attributes.

To inspect traces locally, start the optional Jaeger service:

```sh
docker compose --profile observability up -d jaeger
```

Set `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318` for a locally running
API. For a Compose worker set `OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318`
in the environment used for `docker compose up` and recreate the worker.
The Jaeger UI is at `http://localhost:16686`. OTLP/HTTP export is disabled when
neither `OTEL_EXPORTER_OTLP_ENDPOINT` nor
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` is set. Configure endpoint authentication,
TLS, retention and access controls when using a non-local collector. Traces may
contain user-supplied request identifiers and should be protected.

Acceptance still requires an actual uploaded MP4 through PostgreSQL, RabbitMQ,
MinIO and a worker, with inspection of the resulting trace and logs, including
retry and failed operation cases. Prometheus metrics and Grafana dashboard from
the roadmap are not implemented. Unit tests do not establish runtime acceptance.
