"""Local tests for the design-reference listener, not Undergrid acceptance."""
import json
import tempfile
import unittest
import uuid
from pathlib import Path

from agent_listener import APIError, InboxStore, NoRedirect, api_path, listen_once


def event():
    return {"event_id": str(uuid.uuid4()), "sequence": 1, "type": "opportunity.available",
            "resource_path": "/v1/opportunities/" + str(uuid.uuid4()), "content_trust": "untrusted"}


class FakePublicAPI:
    def __init__(self, item, lose_ack=False, expire=False):
        self.item = item
        self.lose_ack = lose_ack
        self.expire = expire
        self.calls = []
        self.effects = set()

    def request(self, method, path, body=None, key=None, **kwargs):
        self.calls.append((method, path))
        if path == "/v1/agent-context":
            return 200, {"inbox_cursor": "snapshot-cursor", "pending": [], "snapshot_id": str(uuid.uuid4())}
        if path.startswith("/v1/inbox?"):
            if self.expire:
                self.expire = False
                raise APIError(410, "cursor_expired")
            return 200, {"events": [self.item], "next_cursor": "page-cursor"}
        if method == "POST" and path.endswith("/ack"):
            self.effects.add(key)
            if self.lose_ack:
                self.lose_ack = False
                raise APIError(503)  # Remote receipt committed; response lost.
            return 200, {"receipt": "acked"}
        raise AssertionError("Listener attempted an unsupported action")


class ListenerReferenceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.path = Path(self.tmp.name) / "inbox.sqlite"
        self.store = InboxStore(self.path)

    def tearDown(self):
        self.store.close()
        self.tmp.cleanup()

    def test_duplicate_page_has_one_row_and_cursor(self):
        page = {"events": [event()], "next_cursor": "one"}
        self.store.save_page(page)
        self.store.save_page(page)
        self.assertEqual(self.store.db.execute("SELECT count(*) FROM received").fetchone()[0], 1)
        self.assertEqual(self.store.cursor(), "one")

    def test_invalid_batch_never_advances_cursor_or_partially_saves(self):
        self.store.save_page({"events": [], "next_cursor": "before"})
        bad = event()
        bad["resource_path"] = "https://attacker.example/collect"
        with self.assertRaises(ValueError):
            self.store.save_page({"events": [event(), bad], "next_cursor": "after"})
        self.assertEqual(self.store.cursor(), "before")
        self.assertEqual(self.store.pending_receipts(), [])

    def test_restart_after_lost_ack_preserves_receipt_and_idempotency(self):
        api = FakePublicAPI(event(), lose_ack=True)
        with self.assertRaises(APIError):
            listen_once(api, self.store, 0)
        self.assertEqual(self.store.cursor(), "page-cursor")
        self.assertEqual(len(self.store.pending_receipts()), 1)
        self.store.close()
        self.store = InboxStore(self.path)
        listen_once(api, self.store, 0)
        self.assertEqual(len(api.effects), 1)
        self.assertEqual(self.store.pending_receipts(), [])
        self.assertTrue(all(method == "GET" or path.endswith("/ack") for method, path in api.calls))

    def test_expired_cursor_recovers_snapshot_without_join_or_execution(self):
        self.store.save_page({"events": [], "next_cursor": "old-epoch"})
        api = FakePublicAPI(event(), expire=True)
        listen_once(api, self.store, 0)
        self.assertEqual(self.store.cursor(), "snapshot-cursor")
        snapshot = json.loads(self.store.db.execute("SELECT body FROM snapshots").fetchone()[0])
        self.assertEqual(snapshot["pending"], [])
        self.assertFalse(api.effects)

    def test_agent_origin_binding(self):
        self.store.bind("https://network.example", "agent-a")
        self.store.bind("https://network.example", "agent-a")
        with self.assertRaises(ValueError):
            self.store.bind("https://sandbox.example", "agent-a")
        with self.assertRaises(ValueError):
            self.store.bind("https://network.example", "agent-b")

    def test_off_origin_paths_and_redirects_rejected(self):
        for path in ["https://evil.example/v1/me", "//evil.example/v1/me", "/v1/../secret", "/admin"]:
            with self.assertRaises(ValueError):
                api_path(path)
        self.assertIsNone(NoRedirect().redirect_request(None, None, 302, "", {}, "https://evil.example"))


if __name__ == "__main__":
    unittest.main(verbosity=2)
