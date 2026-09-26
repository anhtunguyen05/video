export type VideoStatus = "CREATED" | "UPLOADING" | "UPLOADED" | "QUEUED" | "PROCESSING" | "READY" | "FAILED" | "DELETED";

export type VideoMetadata = {
  duration_ms: number | null;
  width: number | null;
  height: number | null;
  codec: string | null;
  container: string | null;
  source_size_bytes: number | null;
  frame_rate: number | null;
};

export type VideoFailure = {
  code: string | null;
  message: string | null;
};

export type VideoThumbnail = {
  url: string;
  expires_at: string;
};

export type Video = {
  id: string;
  title: string;
  original_filename: string | null;
  status: VideoStatus;
  metadata: VideoMetadata | null;
  failure: VideoFailure | null;
  thumbnail: VideoThumbnail | null;
  processing_version: number;
  created_at: string;
  updated_at: string;
};

export type VideoList = {
  items: Video[];
  next_cursor: string | null;
};

export type VideoEnvelope = { data: Video };
export type VideoListEnvelope = { data: VideoList };

export type CreateVideoInput = {
  title: string;
  original_filename?: string;
};

export type CreateUploadInput = {
  content_type: string;
  size_bytes: number;
};

export type UploadSession = {
  upload_id: string;
  upload_url: string;
  method: "PUT";
  headers: Record<string, string>;
  expires_at: string;
};

export type UploadEnvelope = { data: UploadSession };

export type UploadCompleteEnvelope = {
  data: {
    video_id: string;
    status: "UPLOADED";
  };
};
