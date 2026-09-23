"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { deleteVideo, getVideo } from "@/lib/api/client";
import type { Video } from "@/types/video";

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export function VideoDetail({ videoId }: { videoId: string }) {
  const [video, setVideo] = useState<Video | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    let active = true;
    void getVideo(videoId)
      .then((response) => { if (active) setVideo(response.data); })
      .catch((loadError: unknown) => { if (active) setError(loadError instanceof Error ? loadError.message : "Unable to load video."); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
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
          </dl>
          {video.status === "CREATED" ? <button className="button button-danger" type="button" disabled={deleting} onClick={() => void handleDelete()}>{deleting ? "Deleting…" : "Delete video"}</button> : null}
        </div>
      ) : null}
    </section>
  );
}
