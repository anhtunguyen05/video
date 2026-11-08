"""Accept versioned subtitle requests into service-owned durable job storage."""

from __future__ import annotations

import json
import logging
import os
import time
from dataclasses import dataclass
from uuid import UUID

QUEUE = "subtitles.requested.v1"
QUARANTINE = "subtitles.invalid.v1"
MAX_MESSAGE_BYTES = 16384


class InvalidRequest(ValueError):
    pass


@dataclass(frozen=True)
class SubtitleRequest:
    job_id: str
    video_id: str
    owner_id: str
    source_object_key: str
    source_version: int
    language: str
    correlation_id: str
    traceparent: str | None


def _uuid(value: object) -> str:
    if not isinstance(value, str):
        raise InvalidRequest("invalid identifier")
    try:
        canonical = str(UUID(value))
    except ValueError as exc:
        raise InvalidRequest("invalid identifier") from exc
    if canonical != value:
        raise InvalidRequest("identifier must be canonical")
    return canonical


def parse_request(body: bytes) -> SubtitleRequest:
    if len(body) > MAX_MESSAGE_BYTES:
        raise InvalidRequest("message too large")
    try:
        data = json.loads(body)
        payload = data["payload"]
        if data["message_type"] != "subtitle.requested.v1" or type(data["schema_version"]) is not int or data["schema_version"] != 1:
            raise InvalidRequest("unsupported message version")
        job = _uuid(payload["job_id"])
        if _uuid(data["message_id"]) != job:
            raise InvalidRequest("message and job identifiers differ")
        video, owner = _uuid(payload["video_id"]), _uuid(payload["owner_id"])
        source = payload["source_object_key"]
        version = payload["source_version"]
        language = payload["language"]
        correlation = data["correlation_id"]
        traceparent = data.get("traceparent")
        if not isinstance(source, str) or source not in (
            f"users/{owner}/videos/{video}/source/source.mp4",
            f"users/{owner}/videos/{video}/source/source.webm",
        ):
            raise InvalidRequest("invalid source identity")
        if type(version) is not int or version < 1 or version > 2147483647 or language != "vi":
            raise InvalidRequest("invalid request parameters")
        if not isinstance(correlation, str) or not correlation or len(correlation) > 128:
            raise InvalidRequest("invalid correlation identifier")
        if traceparent is not None and (not isinstance(traceparent, str) or len(traceparent) > 256):
            raise InvalidRequest("invalid trace context")
        return SubtitleRequest(job, video, owner, source, version, language, correlation, traceparent)
    except (KeyError, TypeError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise InvalidRequest("malformed request") from exc


def enqueue(connection, request: SubtitleRequest) -> None:
    """Commit before ACK; identical redelivery is harmless, identity clashes fail."""
    with connection.transaction():
        with connection.cursor() as cursor:
            cursor.execute("""
                INSERT INTO subtitle_jobs
                    (id, video_id, owner_id, source_object_key, source_version,
                     language, correlation_id, traceparent)
                VALUES (%s,%s,%s,%s,%s,%s,%s,%s)
                ON CONFLICT DO NOTHING
                RETURNING id
            """, (request.job_id, request.video_id, request.owner_id,
                  request.source_object_key, request.source_version, request.language,
                  request.correlation_id, request.traceparent))
            if cursor.fetchone() is not None:
                return
            cursor.execute("""
                SELECT video_id::text, owner_id::text, source_object_key,
                       source_version, language
                FROM subtitle_jobs WHERE id = %s
            """, (request.job_id,))
            row = cursor.fetchone()
            if row != (request.video_id, request.owner_id, request.source_object_key,
                       request.source_version, request.language):
                raise InvalidRequest("job identity conflict")


def handle_delivery(channel, method, properties, body, db, quarantine_properties) -> None:
    try:
        request = parse_request(body)
        if properties.message_id != request.job_id:
            raise InvalidRequest("broker message identifier mismatch")
        enqueue(db, request)
    except InvalidRequest:
        # Never drop a poison delivery until its copy is confirmed by RabbitMQ.
        channel.basic_publish(exchange="", routing_key=QUARANTINE, body=body,
                              properties=quarantine_properties, mandatory=True)
    channel.basic_ack(delivery_tag=method.delivery_tag)


def main() -> None:
    import pika
    import psycopg

    logging.basicConfig(level=logging.INFO)
    broker_url, database_url = os.environ["RABBITMQ_URL"], os.environ["DATABASE_URL"]
    while True:
        try:
            with psycopg.connect(database_url, autocommit=True) as db:
                parameters = pika.URLParameters(broker_url)
                parameters.blocked_connection_timeout = 30
                parameters.socket_timeout = 10
                connection = pika.BlockingConnection(parameters)
                try:
                    channel = connection.channel()
                    channel.exchange_declare(exchange="video.events", exchange_type="topic", durable=True)
                    channel.queue_declare(queue=QUEUE, durable=True)
                    channel.queue_bind(queue=QUEUE, exchange="video.events", routing_key="subtitle.requested.v1")
                    channel.queue_declare(queue=QUARANTINE, durable=True)
                    channel.confirm_delivery()
                    channel.basic_qos(prefetch_count=1)
                    quarantine_properties = pika.BasicProperties(content_type="application/json", delivery_mode=2)
                    channel.basic_consume(queue=QUEUE, auto_ack=False,
                        on_message_callback=lambda ch, method, properties, body: handle_delivery(
                            ch, method, properties, body, db, quarantine_properties))
                    channel.start_consuming()
                finally:
                    if connection.is_open:
                        connection.close()
        except KeyboardInterrupt:
            return
        except Exception as exc:
            # Avoid logging message bodies, credentials, or full dependency errors.
            logging.error("subtitle intake restarting after %s", type(exc).__name__)
            time.sleep(5)


if __name__ == "__main__":
    main()
