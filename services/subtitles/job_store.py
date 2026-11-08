"""Subtitle-owned job leases and durable final-result events."""

import json
from dataclasses import dataclass
from uuid import uuid4


@dataclass(frozen=True)
class ClaimedJob:
    id: str
    video_id: str
    owner_id: str
    source_key: str
    source_version: int
    correlation_id: str
    attempt: int
    run_id: str
    output_key: str


def _result_event(db, job_id, video_id, owner_id, source_version, correlation_id, status, output_key):
    event_id = str(uuid4())
    event = {
        "message_id": event_id, "message_type": "subtitle.completed.v1", "schema_version": 1,
        "correlation_id": correlation_id,
        "payload": {"job_id": job_id, "video_id": video_id, "owner_id": owner_id,
                    "source_version": source_version, "language": "vi", "status": status,
                    "output_object_key": output_key},
    }
    db.execute("INSERT INTO subtitle_outbox (id, job_id, payload) VALUES (%s,%s,%s::jsonb)",
               (event_id, job_id, json.dumps(event)))


def recover_expired(db, max_attempts=4):
    """Requeue one expired run, or emit one terminal failure after the last attempt."""
    with db.transaction():
        row = db.execute("""
            SELECT id::text, video_id::text, owner_id::text, source_version,
                   correlation_id, attempt, run_id::text
            FROM subtitle_jobs WHERE status='RUNNING' AND lease_until <= now()
            ORDER BY lease_until FOR UPDATE SKIP LOCKED LIMIT 1
        """).fetchone()
        if row is None:
            return False
        job, video, owner, version, correlation, attempt, run = row
        db.execute("UPDATE subtitle_runs SET status='FAILED', updated_at=now() WHERE id=%s AND status='RUNNING'", (run,))
        if attempt >= max_attempts:
            db.execute("UPDATE subtitle_jobs SET status='FAILED', lease_until=NULL, updated_at=now() WHERE id=%s", (job,))
            _result_event(db, job, video, owner, version, correlation, "FAILED", None)
        else:
            db.execute("UPDATE subtitle_jobs SET status='QUEUED', lease_until=NULL, next_retry_at=now(), updated_at=now() WHERE id=%s", (job,))
        return True


def claim(db, lease_seconds=90):
    run = str(uuid4())
    with db.transaction():
        row = db.execute("""
            UPDATE subtitle_jobs SET status='RUNNING', attempt=attempt+1,
                run_id=%s, lease_until=now() + make_interval(secs => %s), updated_at=now()
            WHERE id = (SELECT id FROM subtitle_jobs WHERE status='QUEUED' AND next_retry_at <= now()
                        ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
            RETURNING id::text, video_id::text, owner_id::text, source_object_key,
                      source_version, correlation_id, attempt
        """, (run, lease_seconds)).fetchone()
        if row is None:
            return None
        job, video, owner, source, version, correlation, attempt = row
        output = f"users/{owner}/videos/{video}/subtitles/v{version}/runs/{run}/vi.vtt"
        db.execute("INSERT INTO subtitle_runs (id, job_id, object_key, status) VALUES (%s,%s,%s,'RUNNING')", (run, job, output))
        return ClaimedJob(job, video, owner, source, version, correlation, attempt, run, output)


def heartbeat(db, job, lease_seconds=90):
    row = db.execute("""
        UPDATE subtitle_jobs SET lease_until=now()+make_interval(secs => %s), updated_at=now()
        WHERE id=%s AND status='RUNNING' AND run_id=%s AND lease_until > now() RETURNING id
    """, (lease_seconds, job.id, job.run_id)).fetchone()
    return row is not None


def finish(db, job, status, max_attempts=4):
    """Fence stale runs and atomically commit state plus an eventual result."""
    if status not in ("READY", "NO_AUDIO", "NO_SPEECH", "RETRY", "FAILED"):
        raise ValueError("invalid result status")
    with db.transaction():
        row = db.execute("""
            SELECT id FROM subtitle_jobs WHERE id=%s AND status='RUNNING' AND run_id=%s
                AND lease_until > now() FOR UPDATE
        """, (job.id, job.run_id)).fetchone()
        if row is None:
            return False
        if status == "RETRY" and job.attempt < max_attempts:
            db.execute("UPDATE subtitle_jobs SET status='QUEUED', lease_until=NULL, next_retry_at=now()+interval '30 seconds', updated_at=now() WHERE id=%s", (job.id,))
            db.execute("UPDATE subtitle_runs SET status='FAILED', updated_at=now() WHERE id=%s", (job.run_id,))
            return True
        if status == "RETRY":
            status = "FAILED"
        succeeded = status in ("READY", "NO_AUDIO", "NO_SPEECH")
        db.execute("UPDATE subtitle_jobs SET status=%s, lease_until=NULL, updated_at=now() WHERE id=%s", ("SUCCEEDED" if succeeded else "FAILED", job.id))
        db.execute("UPDATE subtitle_runs SET status=%s, updated_at=now() WHERE id=%s", ("SUCCEEDED" if status == "READY" else "FAILED", job.run_id))
        _result_event(db, job.id, job.video_id, job.owner_id, job.source_version,
                      job.correlation_id, status, job.output_key if status == "READY" else None)
        return True
