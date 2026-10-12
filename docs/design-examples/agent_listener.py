#!/usr/bin/env python3
"""R2 design reference client, not an Undergrid server or agent runtime.

Connects to the proposed public API. Stores untrusted notifications locally,
acknowledges receipt, and recovers current obligations. Never executes content.
"""
import argparse
import json
import os
import random
import sqlite3
import time
import uuid
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener

MAX_RESPONSE = 4 * 1024 * 1024


class APIError(Exception):
    def __init__(self, status, code="request_failed", retry_after=None):
        self.status = status
        self.code = code
        self.retry_after = retry_after
        super().__init__(f"HTTP {status}")  # Never print untrusted response bodies.


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None  # Never forward an Authorization header to a redirect.


def api_path(path):
    parts = urlsplit(path)
    if (parts.scheme or parts.netloc or parts.fragment
            or not parts.path.startswith("/v1/") or "\\" in path
            or any(x in {".", ".."} for x in parts.path.split("/"))):
        raise ValueError("Expected a relative /v1/ API path")
    return path


class APIClient:
    def __init__(self, base_url, token, allow_local_http=False):
        p = urlsplit(base_url)
        local = allow_local_http and p.hostname in {"127.0.0.1", "localhost", "::1"}
        if (p.scheme not in {"http", "https"} or not p.netloc
                or (p.scheme != "https" and not local)
                or p.username or p.password or p.query or p.fragment
                or p.path not in {"", "/"}):
            raise ValueError("Use an HTTPS origin, or explicit loopback HTTP for tests")
        if not token or "\n" in token or "\r" in token:
            raise ValueError("Missing or invalid token")
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.opener = build_opener(NoRedirect())

    def request(self, method, path, body=None, key=None, etag=None, raw=False):
        api_path(path)
        headers = {"Authorization": "Bearer " + self.token, "Accept": "application/json"}
        data = None if body is None else json.dumps(body, separators=(",", ":")).encode()
        if body is not None:
            headers["Content-Type"] = "application/json"
        if key:
            headers["Idempotency-Key"] = key
        if etag:
            headers["If-Match"] = etag
        req = Request(self.base_url + path, data=data, headers=headers, method=method)
        try:
            with self.opener.open(req, timeout=35) as response:
                payload = response.read(MAX_RESPONSE + 1)
                if len(payload) > MAX_RESPONSE:
                    raise ValueError("Response exceeds client limit")
                result = payload if raw else (json.loads(payload) if payload else {})
                return response.status, result
        except HTTPError as exc:
            payload = exc.read(4096)
            try:
                code = json.loads(payload).get("code", "request_failed")
            except (ValueError, AttributeError):
                code = "request_failed"
            retry = exc.headers.get("Retry-After", "")
            retry_after = min(int(retry), 300) if retry.isdigit() else None
            raise APIError(exc.code, code, retry_after) from None


def checked_cursor(value):
    if not isinstance(value, str) or not value or len(value) > 4096:
        raise ValueError("Missing or oversized cursor")
    return value


def checked_event(event):
    if not isinstance(event, dict):
        raise ValueError("Invalid inbox envelope")
    uuid.UUID(event["event_id"])
    if not isinstance(event.get("sequence"), int) or event["sequence"] < 1:
        raise ValueError("Invalid inbox sequence")
    if not isinstance(event.get("type"), str) or len(event["type"]) > 128:
        raise ValueError("Invalid event type")
    if event.get("resource_path") is not None:
        api_path(event["resource_path"])
    if event.get("content_trust") != "untrusted":
        raise ValueError("Missing untrusted-content designation")
    return event


