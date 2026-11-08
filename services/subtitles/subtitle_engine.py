"""Local batch ASR and WebVTT generation; orchestration is a separate concern."""

from __future__ import annotations

import html
import json
import math
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, Protocol


class Segment(Protocol):
    start: float
    end: float
    text: str


@dataclass(frozen=True)
class Cue:
    start: float
    end: float
    text: str


@dataclass(frozen=True)
class Transcription:
    status: str
    webvtt: str | None = None


class MediaProbeError(Exception):
    """The source could not be inspected; do not mistake this for no audio."""


def has_audio(source: Path, ffprobe: str = "ffprobe", timeout: int = 30) -> bool:
    try:
        result = subprocess.run(
            [ffprobe, "-v", "error", "-select_streams", "a:0", "-show_entries",
             "stream=index", "-of", "json", str(source)],
            capture_output=True, check=True, timeout=timeout,
        )
        data = json.loads(result.stdout)
        streams = data["streams"]
        if not isinstance(streams, list):
            raise ValueError("invalid streams")
        return bool(streams)
    except (OSError, subprocess.SubprocessError, ValueError, KeyError, TypeError) as exc:
        raise MediaProbeError("Unable to inspect media audio streams") from exc


def _timestamp(milliseconds: int) -> str:
    hours, remainder = divmod(milliseconds, 3_600_000)
    minutes, remainder = divmod(remainder, 60_000)
    seconds, millis = divmod(remainder, 1_000)
    return f"{hours:02}:{minutes:02}:{seconds:02}.{millis:03}"


def render_webvtt(segments: Iterable[Segment]) -> str | None:
    cues: list[str] = []
    last_start = -1
    for segment in segments:
        if not math.isfinite(segment.start) or not math.isfinite(segment.end):
            raise ValueError("Non-finite cue timestamp")
        start = round(segment.start * 1000)
        end = round(segment.end * 1000)
        if start < 0 or start < last_start or end <= start:
            raise ValueError("Invalid cue timestamp")
        last_start = start
        # Normalize whitespace to prevent blank lines or markup from injecting cues.
        text = " ".join(segment.text.split())
        if not text:
            continue
        text = html.escape(text, quote=False)
        cues.append(f"{_timestamp(start)} --> {_timestamp(end)}\n{text}")
    if not cues:
        return None
    return "WEBVTT\n\n" + "\n\n".join(cues) + "\n"


def word_timed_cues(segments: Iterable[Segment], max_seconds: float = 4, max_chars: int = 42, max_gap: float = 0.8) -> Iterable[Cue]:
    """Group model word timestamps into readable cues without retiming words.

    When a segment has no usable word timing, retain its original timing so the
    transcription is not silently dropped. Word timing is an estimate and must
    still be validated against representative audio.
    """
    for segment in segments:
        words = getattr(segment, "words", None) or []
        usable = [word for word in words if isinstance(word.word, str) and word.word.strip()
                  and math.isfinite(word.start) and math.isfinite(word.end)
                  and word.start >= 0 and round(word.end * 1000) > round(word.start * 1000)]
        if not usable or len(usable) != len([word for word in words if word.word.strip()]):
            # Do not invent word timing from character counts.
            yield Cue(segment.start, segment.end, segment.text)
            continue
        current = []
        for word in usable:
            if current:
                combined = "".join(item.word for item in current) + word.word
                if word.start < current[-1].start:
                    raise ValueError("Out-of-order word timestamps")
                if (word.start - current[-1].end > max_gap or
                        word.end - current[0].start > max_seconds or len(combined.strip()) > max_chars):
                    yield Cue(current[0].start, current[-1].end, "".join(item.word for item in current))
                    current = []
            current.append(word)
        if current:
            yield Cue(current[0].start, current[-1].end, "".join(item.word for item in current))


class SubtitleEngine:
    def __init__(self, model: object, ffprobe: str = "ffprobe") -> None:
        # The caller loads and retains one faster-whisper model per process.
        self.model = model
        self.ffprobe = ffprobe

    def transcribe(self, source: Path) -> Transcription:
        if not has_audio(source, self.ffprobe):
            return Transcription("NO_AUDIO")
        segments, _ = self.model.transcribe(
            str(source), language="vi", task="transcribe", word_timestamps=True,
            vad_filter=True, condition_on_previous_text=False,
        )
        track = render_webvtt(word_timed_cues(segments))
        if track is None:
            return Transcription("NO_SPEECH")
        return Transcription("READY", track)
