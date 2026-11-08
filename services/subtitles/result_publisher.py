"""Publish service-owned result outbox with confirms; delivery is at least once."""

import json
import logging
import os
import time

EXCHANGE = "subtitle.events"
QUEUE = "media.subtitle-completed.v1"
ROUTING = "subtitle.completed.v1"


def publish_one(db, channel, properties_factory):
    with db.transaction():
        row = db.execute("""SELECT id::text, payload FROM subtitle_outbox
            WHERE published_at IS NULL ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1""").fetchone()
        if row is None:
            return False
        event_id, payload = row
        channel.basic_publish(exchange=EXCHANGE, routing_key=ROUTING,
                              body=json.dumps(payload).encode("utf-8"),
                              properties=properties_factory(event_id), mandatory=True)
        db.execute("UPDATE subtitle_outbox SET published_at=now() WHERE id=%s", (event_id,))
        return True


def main():
    import pika
    import psycopg

    logging.basicConfig(level=logging.INFO)
    while True:
        try:
            with psycopg.connect(os.environ["DATABASE_URL"], autocommit=True) as db:
                parameters = pika.URLParameters(os.environ["RABBITMQ_URL"])
                parameters.blocked_connection_timeout = 30
                parameters.socket_timeout = 10
                broker = pika.BlockingConnection(parameters)
                try:
                    channel = broker.channel()
                    channel.exchange_declare(exchange=EXCHANGE, exchange_type="topic", durable=True)
                    channel.queue_declare(queue=QUEUE, durable=True)
                    channel.queue_bind(queue=QUEUE, exchange=EXCHANGE, routing_key=ROUTING)
                    channel.confirm_delivery()
                    while True:
                        if not publish_one(db, channel, lambda message_id: pika.BasicProperties(
                                content_type="application/json", delivery_mode=2, message_id=message_id)):
                            broker.process_data_events(time_limit=2)
                finally:
                    if broker.is_open:
                        broker.close()
        except KeyboardInterrupt:
            return
        except Exception as exc:
            logging.error("subtitle publisher restarting after %s", type(exc).__name__)
            time.sleep(5)


if __name__ == "__main__":
    main()
