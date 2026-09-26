# Repository Structure

Ban đầu dùng monorepo để dễ phát triển nhưng vẫn giữ ranh giới deployable unit rõ ràng.

```text
mini-video-platform/
├── apps/
│   └── web/
├── services/
│   ├── api/
│   └── worker/
├── contracts/
│   ├── openapi/
│   └── events/
├── migrations/
├── deploy/
├── docs/
├── scripts/
├── .github/workflows/
├── .env.example
├── Makefile
└── README.md
```

## 1. Next.js

```text
apps/web/
├── app/
│   ├── layout.tsx
│   ├── page.tsx
│   ├── login/page.tsx
│   └── videos/
│       ├── page.tsx
│       ├── new/page.tsx
│       └── [videoId]/page.tsx
├── components/
│   ├── video/
│   ├── upload/
│   ├── player/
│   └── ui/
├── lib/
│   ├── api/
│   ├── auth/
│   └── utils/
└── types/
    └── api.ts
```

## 2. Go API

```text
services/api/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── auth/
│   ├── media/
│   ├── upload/
│   ├── processing/
│   ├── notification/
│   └── platform/
├── go.mod
└── go.sum
```

## 3. Cấu trúc một module

Ví dụ `media`:

```text
internal/media/
├── domain/
│   ├── video.go
│   ├── status.go
│   ├── rendition.go
│   └── asset.go
├── application/
│   ├── create_video.go
│   ├── get_video.go
│   ├── list_videos.go
│   ├── delete_video.go
│   └── complete_processing.go
├── ports/
│   ├── video_repository.go
│   ├── asset_repository.go
│   └── event_publisher.go
├── infrastructure/
│   └── postgres/
│       ├── video_repository.go
│       └── asset_repository.go
└── transport/
    └── http/
        ├── handler.go
        ├── request.go
        ├── response.go
        └── routes.go
```

Dependency:
```text
transport
   ↓
application
   ↓
domain

infrastructure
   ↑
ports
```

Không cần Clean Architecture textbook-perfect; chỉ cần dependency rõ và testable.

## 4. Upload module

```text
internal/upload/
├── domain/
│   └── upload.go
├── application/
│   ├── create_upload.go
│   ├── complete_upload.go
│   └── abort_upload.go
├── ports/
│   ├── upload_repository.go
│   ├── object_storage.go
│   └── event_publisher.go
├── infrastructure/
│   ├── postgres/
│   └── s3/
└── transport/http/
```

## 5. Processing module

```text
internal/processing/
├── domain/
│   ├── job.go
│   ├── stage.go
│   └── error.go
├── application/
│   ├── enqueue_processing.go
│   ├── mark_started.go
│   ├── mark_completed.go
│   └── mark_failed.go
├── ports/
│   ├── job_repository.go
│   └── message_publisher.go
└── infrastructure/
    ├── postgres/
    └── rabbitmq/
```

## 6. Worker

```text
services/worker/
├── cmd/
│   └── worker/
│       └── main.go
├── internal/
│   ├── consumer/
│   │   └── video_uploaded.go
│   ├── processing/
│   │   ├── pipeline.go
│   │   ├── metadata.go
│   │   ├── thumbnail.go
│   │   ├── transcode.go
│   │   └── hls.go
│   ├── ffmpeg/
│   │   ├── client.go
│   │   └── ffprobe.go
│   ├── storage/
│   │   └── s3.go
│   ├── messaging/
│   │   └── rabbitmq.go
│   └── platform/
│       ├── config/
│       └── logging/
├── go.mod
└── go.sum
```

## 7. Shared code

Được chia sẻ:
```text
contracts
logging convention
ID helpers
small infra primitives
```

Không nên chia sẻ:
```text
business entities
repositories
application use cases
service-specific logic
```

## 8. Contracts

```text
contracts/
├── openapi/
│   └── api.yaml
└── events/
    ├── video-uploaded-v1.json
    ├── processing-completed-v1.json
    └── processing-failed-v1.json
```

## 9. Migrations

```text
migrations/
├── 000001_create_users.up.sql
├── 000002_create_videos.up.sql
├── 000003_create_video_uploads.up.sql
├── 000004_create_processing_jobs.up.sql
├── 000005_prepare_metadata_processing.up.sql
└── 000006_create_assets.up.sql
```

## 10. Platform package

`internal/platform/` chỉ chứa technical concerns:
```text
config
database
logging
httpserver
telemetry
ids
clock
```

Không đặt business logic vào đây.

## 11. Khi tách microservices

```text
services/
├── auth/
├── media/
├── upload/
├── processing/
└── notification/
```

Module boundary hiện tại nên map tương đối tốt sang service boundary tương lai.
