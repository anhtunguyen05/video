"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { deleteVideo, listVideos } from "@/lib/api/client";
import type { Video } from "@/types/video";

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export function VideoList() {
  const [videos, setVideos] = useState<Video[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deletingID, setDeletingID] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    void listVideos()
      .then((response) => {
        if (active) setVideos(response.data.items);
      })
      .catch((loadError: unknown) => {
        if (active) setError(loadError instanceof Error ? loadError.message : "Unable to load videos.");
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  async function handleDelete(video: Video) {
    if (!window.confirm(`Delete “${video.title}”?`)) return;
    setDeletingID(video.id);
    setError(null);
    try {
      await deleteVideo(video.id);
      setVideos((current) => current.filter((item) => item.id !== video.id));
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : "Unable to delete video.");
    } finally {
      setDeletingID(null);
    }
  }

  return (
    <section className="page-stack" aria-labelledby="videos-heading">
      <div className="page-heading">
        <div>
          <p className="eyebrow">Library</p>
          <h1 id="videos-heading">Your videos</h1>
          <p className="lede">Upload source videos directly to your private media library.</p>
        </div>
        <Link className="button button-primary" href="/videos/new">New video</Link>
      </div>
      {error ? <p className="alert alert-error" role="alert">{error}</p> : null}
      {loading ? <p className="panel muted" aria-live="polite">Loading videos…</p> : null}
      {!loading && videos.length === 0 ? (
        <div className="panel empty-state">
          <h2>No videos yet</h2>
          <p>Upload your first source video to start building the processing workflow.</p>
          <Link className="button button-secondary" href="/videos/new">Create a video</Link>
        </div>
      ) : null}
      {!loading && videos.length > 0 ? (
        <div className="panel table-wrap">
          <table>
            <thead><tr><th scope="col">Title</th><th scope="col">Status</th><th scope="col">Created</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead>
            <tbody>
              {videos.map((video) => (
                <tr key={video.id}>
                  <th scope="row"><Link className="text-link" href={`/videos/${video.id}`}>{video.title}</Link></th>
                  <td><span className={`status status-${video.status.toLowerCase()}`}>{video.status}</span></td>
                  <td>{formatDate(video.created_at)}</td>
                  <td className="table-actions">
                    <Link className="text-link" href={`/videos/${video.id}`}>View</Link>
                    <button className="button-link danger-link" type="button" disabled={deletingID === video.id} onClick={() => void handleDelete(video)}>{deletingID === video.id ? "Deleting…" : "Delete"}</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
