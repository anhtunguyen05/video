import json
import unittest
from contextlib import contextmanager
from types import SimpleNamespace

from request_consumer import InvalidRequest, handle_delivery, parse_request

JOB = "11111111-1111-4111-8111-111111111111"
VIDEO = "22222222-2222-4222-8222-222222222222"
OWNER = "33333333-3333-4333-8333-333333333333"


def message():
    return {
        "message_type": "subtitle.requested.v1", "schema_version": 1,
        "message_id": JOB, "correlation_id": VIDEO, "traceparent": None,
        "payload": {"job_id": JOB, "video_id": VIDEO, "owner_id": OWNER,
                    "source_object_key": f"users/{OWNER}/videos/{VIDEO}/source/source.mp4",
                    "source_version": 1, "language": "vi"},
    }


class FakeDB:
    def __init__(self, operations, existing=None, fail=False):
        self.operations, self.existing, self.fail = operations, existing, fail
        self.query = ""

    @contextmanager
    def transaction(self):
        yield
        self.operations.append("commit")

    def cursor(self):
        return self

    def __enter__(self):
        return self

    def __exit__(self, *args):
        pass

    def execute(self, query, params):
        if self.fail:
            raise RuntimeError("database unavailable")
        self.query = query

    def fetchone(self):
        if "INSERT" in self.query:
            return None if self.existing is not None else (JOB,)
        return self.existing


class FakeChannel:
    def __init__(self, operations, fail_publish=False):
        self.operations, self.fail_publish = operations, fail_publish

    def basic_publish(self, **kwargs):
        if self.fail_publish:
            raise RuntimeError("broker unavailable")
        self.operations.append("quarantine")

    def basic_ack(self, **kwargs):
        self.operations.append("ack")


class IntakeTest(unittest.TestCase):
    def delivery(self, msg, db, channel, message_id=JOB):
        handle_delivery(channel, SimpleNamespace(delivery_tag=1),
                        SimpleNamespace(message_id=message_id), json.dumps(msg).encode(), db, object())

    def test_valid_request_commits_before_ack(self):
        ops = []
        self.delivery(message(), FakeDB(ops), FakeChannel(ops))
        self.assertEqual(ops, ["commit", "ack"])

    def test_duplicate_request_is_acked_after_commit(self):
        ops = []
        existing = (VIDEO, OWNER, message()["payload"]["source_object_key"], 1, "vi")
        self.delivery(message(), FakeDB(ops, existing=existing), FakeChannel(ops))
        self.assertEqual(ops, ["commit", "ack"])

    def test_identity_conflict_is_quarantined(self):
        ops = []
        self.delivery(message(), FakeDB(ops, existing=(VIDEO, OWNER, "other", 1, "vi")), FakeChannel(ops))
        self.assertEqual(ops, ["quarantine", "ack"])

    def test_database_and_quarantine_failures_do_not_ack(self):
        ops = []
        with self.assertRaises(RuntimeError):
            self.delivery(message(), FakeDB(ops, fail=True), FakeChannel(ops))
        self.assertEqual(ops, [])
        with self.assertRaises(RuntimeError):
            self.delivery({}, FakeDB(ops), FakeChannel(ops, fail_publish=True))
        self.assertEqual(ops, [])

    def test_source_identity_and_version_are_validated(self):
        for key, value in (("source_object_key", "other-owner/file"), ("source_version", True), ("language", "en")):
            msg = message()
            msg["payload"][key] = value
            with self.subTest(key=key), self.assertRaises(InvalidRequest):
                parse_request(json.dumps(msg).encode())

    def test_oversized_and_invalid_json_rejected(self):
        for body in (b"x" * 16385, b"{"):
            with self.assertRaises(InvalidRequest):
                parse_request(body)


if __name__ == "__main__":
    unittest.main()
