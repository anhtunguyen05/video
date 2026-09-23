import Link from "next/link";
import { VideoCreateForm } from "@/components/video/video-create-form";

export default function NewVideoPage() {
  return (
    <section className="page-stack" aria-labelledby="new-video-heading">
      <Link className="text-link" href="/videos">← Back to videos</Link>
      <div>
        <p className="eyebrow">Videos</p>
        <h1 id="new-video-heading">New video</h1>
        <p className="lede">Create a video resource before connecting its source file.</p>
      </div>
      <VideoCreateForm />
    </section>
  );
}