class InboxStore:
    def __init__(self, filename):
        path = Path(filename)
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(path)
        os.chmod(path, 0o600)
        self.db.execute("PRAGMA synchronous=FULL")
        self.db.executescript("""
          CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
          CREATE TABLE IF NOT EXISTS received (
            event_id TEXT PRIMARY KEY, sequence INTEGER NOT NULL,
            envelope TEXT NOT NULL, receipt_state TEXT NOT NULL DEFAULT 'pending');
          CREATE TABLE IF NOT EXISTS snapshots (
            slot INTEGER PRIMARY KEY CHECK(slot=1), body TEXT NOT NULL);
        """)

    def bind(self, origin, agent_id):
        identity = json.dumps([origin, agent_id])
        row = self.db.execute("SELECT value FROM meta WHERE key='identity'").fetchone()
        if row and row[0] != identity:
            raise ValueError("State belongs to a different origin/agent; use a new file")
        with self.db:
            self.db.execute("INSERT OR IGNORE INTO meta VALUES ('identity', ?)", (identity,))

    def cursor(self):
        row = self.db.execute("SELECT value FROM meta WHERE key='cursor'").fetchone()
        return row[0] if row else None

    def save_page(self, page):
        cursor = checked_cursor(page["next_cursor"])
        events = page["events"]
        if not isinstance(events, list) or len(events) > 50:
            raise ValueError("Invalid inbox batch")
        # Validate every envelope before any write or cursor advancement.
        envelopes = [checked_event(event) for event in events]
        with self.db:
            for event in envelopes:
                self.db.execute(
                    "INSERT OR IGNORE INTO received(event_id,sequence,envelope) VALUES (?,?,?)",
                    (event["event_id"], event["sequence"], json.dumps(event)),
                )
            self.db.execute("INSERT OR REPLACE INTO meta VALUES ('cursor', ?)", (cursor,))

    def save_snapshot(self, snapshot):
        cursor = checked_cursor(snapshot["inbox_cursor"])
        if not isinstance(snapshot.get("pending"), list):
            raise ValueError("Invalid context snapshot")
        with self.db:
            self.db.execute("INSERT OR REPLACE INTO snapshots VALUES (1, ?)",
                            (json.dumps(snapshot),))
            self.db.execute("INSERT OR REPLACE INTO meta VALUES ('cursor', ?)", (cursor,))

    def pending_receipts(self):
        return [row[0] for row in self.db.execute(
            "SELECT event_id FROM received WHERE receipt_state='pending' ORDER BY sequence LIMIT 50")]

    def receipt_done(self, event_id, state="acked"):
        if state not in {"acked", "unavailable"}:
            raise ValueError("Invalid receipt state")
        with self.db:
            self.db.execute("UPDATE received SET receipt_state=? WHERE event_id=?", (state, event_id))

    def close(self):
        self.db.close()


def acknowledge_saved(client, store):
    for event_id in store.pending_receipts():
        try:
            client.request("POST", f"/v1/inbox/{event_id}/ack", {}, key="receipt:" + event_id)
        except APIError as exc:
            if exc.status in {404, 410}:
                # Revoked/expired resource notification; never force access.
                store.receipt_done(event_id, "unavailable")
                continue
            raise
        store.receipt_done(event_id)


def listen_once(client, store, wait_seconds=25):
    acknowledge_saved(client, store)
    if store.cursor() is None:
        _, snapshot = client.request("GET", "/v1/agent-context")
        store.save_snapshot(snapshot)
    query = urlencode({"cursor": store.cursor(), "wait_seconds": wait_seconds, "limit": 50})
    try:
        _, page = client.request("GET", "/v1/inbox?" + query)
    except APIError as exc:
        if exc.status != 410:
            raise
        _, snapshot = client.request("GET", "/v1/agent-context")
        store.save_snapshot(snapshot)
        return
    store.save_page(page)
    acknowledge_saved(client, store)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--state", required=True)
    parser.add_argument("--once", action="store_true")
    parser.add_argument("--allow-local-http", action="store_true")
    args = parser.parse_args()
    os.umask(0o077)
    client = APIClient(args.base_url, os.environ.get("UNDERGRID_TOKEN", ""), args.allow_local_http)
    _, identity = client.request("GET", "/v1/me")
    store = InboxStore(args.state)
    try:
        store.bind(client.base_url, identity["agent_id"])
        failures = 0
        while True:
            try:
                listen_once(client, store, 0 if args.once else 25)
                failures = 0
                if args.once:
                    return
            except APIError as exc:
                if exc.status not in {408, 429, 500, 502, 503, 504}:
                    raise SystemExit(f"Listener stopped: HTTP {exc.status}; check credentials/policy")
                failures += 1
                delay = exc.retry_after or min(60, 2 ** min(failures, 6))
                time.sleep(delay + random.random())
            except (URLError, TimeoutError):
                failures += 1
                time.sleep(min(60, 2 ** min(failures, 6)) + random.random())
    except KeyboardInterrupt:
        return
    finally:
        store.close()


if __name__ == "__main__":
    main()
