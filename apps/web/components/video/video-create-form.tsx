"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { abortUpload, completeUpload, createUpload, createVideo, uploadFile } from "@/lib/api/client";
import type { UploadSession } from "@/types/video";

const maxUploadSizeBytes = 100 * 1024 * 1024;
const allowedTypes = new Set(["video/mp4", "video/webm"]);

type UploadStage = "idle" | "uploading" | "confirming" | "error";

export function VideoCreateForm() {
  const router = useRouter();
  const [title, setTitle] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [videoId, setVideoId] = useState<string | null>(null);
  const [session, setSession] = useState<UploadSession | null>(null);
  const [progress, setProgress] = useState(0);
  const [stage, setStage] = useState<UploadStage>("idle");
  const [error, setError] = useState<string | null>(null);

  async function transfer(activeFile: File, activeSession: UploadSession) {
    setStage("uploading");
    setError(null);
    await uploadFile(activeSession, activeFile, setProgress);
    setStage("confirming");
    await completeUpload(activeSession.upload_id);
  }

  async function createSessionAndTransfer(activeVideoId: string, activeFile: File) {
    const uploadResponse = await createUpload(activeVideoId, {
      content_type: activeFile.type,
      size_bytes: activeFile.size,
    });
    setSession(uploadResponse.data);
    await transfer(activeFile, uploadResponse.data);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    if (!file) {
      setError("Choose an MP4 or WebM video first.");
      return;
    }
    if (!allowedTypes.has(file.type)) {
      setError("Only MP4 and WebM videos are supported.");
      return;
    }
    if (file.size > maxUploadSizeBytes) {
      setError("The video must be 100 MiB or smaller.");
      return;
    }

    try {
      const videoResponse = await createVideo({ title, original_filename: file.name });
      setVideoId(videoResponse.data.id);
      await createSessionAndTransfer(videoResponse.data.id, file);
      router.push(`/videos/${videoResponse.data.id}`);
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : "Unable to upload video.");
      setStage("error");
    }
  }

  async function handleRetry() {
    if (!file || (!session && !videoId)) return;
    try {
      if (session) {
        await transfer(file, session);
      } else if (videoId) {
        await createSessionAndTransfer(videoId, file);
      }
      if (videoId) router.push(`/videos/${videoId}`);
    } catch (retryError) {
      setError(retryError instanceof Error ? retryError.message : "Unable to retry upload.");
      setStage("error");
    }
  }

  async function handleAbort() {
    if (!session) return;
    try {
      await abortUpload(session.upload_id);
      router.push(videoId ? `/videos/${videoId}` : "/videos");
    } catch (abortError) {
      setError(abortError instanceof Error ? abortError.message : "Unable to abort upload.");
    }
  }

  const busy = stage === "uploading" || stage === "confirming";
  const canRetry = stage === "error" && Boolean(videoId) && Boolean(file);

  return (
    <form className="panel form-stack" onSubmit={handleSubmit}>
      {error ? <p className="alert alert-error" role="alert">{error}</p> : null}
      <div className="field">
        <label htmlFor="video-title">Title</label>
        <input id="video-title" name="title" maxLength={255} required value={title} onChange={(event) => setTitle(event.target.value)} disabled={busy} />
        <p className="field-help">Use a short name that will be easy to recognize in your library.</p>
      </div>
      <div className="field">
        <label htmlFor="video-file">Source video</label>
        <input id="video-file" name="video_file" type="file" accept="video/mp4,video/webm" required={!session} disabled={busy} onChange={(event) => setFile(event.target.files?.[0] ?? null)} />
        <p className="field-help">MP4 or WebM, maximum 100 MiB.</p>
      </div>
      {busy || progress > 0 ? (
        <div className="upload-progress" aria-live="polite">
          <div className="upload-progress-heading"><span>{stage === "confirming" ? "Confirming upload" : "Uploading"}</span><span>{progress}%</span></div>
          <progress max="100" value={progress}>{progress}%</progress>
        </div>
      ) : null}
      <div className="form-actions">
        {canRetry ? <button className="button button-secondary" type="button" onClick={() => void handleRetry()}>Retry upload</button> : null}
        {session && !busy ? <button className="button button-danger" type="button" onClick={() => void handleAbort()}>Abort upload</button> : null}
        {!canRetry ? <button className="button button-primary" type="submit" disabled={busy}>{busy ? (stage === "confirming" ? "Confirming…" : "Uploading…") : "Upload video"}</button> : null}
      </div>
    </form>
  );
}
