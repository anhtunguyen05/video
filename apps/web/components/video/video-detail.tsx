"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { deleteVideo, getVideo } from "@/lib/api/client";
import type { Video } from "@/types/video";

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function formatDuration(durationMs: number | null): string {
  if (durationMs === null) return "Pending";
  const totalSeconds = Math.round(durationMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

function formatBytes(value: number | null): string {
  if (value === null) return "Pending";
  if (value < 1024 * 1024) return `${Math.round(value / 1024)} KiB`;
  return `${(value / (1024 * 1024)).toFixed(2)} MiB`;
}

const processingStatuses = new Set<Video["status"]>(["UPLOADED", "QUEUED", "PROCESSING"]);

export function VideoDetail({ videoId }: { videoId: string }) {
  const [video, setVideo] = useState<Video | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let attempts = 0;

    async function load() {
      try {
        const response = await getVideo(videoId);
        if (!active) return;
        setVideo(response.data);
        setLoading(false);
        if (processingStatuses.has(response.data.status) && attempts < 30) {
          attempts += 1;
          timer = setTimeout(() => void load(), 1000);
        }
      } catch (loadError: unknown) {
        if (active) {
          setError(loadError instanceof Error ? loadError.message : "Unable to load video.");
          setLoading(false);
        }
      }
    }

    void load();
    return () => {
      active = false;
      if (timer) clearTimeout(timer);
    };
  }, [videoId]);

  async function handleDelete() {
    if (!video || !window.confirm(`Delete “${video.title}”?`)) return;
    setDeleting(true);
    setError(null);
    try {
      await deleteVideo(video.id);
      setVideo({ ...video, status: "DELETED" });
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : "Unable to delete video.");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <section className="page-stack" aria-labelledby="video-detail-heading">
      <Link className="text-link" href="/videos">← Back to videos</Link>
      {loading ? <p className="panel muted" aria-live="polite">Loading video…</p> : null}
      {error ? <p className="alert alert-error" role="alert">{error}</p> : null}
      {!loading && video ? (
        <div className="panel detail-card">
          <div className="page-heading">
            <div><p className="eyebrow">Video resource</p><h1 id="video-detail-heading">{video.title}</h1></div>
            <span className={`status status-${video.status.toLowerCase()}`}>{video.status}</span>
          </div>
          <dl className="metadata-grid">
            <div><dt>Original filename</dt><dd>{video.original_filename ?? "Not provided"}</dd></div>
            <div><dt>Created</dt><dd>{formatDate(video.created_at)}</dd></div>
            <div><dt>Updated</dt><dd>{formatDate(video.updated_at)}</dd></div>
            <div><dt>Processing version</dt><dd>{video.processing_version}</dd></div>
            <div><dt>Video ID</dt><dd className="breakable">{video.id}</dd></div>
            <div><dt>Duration</dt><dd>{formatDuration(video.metadata?.duration_ms ?? null)}</dd></div>
            <div><dt>Resolution</dt><dd>{video.metadata?.width && video.metadata?.height ? `${video.metadata.width} × ${video.metadata.height}` : "Pending"}</dd></div>
            <div><dt>Codec</dt><dd>{video.metadata?.codec ?? "Pending"}</dd></div>
            <div><dt>Container</dt><dd>{video.metadata?.container ?? "Pending"}</dd></div>
            <div><dt>Frame rate</dt><dd>{video.metadata?.frame_rate ? `${video.metadata.frame_rate.toFixed(2)} fps` : "Pending"}</dd></div>
            <div><dt>Source size</dt><dd>{formatBytes(video.metadata?.source_size_bytes ?? null)}</dd></div>
          </dl>
          {video.failure ? <p className="alert alert-error" role="alert">{video.failure.code}: {video.failure.message}</p> : null}
          {video.status === "CREATED" ? <button className="button button-danger" type="button" disabled={deleting} onClick={() => void handleDelete()}>{deleting ? "Deleting…" : "Delete video"}</button> : null}
        </div>
      ) : null}
    </section>
  );
}
