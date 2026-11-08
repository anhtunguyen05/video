"""Run only against an isolated database with migrations through 11."""
import os
import unittest
from uuid import uuid4

from job_store import claim, finish, heartbeat, recover_expired
from worker import run_job
from result_publisher import publish_one, EXCHANGE, QUEUE, ROUTING


@unittest.skipUnless(os.getenv("TEST_DATABASE_URL"), "requires isolated TEST_DATABASE_URL")
class JobStoreIntegrationTest(unittest.TestCase):
    def test_worker_with_fake_asr_stores_track_and_result(self):
        import psycopg
        job, video, owner = (str(uuid4()) for _ in range(3))

        class Storage:
            def __init__(self): self.objects = {}
            def download_file(self, bucket, key, path):
                with open(path, "wb") as output: output.write(b"fake video")
            def put_object(self, Bucket, Key, Body, ContentType): self.objects[Key] = Body
            def delete_object(self, Bucket, Key): self.objects.pop(Key, None)

        class Model:
            def transcribe(self, source, timeout): return "READY", "WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nXin chào\n"

        url = os.environ["TEST_DATABASE_URL"]
        with psycopg.connect(url, autocommit=True) as db:
            db.execute("""INSERT INTO subtitle_jobs (id, video_id, owner_id, source_object_key,
                       source_version, language, correlation_id) VALUES (%s,%s,%s,'source',1,'vi',%s)""",
                       (job, video, owner, video))
            try:
                active = claim(db)
                storage = Storage()
                run_job(db, url, storage, "test", Model(), active, 30)
                self.assertIn("Xin chào", storage.objects[active.output_key].decode())
                self.assertEqual(db.execute("SELECT status FROM subtitle_jobs WHERE id=%s", (job,)).fetchone()[0], "SUCCEEDED")
                self.assertEqual(db.execute("SELECT payload->'payload'->>'status' FROM subtitle_outbox WHERE job_id=%s", (job,)).fetchone()[0], "READY")
                broker_url = os.getenv("TEST_RABBITMQ_URL")
                if broker_url:
                    import pika
                    broker = pika.BlockingConnection(pika.URLParameters(broker_url))
                    try:
                        channel = broker.channel()
                        channel.exchange_declare(exchange=EXCHANGE, exchange_type="topic", durable=True)
                        channel.queue_declare(queue=QUEUE, durable=True)
                        channel.queue_bind(queue=QUEUE, exchange=EXCHANGE, routing_key=ROUTING)
                        channel.confirm_delivery()
                        self.assertTrue(publish_one(db, channel, lambda message_id: pika.BasicProperties(delivery_mode=2, message_id=message_id)))
                        method, properties, _ = channel.basic_get(queue=QUEUE, auto_ack=False)
                        self.assertIsNotNone(method)
                        channel.basic_ack(delivery_tag=method.delivery_tag)
                        self.assertTrue(db.execute("SELECT published_at IS NOT NULL FROM subtitle_outbox WHERE job_id=%s", (job,)).fetchone()[0])
                    finally:
                        broker.close()
            finally:
                db.execute("DELETE FROM subtitle_outbox WHERE job_id=%s", (job,))
                db.execute("DELETE FROM subtitle_runs WHERE job_id=%s", (job,))
                db.execute("DELETE FROM subtitle_jobs WHERE id=%s", (job,))

    def test_expired_run_is_fenced_and_one_result_is_committed(self):
        import psycopg
        job, video, owner = (str(uuid4()) for _ in range(3))
        with psycopg.connect(os.environ["TEST_DATABASE_URL"], autocommit=True) as db:
            db.execute("""INSERT INTO subtitle_jobs (id, video_id, owner_id, source_object_key,
                       source_version, language, correlation_id) VALUES (%s,%s,%s,'source',1,'vi',%s)""",
                       (job, video, owner, video))
            try:
                first = claim(db)
                self.assertEqual(first.id, job)
                self.assertTrue(heartbeat(db, first))
                db.execute("UPDATE subtitle_jobs SET lease_until=now()-interval '1 second' WHERE id=%s", (job,))
                self.assertFalse(finish(db, first, "READY"))
                self.assertTrue(recover_expired(db))
                second = claim(db)
                self.assertEqual(second.attempt, 2)
                self.assertFalse(finish(db, first, "READY"))
                self.assertTrue(finish(db, second, "READY"))
                self.assertFalse(finish(db, second, "READY"))
                result = db.execute("SELECT payload->'payload'->>'status', payload->'payload'->>'output_object_key' FROM subtitle_outbox WHERE job_id=%s", (job,)).fetchall()
                self.assertEqual(result, [("READY", second.output_key)])
            finally:
                db.execute("DELETE FROM subtitle_outbox WHERE job_id=%s", (job,))
                db.execute("DELETE FROM subtitle_runs WHERE job_id=%s", (job,))
                db.execute("DELETE FROM subtitle_jobs WHERE id=%s", (job,))


if __name__ == "__main__":
    unittest.main()
