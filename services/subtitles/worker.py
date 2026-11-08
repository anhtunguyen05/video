"""Run one subtitle job at a time with a persistent local ASR child process."""

import logging
import multiprocessing
import os
import tempfile
import threading
import time
from pathlib import Path

from job_store import claim, finish, heartbeat, recover_expired


def model_process(pipe, model_dir):
    from faster_whisper import WhisperModel
    from subtitle_engine import SubtitleEngine

    try:
        if not (Path(model_dir) / "tokenizer.json").is_file():
            raise ValueError("local tokenizer required")
        model = WhisperModel(model_dir, device="cpu", compute_type="int8", local_files_only=True)
        if "vi" not in model.supported_languages:
            raise ValueError("multilingual model required")
        pipe.send(("INITIALIZED", None))
        engine = SubtitleEngine(model)
        while True:
            source = pipe.recv()
            try:
                result = engine.transcribe(Path(source))
                pipe.send((result.status, result.webvtt))
            except Exception as exc:
                pipe.send(("ERROR", type(exc).__name__))
    except (EOFError, BrokenPipeError):
        pass
    except Exception as exc:
        try:
            pipe.send(("INIT_ERROR", type(exc).__name__))
        except (EOFError, BrokenPipeError):
            pass
    finally:
        pipe.close()


class ModelChild:
    def __init__(self, model_dir):
        self.model_dir = model_dir
        self.process = None
        self.pipe = None
        self.start()

    def start(self):
        self.pipe, child = multiprocessing.Pipe()
        self.process = multiprocessing.Process(target=model_process, args=(child, self.model_dir), daemon=True)
        self.process.start()
        child.close()
        if not self.pipe.poll(300):
            self.stop()
            raise RuntimeError("model initialization timeout")
        status, _ = self.pipe.recv()
        if status != "INITIALIZED":
            self.stop()
            raise RuntimeError("model initialization failed")

    def transcribe(self, source, timeout_seconds):
        self.pipe.send(str(source))
        if not self.pipe.poll(timeout_seconds):
            self.stop()
            raise TimeoutError("transcription timeout")
        status, result = self.pipe.recv()
        if status == "ERROR":
            raise RuntimeError("transcription failed")
        return status, result

    def stop(self):
        if self.process is not None:
            if self.process.is_alive():
                self.process.terminate()
                self.process.join(10)
                if self.process.is_alive():
                    self.process.kill()
                    self.process.join()
            self.process.close()
            self.process = None
        if self.pipe is not None:
            self.pipe.close()
            self.pipe = None


def run_job(db, database_url, storage, bucket, model, job, timeout_seconds):
    import psycopg

    stopped = threading.Event()
    lost = threading.Event()

    def renew():
        try:
            with psycopg.connect(database_url, autocommit=True, connect_timeout=10) as lease_db:
                while not stopped.wait(15):
                    if not heartbeat(lease_db, job):
                        lost.set()
                        return
        except Exception:
            lost.set()

    thread = threading.Thread(target=renew, daemon=True)
    thread.start()
    uploaded = False
    committed = False
    try:
        with tempfile.TemporaryDirectory(prefix="subtitle-") as temp:
            source = Path(temp) / "source"
            storage.download_file(bucket, job.source_key, str(source))
            if lost.is_set():
                return
            status, text = model.transcribe(source, timeout_seconds)
            if status == "READY":
                if not text or not text.startswith("WEBVTT\n\n"):
                    raise ValueError("invalid track")
                storage.put_object(Bucket=bucket, Key=job.output_key, Body=text.encode("utf-8"), ContentType="text/vtt; charset=utf-8")
                uploaded = True
            if lost.is_set():
                return
            committed = finish(db, job, status)
    except Exception as exc:
        logging.error("subtitle job attempt failed: %s", type(exc).__name__)
        if not lost.is_set():
            try:
                finish(db, job, "RETRY")
            except Exception:
                pass
    finally:
        stopped.set()
        thread.join(20)
        if uploaded and not committed:
            try:
                storage.delete_object(Bucket=bucket, Key=job.output_key)
            except Exception:
                logging.error("could not clean abandoned subtitle output")


def cleanup_failed(db, storage, bucket):
    # Long grace covers a timed-out process or a delayed storage request.
    rows = db.execute("""SELECT id::text, object_key FROM subtitle_runs
        WHERE status='FAILED' AND cleaned_at IS NULL AND updated_at < now()-interval '24 hours' LIMIT 100""").fetchall()
    for run_id, key in rows:
        storage.delete_object(Bucket=bucket, Key=key)
        db.execute("UPDATE subtitle_runs SET cleaned_at=now() WHERE id=%s AND status='FAILED'", (run_id,))


def main():
    import boto3
    import psycopg
    from botocore.config import Config

    logging.basicConfig(level=logging.INFO)
    model_dir = os.environ["SUBTITLE_MODEL_DIR"]
    database_url = os.environ["DATABASE_URL"]
    bucket = os.environ["S3_BUCKET"]
    timeout_seconds = int(os.getenv("SUBTITLE_TIMEOUT_SECONDS", "7200"))
    if timeout_seconds <= 0 or timeout_seconds > 7200:
        raise ValueError("SUBTITLE_TIMEOUT_SECONDS must be 1 to 7200")
    storage = boto3.client("s3", endpoint_url=os.environ["S3_ENDPOINT"],
        aws_access_key_id=os.environ["S3_ACCESS_KEY"], aws_secret_access_key=os.environ["S3_SECRET_KEY"],
        region_name=os.getenv("S3_REGION", "us-east-1"),
        config=Config(s3={"addressing_style": "path"}, connect_timeout=10, read_timeout=30, retries={"max_attempts": 3}))
    model = ModelChild(model_dir)
    try:
        while True:
            try:
                with psycopg.connect(database_url, autocommit=True, connect_timeout=10) as db:
                    if model.process is not None and not model.process.is_alive():
                        model.stop()
                    if model.process is None:
                        model.start()
                    recover_expired(db)
                    cleanup_failed(db, storage, bucket)
                    job = claim(db)
                    if job:
                        run_job(db, database_url, storage, bucket, model, job, timeout_seconds)
                    else:
                        time.sleep(5)
            except Exception as exc:
                logging.error("subtitle worker restarting after %s", type(exc).__name__)
                time.sleep(5)
    except KeyboardInterrupt:
        pass
    finally:
        model.stop()


if __name__ == "__main__":
    multiprocessing.freeze_support()
    main()
