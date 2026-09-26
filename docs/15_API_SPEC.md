# API Specification

Base path:
```text
/api/v1
```

## 1. Response envelope

Success:
```json
{"data": {}}
```

Error:
```json
{
  "error": {
    "code": "VIDEO_NOT_FOUND",
    "message": "Video was not found.",
    "request_id": "req_123"
  }
}
```

## 2. Auth

### POST /auth/register

```json
{
  "email": "user@example.com",
  "password": "strong-password"
}
```

### POST /auth/login

```json
{
  "email": "user@example.com",
  "password": "strong-password"
}
```

Khuyến nghị browser app dùng secure HTTP-only session cookie.

### POST /auth/logout
```text
204 No Content
```

### GET /me

```json
{
  "data": {
    "id": "usr_123",
    "email": "user@example.com"
  }
}
```

## 3. Videos

### POST /videos

```json
{
  "title": "Summer Trip",
  "original_filename": "summer-trip.mp4"
}
```

Response:
```json
{
  "data": {
    "id": "vid_123",
    "title": "Summer Trip",
    "status": "CREATED",
    "created_at": "2026-09-23T02:00:00Z"
  }
}
```

### GET /videos

Query:
```text
limit
cursor
status
```

### GET /videos/{videoId}

```json
{
  "data": {
    "id": "vid_123",
    "title": "Summer Trip",
    "status": "READY",
    "metadata": {
      "duration_ms": 302000,
      "width": 1920,
      "height": 1080,
      "codec": "h264"
    },
    "thumbnail": {
      "url": "https://minio.example/.../thumbnails/default.jpg?...",
      "expires_at": "2026-09-26T12:15:00Z"
    },
    "renditions": [
      {"name": "360p", "status": "READY"},
      {"name": "720p", "status": "READY"},
      {"name": "1080p", "status": "READY"}
    ],
    "playback": {
      "hls_url": "..."
    }
  }
}
```

Unauthorized owner nên trả:
```text
404 VIDEO_NOT_FOUND
```

`thumbnail` is `null` while processing. When available, `url` is a short-lived
signed GET URL for the private thumbnail object. The API only resolves a
thumbnail through the authenticated video's owner-scoped record.

### DELETE /videos/{videoId}

```text
202 Accepted
```

### POST /videos/{videoId}/reprocess

```json
{
  "processing_version": 2
}
```

Response:
```text
202 Accepted
```

## 4. Uploads

### POST /videos/{videoId}/uploads

Request:
```json
{
  "content_type": "video/mp4",
  "size_bytes": 104857600
}
```

Response:
```json
{
  "data": {
    "upload_id": "upl_123",
    "upload_url": "http://minio/...",
    "method": "PUT",
    "headers": {
      "Content-Type": "video/mp4"
    },
    "expires_at": "2026-09-23T02:15:00Z"
  }
}
```

Errors:
```text
404 VIDEO_NOT_FOUND
409 UPLOAD_ALREADY_ACTIVE
413 FILE_TOO_LARGE
415 UNSUPPORTED_MEDIA_TYPE
```

### POST /uploads/{uploadId}/complete

Server verify:
- object tồn tại
- size hợp lệ
- đúng user
- upload session còn hợp lệ

Response:
```json
{
  "data": {
    "video_id": "vid_123",
    "status": "UPLOADED"
  }
}
```

Repeated complete phải idempotent.

### POST /uploads/{uploadId}/abort
```text
204 No Content
```

## 5. Processing

### GET /videos/{videoId}/processing

```json
{
  "data": {
    "video_id": "vid_123",
    "status": "PROCESSING",
    "stage": "TRANSCODING",
    "progress": {
      "completed_steps": 2,
      "total_steps": 4
    },
    "latest_error": null
  }
}
```

Failed:
```json
{
  "data": {
    "video_id": "vid_123",
    "status": "FAILED",
    "stage": "TRANSCODING",
    "latest_error": {
      "code": "INVALID_MEDIA_STREAM",
      "message": "The uploaded video could not be processed."
    }
  }
}
```

## 6. Playback

### GET /videos/{videoId}/playback

```json
{
  "data": {
    "type": "hls",
    "url": "signed-url-or-api-url",
    "expires_at": "2026-09-23T03:00:00Z"
  }
}
```

Only valid khi `READY`.

## 7. Request ID

Mọi response có:
```text
X-Request-ID
```

## 8. Pagination

Dùng cursor pagination:
```text
GET /videos?limit=20&cursor=...
```

## 9. OpenAPI

Sau khi API shape ổn:
```text
contracts/openapi/api.yaml
```

Markdown là human-readable source; OpenAPI là machine-readable contract.
