"use client";

import Hls from "hls.js";
import { useEffect, useRef, useState } from "react";

export function HlsPlayer({ src, onRefresh }: { src: string; onRefresh: () => void }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = useState<{ src: string; message: string } | null>(null);
  const error = failure?.src === src ? failure.message : null;

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = src;
      return () => { video.removeAttribute("src"); video.load(); };
    }
    if (!Hls.isSupported()) {
      video.src = src;
      return;
    }
    const player = new Hls();
    player.loadSource(src);
    player.attachMedia(video);
    player.on(Hls.Events.ERROR, (_event, data) => {
      if (data.fatal) setFailure({ src, message: "Playback stopped. Refresh the playback link and try again." });
    });
    return () => { player.destroy(); };
  }, [src]);

  return (
    <section className="playback-section" aria-label="HLS video player">
      <video ref={videoRef} className="hls-video" controls playsInline preload="metadata" onError={() => setFailure({ src, message: "This browser cannot play the HLS stream." })}>
        Your browser cannot play this video.
      </video>
      {error ? <div className="playback-error" role="alert"><p>{error}</p><button className="button button-secondary" type="button" onClick={onRefresh}>Refresh playback</button></div> : null}
    </section>
  );
}
