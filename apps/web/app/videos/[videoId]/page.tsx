type VideoPageProps = {
  params: Promise<{ videoId: string }>;
};

export default async function VideoPage({ params }: VideoPageProps) {
  const { videoId } = await params;

  return (
    <section className="placeholder-card">
      <p className="eyebrow">Video detail</p>
      <h1>Video {videoId}</h1>
      <p>Video metadata and playback will be connected in later milestones.</p>
    </section>
  );
}

