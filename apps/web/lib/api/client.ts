import type {
  CreateUploadInput,
  CreateVideoInput,
  UploadCompleteEnvelope,
  UploadEnvelope,
  UploadSession,
  VideoEnvelope,
  VideoListEnvelope,
} from "@/types/video";

type HealthResponse = {
  status: "ok";
};

const apiBasePath = "/api/backend/api/v1";

export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiBasePath + path, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
    cache: "no-store",
  });

  if (!response.ok) {
    let message = "The API request failed.";
    let code: string | undefined;
    try {
      const payload = (await response.json()) as { error?: { message?: string; code?: string } };
      message = payload.error?.message ?? message;
      code = payload.error?.code;
    } catch {
      // Keep the generic message when the response is not JSON.
    }
    throw new ApiError(message, response.status, code);
  }

  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export async function getLiveHealth(): Promise<HealthResponse> {
  const response = await fetch("/api/backend/health/live", {
    headers: { Accept: "application/json" },
    cache: "no-store",
  });

  if (!response.ok) {
    throw new Error("API health request failed with status " + response.status);
  }

  return response.json() as Promise<HealthResponse>;
}

export async function listVideos(): Promise<VideoListEnvelope> {
  return request<VideoListEnvelope>("/videos");
}

export async function getVideo(videoId: string): Promise<VideoEnvelope> {
  return request<VideoEnvelope>(`/videos/${encodeURIComponent(videoId)}`);
}

export async function createVideo(input: CreateVideoInput): Promise<VideoEnvelope> {
  return request<VideoEnvelope>("/videos", { method: "POST", body: JSON.stringify(input) });
}

export async function deleteVideo(videoId: string): Promise<void> {
  return request<void>(`/videos/${encodeURIComponent(videoId)}`, { method: "DELETE" });
}

export async function createUpload(videoId: string, input: CreateUploadInput): Promise<UploadEnvelope> {
  return request<UploadEnvelope>(`/videos/${encodeURIComponent(videoId)}/uploads`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function completeUpload(uploadId: string): Promise<UploadCompleteEnvelope> {
  return request<UploadCompleteEnvelope>(`/uploads/${encodeURIComponent(uploadId)}/complete`, { method: "POST" });
}

export async function abortUpload(uploadId: string): Promise<void> {
  return request<void>(`/uploads/${encodeURIComponent(uploadId)}/abort`, { method: "POST" });
}

export function uploadFile(
  session: UploadSession,
  file: File,
  onProgress: (percent: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open(session.method, session.upload_url, true);
    Object.entries(session.headers).forEach(([name, value]) => xhr.setRequestHeader(name, value));
    xhr.upload.addEventListener("progress", (event) => {
      if (event.lengthComputable) onProgress(Math.round((event.loaded / event.total) * 100));
    });
    xhr.addEventListener("load", () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        onProgress(100);
        resolve();
        return;
      }
      reject(new Error(`Storage upload failed with status ${xhr.status}.`));
    });
    xhr.addEventListener("error", () => reject(new Error("Storage upload failed.")));
    xhr.addEventListener("abort", () => reject(new Error("Storage upload was aborted.")));
    xhr.send(file);
  });
}

