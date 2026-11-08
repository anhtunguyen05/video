import json
import subprocess
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from subtitle_engine import Cue, MediaProbeError, SubtitleEngine, has_audio, render_webvtt, word_timed_cues


def segment(start, end, text):
    return SimpleNamespace(start=start, end=end, text=text)


class SubtitleEngineTest(unittest.TestCase):
    def test_unicode_timing_and_escaped_cue_text(self):
        result = render_webvtt([segment(0, 1.25, " Xin chào\n\n<script> --> "),
                                segment(3600.001, 3601.1, "Việt Nam")])
        self.assertEqual(result, "WEBVTT\n\n00:00:00.000 --> 00:00:01.250\n"
                                 "Xin chào &lt;script&gt; --&gt;\n\n"
                                 "01:00:00.001 --> 01:00:01.100\nViệt Nam\n")

    def test_rejects_invalid_or_out_of_order_times(self):
        for segments in ([segment(2, 1, "x")], [segment(float("nan"), 1, "x")],
                         [segment(2, 3, "x"), segment(1, 2, "y")]):
            with self.subTest(segments=segments), self.assertRaises(ValueError):
                render_webvtt(segments)

    def test_no_speech_and_no_audio_are_distinct(self):
        model = SimpleNamespace(transcribe=lambda *args, **kwargs: (iter([]), None))
        engine = SubtitleEngine(model)
        with patch("subtitle_engine.has_audio", return_value=False):
            self.assertEqual(engine.transcribe(Path("source.mp4")).status, "NO_AUDIO")
        with patch("subtitle_engine.has_audio", return_value=True):
            self.assertEqual(engine.transcribe(Path("source.mp4")).status, "NO_SPEECH")

    def test_probe_errors_are_not_no_audio(self):
        with patch("subtitle_engine.subprocess.run", side_effect=subprocess.CalledProcessError(1, "ffprobe")):
            with self.assertRaises(MediaProbeError):
                has_audio(Path("broken.mp4"))
        with patch("subtitle_engine.subprocess.run", return_value=SimpleNamespace(stdout=json.dumps({"streams": []}).encode())):
            self.assertFalse(has_audio(Path("silent.mp4")))

    def test_asr_uses_vietnamese_transcription(self):
        class Model:
            def transcribe(self, source, **kwargs):
                self.args = (source, kwargs)
                return iter([segment(0, 1, "Xin chào")]), None
        model = Model()
        with patch("subtitle_engine.has_audio", return_value=True):
            result = SubtitleEngine(model).transcribe(Path("source.mp4"))
        self.assertEqual(model.args[1], {"language": "vi", "task": "transcribe", "word_timestamps": True,
                                         "vad_filter": True, "condition_on_previous_text": False})
        self.assertEqual(result.status, "READY")
        self.assertIn("Xin chào", result.webvtt)

    def test_long_segment_is_split_on_actual_word_times(self):
        speech = SimpleNamespace(start=0, end=20, text="Xin chào các bạn",
                                 words=[SimpleNamespace(start=1.0, end=1.5, word=" Xin"),
                                        SimpleNamespace(start=1.6, end=2.0, word=" chào"),
                                        SimpleNamespace(start=9.0, end=9.3, word=" các"),
                                        SimpleNamespace(start=9.4, end=10.0, word=" bạn")])
        track = render_webvtt(word_timed_cues([speech]))
        self.assertIn("00:00:01.000 --> 00:00:02.000\nXin chào", track)
        self.assertIn("00:00:09.000 --> 00:00:10.000\ncác bạn", track)

    def test_missing_word_timing_preserves_segment(self):
        speech = SimpleNamespace(start=1, end=4, text="Xin chào", words=[])
        self.assertEqual(render_webvtt(word_timed_cues([speech])),
                         "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nXin chào\n")

    def test_short_segment_is_split_at_silence(self):
        speech = SimpleNamespace(start=0, end=3, text="one two", words=[
            SimpleNamespace(start=0, end=0.3, word=" one"),
            SimpleNamespace(start=2, end=2.3, word=" two"),
        ])
        self.assertEqual(list(word_timed_cues([speech])), [
            Cue(0, 0.3, " one"),
            Cue(2, 2.3, " two"),
        ])

    def test_repeated_long_speech_is_not_discarded(self):
        text = "This sentence is long enough to be repeated in a song"
        cues = list(word_timed_cues([segment(1, 3, text), segment(10, 12, text)]))
        self.assertEqual(len(cues), 2)

    def test_missing_word_times_do_not_invent_alignment(self):
        text = "A long sentence without word timestamps should retain the segment interval"
        cues = list(word_timed_cues([segment(1, 10, text)]))
        self.assertEqual([(cue.start, cue.end, cue.text) for cue in cues], [(1, 10, text)])

    def test_unrepresentable_word_time_falls_back_without_losing_text(self):
        speech = SimpleNamespace(start=1, end=4, text="Xin chào", words=[
            SimpleNamespace(start=1.0001, end=1.0002, word=" Xin"),
            SimpleNamespace(start=2, end=3, word=" chào"),
        ])
        self.assertEqual(render_webvtt(word_timed_cues([speech])),
                         "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nXin chào\n")

if __name__ == "__main__":
    unittest.main()
