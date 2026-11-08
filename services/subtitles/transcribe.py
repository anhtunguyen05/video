"""Opt-in local smoke runner for the subtitle engine, not a queue consumer."""

import argparse
import sys
from pathlib import Path

from subtitle_engine import MediaProbeError, SubtitleEngine


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate a Vietnamese WebVTT track locally")
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path, help="New output path; existing files are not overwritten")
    parser.add_argument("--model-dir", type=Path, required=True,
                        help="Provisioned faster-whisper/CTranslate2 multilingual model directory")
    parser.add_argument("--ffprobe", default="ffprobe")
    args = parser.parse_args()
    if not args.source.is_file() or not args.model_dir.is_dir():
        parser.error("source must be a file and --model-dir must be a provisioned model directory")
    # faster-whisper otherwise falls back to downloading a tokenizer even for
    # a local model directory.
    if not (args.model_dir / "tokenizer.json").is_file():
        parser.error("model directory must contain tokenizer.json for offline operation")
    if args.output.exists():
        parser.error("output already exists")

    try:
        from faster_whisper import WhisperModel
        model = WhisperModel(str(args.model_dir), device="cpu", compute_type="int8", local_files_only=True)
        if "vi" not in model.supported_languages:
            raise ValueError("Vietnamese requires a multilingual model")
        result = SubtitleEngine(model, args.ffprobe).transcribe(args.source)
        if result.webvtt is not None:
            with args.output.open("x", encoding="utf-8", newline="\n") as output:
                output.write(result.webvtt)
        print(result.status)
        return 0
    except (ImportError, MediaProbeError, OSError, ValueError, RuntimeError) as exc:
        # Model input and recognized speech are private; do not print exceptions.
        print(f"Transcription failed ({type(exc).__name__}).", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
