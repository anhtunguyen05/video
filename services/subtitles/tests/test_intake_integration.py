"""Opt-in test: use only a disposable PostgreSQL database and RabbitMQ broker."""
import json
import os
import unittest
from uuid import uuid4

from request_consumer import QUEUE, QUARANTINE, handle_delivery


@unittest.skipUnless(os.getenv("TEST_DATABASE_URL") and os.getenv("TEST_RABBITMQ_URL"),
                     "requires isolated TEST_DATABASE_URL and TEST_RABBITMQ_URL")
class IntakeIntegrationTest(unittest.TestCase):
    def test_committed_job_survives_duplicate_delivery(self):
        import pika
        import psycopg

        job, video, owner = (str(uuid4()) for _ in range(3))
        envelope = {"message_id": job, "message_type": "subtitle.requested.v1", "schema_version": 1,
                    "correlation_id": video, "payload": {"job_id": job, "video_id": video,
                    "owner_id": owner, "source_object_key": f"users/{owner}/videos/{video}/source/source.mp4",
                    "source_version": 1, "language": "vi"}}
        broker = pika.BlockingConnection(pika.URLParameters(os.environ["TEST_RABBITMQ_URL"]))
        self.addCleanup(broker.close)
        channel = broker.channel()
        channel.exchange_declare(exchange="video.events", exchange_type="topic", durable=True)
        channel.queue_declare(queue=QUEUE, durable=True)
        channel.queue_bind(queue=QUEUE, exchange="video.events", routing_key="subtitle.requested.v1")
        channel.queue_declare(queue=QUARANTINE, durable=True)
        channel.confirm_delivery()
        props = pika.BasicProperties(content_type="application/json", delivery_mode=2, message_id=job)
        with psycopg.connect(os.environ["TEST_DATABASE_URL"], autocommit=True) as db:
            try:
                for _ in range(2):
                    channel.basic_publish(exchange="video.events", routing_key="subtitle.requested.v1",
                                          body=json.dumps(envelope).encode(), properties=props, mandatory=True)
                    method, received_properties, body = channel.basic_get(queue=QUEUE, auto_ack=False)
                    self.assertIsNotNone(method, "expected a delivery on isolated broker")
                    handle_delivery(channel, method, received_properties, body, db,
                                    pika.BasicProperties(delivery_mode=2))
                row = db.execute("SELECT count(*) FROM subtitle_jobs WHERE id=%s AND status='QUEUED'", (job,)).fetchone()
                self.assertEqual(row[0], 1)
            finally:
                db.execute("DELETE FROM subtitle_jobs WHERE id=%s", (job,))


if __name__ == "__main__":
    unittest.main()
