export type VideoStatus = "CREATED" | "DELETED";

export type Video = {
  id: string;
  title: string;
  original_filename: string | null;
  status: VideoStatus;
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
