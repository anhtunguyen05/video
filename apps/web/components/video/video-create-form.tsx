"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { createVideo } from "@/lib/api/client";

export function VideoCreateForm() {
  const router = useRouter();
  const [title, setTitle] = useState("");
  const [filename, setFilename] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const response = await createVideo({
        title,
        ...(filename.trim() ? { original_filename: filename.trim() } : {}),
      });
      router.push(`/videos/${response.data.id}`);
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : "Unable to create video.");
      setSubmitting(false);
    }
  }

  return (
    <form className="panel form-stack" onSubmit={handleSubmit}>
      {error ? <p className="alert alert-error" role="alert">{error}</p> : null}
      <div className="field">
        <label htmlFor="video-title">Title</label>
        <input id="video-title" name="title" maxLength={255} required value={title} onChange={(event) => setTitle(event.target.value)} />
        <p className="field-help">Use a short name that will be easy to recognize in your library.</p>
      </div>
      <div className="field">
        <label htmlFor="original-filename">Original filename <span className="optional">(optional)</span></label>
        <input id="original-filename" name="original_filename" maxLength={255} value={filename} onChange={(event) => setFilename(event.target.value)} />
        <p className="field-help">The actual file upload will be connected in Milestone 2.</p>
      </div>
      <div className="form-actions"><button className="button button-primary" type="submit" disabled={submitting}>{submitting ? "Creating…" : "Create video"}</button></div>
    </form>
  );
}
