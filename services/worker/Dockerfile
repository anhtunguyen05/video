FROM golang:1.23-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/video-worker ./cmd/worker

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install --no-install-recommends --yes ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --create-home --home-dir /home/worker worker \
    && mkdir -p /tmp/video-processing \
    && chown -R worker:worker /tmp/video-processing

COPY --from=builder /out/video-worker /usr/local/bin/video-worker

ENV FFPROBE_PATH=/usr/bin/ffprobe \
    PROCESSING_TEMP_DIR=/tmp/video-processing

USER worker
ENTRYPOINT ["/usr/local/bin/video-worker"]
