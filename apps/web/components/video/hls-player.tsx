"use client";

import Hls from "hls.js";
import { useEffect, useRef, useState } from "react";

export function HlsPlayer({ src, onRefresh, subtitleSrc }: { src: string; onRefresh: () => void; subtitleSrc?: string }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const subtitleRef = useRef<HTMLTrackElement>(null);
  const [failure, setFailure] = useState<{ src: string; message: string } | null>(null);
  const [subtitlesEnabled, setSubtitlesEnabled] = useState(true);
  const [subtitleFailure, setSubtitleFailure] = useState<string | null>(null);
  const error = failure?.src === src ? failure.message : null;

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    if (!Hls.isSupported() && video.canPlayType("application/vnd.apple.mpegurl")) {
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

  useEffect(() => {
    if (subtitleSrc && subtitleRef.current) {
      subtitleRef.current.track.mode = subtitlesEnabled ? "showing" : "disabled";
    }
  }, [subtitleSrc, subtitlesEnabled]);

  return (
    <section className="playback-section" aria-label="HLS video player">
      <video ref={videoRef} className="hls-video" controls playsInline preload="metadata" onError={() => setFailure({ src, message: "Playback failed. Refresh the playback link and try again." })}>
        {subtitleSrc ? <track ref={subtitleRef} kind="subtitles" src={subtitleSrc} srcLang="vi" label="Tiếng Việt (AI)" default
          onLoad={() => {
            if (subtitleRef.current) subtitleRef.current.track.mode = subtitlesEnabled ? "showing" : "disabled";
            setSubtitleFailure(null);
          }}
          onError={() => setSubtitleFailure("Không thể tải phụ đề. Hãy tải lại trang.")} /> : null}
        Your browser cannot play this video.
      </video>
      {subtitleSrc ? <button className="button button-secondary" type="button" aria-pressed={subtitlesEnabled} onClick={() => setSubtitlesEnabled((enabled) => !enabled)}>
        {subtitlesEnabled ? "Tắt phụ đề" : "Bật phụ đề"}
      </button> : null}
      {subtitleFailure ? <p className="playback-error" role="alert">{subtitleFailure}</p> : null}
      {error ? <div className="playback-error" role="alert"><p>{error}</p><button className="button button-secondary" type="button" onClick={onRefresh}>Refresh playback</button></div> : null}
    </section>
  );
}
