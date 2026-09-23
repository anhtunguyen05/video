# Data Model

PostgreSQL là system of record.

## 1. users

| Column | Type | Nullable |
|---|---|---:|
| id | uuid | no |
| email | varchar(320) | no |
| password_hash | text | yes |
| created_at | timestamptz | no |
| updated_at | timestamptz | no |

Constraint:
```text
UNIQUE(email)
```

## 2. videos

| Column | Type | Nullable |
|---|---|---:|
| id | uuid | no |
| owner_id | uuid | no |
| title | varchar(255) | no |
| original_filename | varchar(255) | yes |
| status | varchar(32) | no |
| source_object_key | text | yes |
| source_size_bytes | bigint | yes |
| source_container | varchar(32) | yes |
| source_codec | varchar(64) | yes |
| duration_ms | bigint | yes |
| width | integer | yes |
| height | integer | yes |
| frame_rate | numeric(10,4) | yes |
| processing_version | integer | no |
| failure_code | varchar(64) | yes |
| failure_message | text | yes |
| created_at | timestamptz | no |
| updated_at | timestamptz | no |
| deleted_at | timestamptz | yes |

Indexes:
```text
(owner_id, created_at desc)
(status)
```

## 3. Video status

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

Main flow:
```text
CREATED
↓
UPLOADING
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
```

Không cho phép code đổi status tùy ý; state transition phải nằm trong domain/application rule.

## 4. video_uploads

| Column | Type | Nullable |
|---|---|---:|
| id | uuid | no |
| video_id | uuid | no |
| object_key | text | no |
| status | varchar(32) | no |
| expected_content_type | varchar(128) | yes |
| expected_size_bytes | bigint | yes |
| completed_size_bytes | bigint | yes |
| expires_at | timestamptz | no |
| completed_at | timestamptz | yes |
| created_at | timestamptz | no |
| updated_at | timestamptz | no |

Status:
```text
PENDING
UPLOADING
COMPLETED
ABORTED
EXPIRED
```

## 5. processing_jobs

| Column | Type | Nullable |
|---|---|---:|
| id | uuid | no |
| video_id | uuid | no |
| job_type | varchar(64) | no |
| stage | varchar(64) | no |
| status | varchar(32) | no |
| operation_key | varchar(255) | no |
| attempt | integer | no |
| max_attempts | integer | no |
| started_at | timestamptz | yes |
| finished_at | timestamptz | yes |
| next_retry_at | timestamptz | yes |
| error_code | varchar(64) | yes |
| error_message | text | yes |
| created_at | timestamptz | no |
| updated_at | timestamptz | no |

Critical constraint:
```text
UNIQUE(operation_key)
```

Example:
```text
vid_123:METADATA:v1
vid_123:THUMBNAIL:v1
vid_123:TRANSCODE:v1:720p
vid_123:HLS:v1
```

## 6. assets

| Column | Type | Nullable |
|---|---|---:|
| id | uuid | no |
| video_id | uuid | no |
| asset_type | varchar(32) | no |
| variant | varchar(32) | yes |
| object_key | text | no |
| content_type | varchar(128) | yes |
| size_bytes | bigint | yes |
| checksum | varchar(128) | yes |
| created_at | timestamptz | no |

Asset types:
```text
SOURCE
THUMBNAIL
RENDITION
HLS_MASTER
HLS_VARIANT
```

Không cần tạo DB row cho từng HLS segment ở bản đầu.

## 7. renditions

| Column | Type | Nullable |
|---|---|---:|
| id | uuid | no |
| video_id | uuid | no |
| name | varchar(32) | no |
| width | integer | no |
| height | integer | no |
| codec | varchar(64) | no |
| bitrate_kbps | integer | yes |
| status | varchar(32) | no |
| object_prefix | text | yes |
| created_at | timestamptz | no |
| updated_at | timestamptz | no |

Constraint:
```text
UNIQUE(video_id, name)
```

Status:
```text
PLANNED
PROCESSING
READY
FAILED
```

## 8. outbox_events

Thêm ở reliability milestone.

| Column | Type | Nullable |
|---|---|---:|
| id | uuid | no |
| aggregate_type | varchar(64) | no |
| aggregate_id | uuid | no |
| event_type | varchar(128) | no |
| payload | jsonb | no |
| created_at | timestamptz | no |
| published_at | timestamptz | yes |
| attempt_count | integer | no |

## 9. ER overview

```text
users
  |
  | 1:N
  v
videos
  |
  +---- 1:N video_uploads
  +---- 1:N processing_jobs
  +---- 1:N assets
  +---- 1:N renditions
```

## 10. Ownership

User-facing video query phải gắn owner:
```text
WHERE id = ? AND owner_id = ?
```

Không dùng UUID khó đoán thay cho authorization.

## 11. Không lưu vào PostgreSQL

Không lưu:
```text
video binary
thumbnail binary
HLS segments
large FFmpeg output
```

Media nằm ở object storage.

## 12. Transaction boundary

Có thể atomically:
```text
update DB row
insert processing job
insert outbox event
```

Không được giả định:
```text
PostgreSQL + RabbitMQ + MinIO
```

là cùng một transaction.
