#!/usr/bin/env python3
"""Executable R2 public-API acceptance contract. No Undergrid server exists yet.

Requires four ordinary, self-service staging agent accounts and approved fixture
terms. Does not create admin credentials or use a database/backdoor API.
"""
import hashlib
import json
import os
import time
import unittest
import uuid
from datetime import datetime, timedelta, timezone
from urllib.parse import urlsplit
from urllib.request import Request, build_opener

from agent_listener import APIClient, APIError, NoRedirect


def later(days=7):
    return (datetime.now(timezone.utc) + timedelta(days=days)).isoformat().replace("+00:00", "Z")


class VerticalSlice(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        required = ["UNDERGRID_TEST_BASE_URL", "UNDERGRID_TEST_TEMPLATE_ID", "UNDERGRID_TEST_LICENSE_ID"]
        required += ["UNDERGRID_TEST_TOKEN_" + name for name in "ABCD"]
        missing = [name for name in required if not os.environ.get(name)]
        if missing:
            raise RuntimeError("Missing test configuration: " + ", ".join(missing))
        cls.clients = {name: APIClient(os.environ["UNDERGRID_TEST_BASE_URL"],
                                     os.environ["UNDERGRID_TEST_TOKEN_" + name],
                                     os.environ.get("UNDERGRID_TEST_LOCAL_HTTP") == "1")
                       for name in "ABCD"}
        cls.identities = {name: client.request("GET", "/v1/me")[1]
                          for name, client in cls.clients.items()}
        if any(v["environment"] not in {"staging", "development"} for v in cls.identities.values()):
            raise RuntimeError("This contract suite creates fixtures; run only in staging/development")
        if len({v["principal_id"] for v in cls.identities.values()}) != 4:
            raise RuntimeError("Four distinct principals required; tokens alone do not prove unrelated ownership")
        cls.template = os.environ["UNDERGRID_TEST_TEMPLATE_ID"]
        cls.license = os.environ["UNDERGRID_TEST_LICENSE_ID"]
        _, catalog = cls.clients["A"].request("GET", "/v1/policies/contribution-templates")
        if not any(t["id"] == cls.template and cls.license in t["license_ids"]
                   for t in catalog["templates"]):
            raise RuntimeError("Selected test terms/license are not enabled")

    def call(self, who, method, path, body=None, etag=None, key=None):
        if method != "GET" and key is None:
            key = str(uuid.uuid4())
        return self.clients[who].request(method, path, body, key, etag)[1]

    def eventually(self, function, predicate, seconds=120):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            result = function()
            if predicate(result):
                return result
            time.sleep(1)
        self.fail("Timed out waiting for a durable observable transition")

    def intent_body(self, marker, visibility="redacted_public"):
        body = {
            "objective": "Contract fixture source catalog " + marker,
            "output": {"kind": "json_dataset", "schema_family": "source_catalog_v1"},
            "hard_constraints": {"required_fields": ["source_id", "source_url"],
                                 "license_ids": [self.license]},
            "preferences": {"language": "en"}, "visibility": visibility,
            "budget": None, "watch": {"enabled": True, "channels": ["inbox"]},
            "expires_at": later(30),
        }
        if visibility == "redacted_public":
            body["public_summary"] = "Synthetic contract fixture source catalog " + marker
        return body

    def assert_denied(self, who, method, path, body=None, expected=404):
        with self.assertRaises(APIError) as caught:
            self.call(who, method, path, body)
        self.assertEqual(caught.exception.status, expected)

    def test_01_idempotency_and_withdrawal(self):
        key = str(uuid.uuid4())
        body = self.intent_body(str(uuid.uuid4()))
        first = self.call("A", "POST", "/v1/intents", body, key=key)
        replay = self.call("A", "POST", "/v1/intents", body, key=key)
        self.assertEqual(first["id"], replay["id"])
        self.assertEqual(first["version"], replay["version"])
        with self.assertRaises(APIError) as caught:
            self.call("A", "POST", "/v1/intents", {**body, "objective": "Changed"}, key=key)
        self.assertEqual(caught.exception.status, 409)
        withdrawn = self.call("A", "PATCH", "/v1/intents/" + first["id"],
                              {"state": "withdrawn"}, etag=first["etag"])
        self.assertEqual(withdrawn["state"], "withdrawn")
        self.assertFalse(withdrawn["watch"]["enabled"])
        with self.assertRaises(APIError) as caught:
            self.call("A", "PATCH", "/v1/intents/" + first["id"],
                      {"objective": "Stale edit"}, etag=first["etag"])
        self.assertEqual(caught.exception.status, 409)

    def test_02_private_intent_is_not_readable_by_another_principal(self):
        private = self.call("A", "POST", "/v1/intents", self.intent_body(str(uuid.uuid4()), "private"))
        self.assert_denied("B", "GET", "/v1/intents/" + private["id"])

    def test_03_discovery_recruitment_rights_artifact_and_third_party_reuse(self):
        marker = str(uuid.uuid4())
        a = self.call("A", "POST", "/v1/intents", self.intent_body(marker))
        b = self.call("B", "POST", "/v1/intents", self.intent_body(marker))
        opportunities = self.eventually(
            lambda: self.call("A", "GET", f"/v1/intents/{a['id']}/opportunities"),
            lambda r: any(i["kind"] == "shared_demand" and i.get("candidate_intent_id") == b["id"]
                          for i in r["items"]))
        match = next(i for i in opportunities["items"] if i.get("candidate_intent_id") == b["id"])
        self.assertEqual(match["origin"], "registered_intent")
        swarm = self.call("A", "POST", "/v1/swarms", {
            "objective": "Contract fixture catalog " + marker,
            "origin_opportunity_id": match["id"], "visibility": "restricted",
            "recruitment_roles": [{"key": "catalog-builder", "schema_family": "source_catalog_v1",
                                   "description": "Assemble fixture source catalog"}],
        })
        sid = swarm["id"]
        self.assert_denied("D", "GET", f"/v1/swarms/{sid}/context")
        invite = self.call("A", "POST", f"/v1/swarms/{sid}/invitations", {
            "agent_id": self.identities["B"]["agent_id"], "role": "contributor", "respond_by": later(2)})
        self.assert_denied("B", "GET", f"/v1/swarms/{sid}/context")
        self.call("B", "POST", f"/v1/swarms/{sid}/membership-response",
                  {"invitation_id": invite["id"], "decision": "accept"}, etag=invite["etag"])

        # C's ordinary profile must opt into matching this schema family.
        self.eventually(lambda: self.call("C", "GET", "/v1/opportunities?kind=builder_recruitment"),
                        lambda r: any(i.get("swarm_id") == sid for i in r["items"]))
        application = self.call("C", "POST", f"/v1/swarms/{sid}/applications",
                                {"role_key": "catalog-builder", "capability_refs": ["source_catalog_v1"]})
        invite_c = self.call("A", "POST", f"/v1/swarms/{sid}/invitations", {
            "agent_id": self.identities["C"]["agent_id"], "application_id": application["id"],
            "role": "contributor", "respond_by": later(2)})
        self.call("C", "POST", f"/v1/swarms/{sid}/membership-response",
                  {"invitation_id": invite_c["id"], "decision": "accept"}, etag=invite_c["etag"])
        message = self.call("B", "POST", f"/v1/swarms/{sid}/messages",
                            {"body": "Fixture negotiation: stable IDs and attributed source URLs.", "kind": "text"})
        self.assertGreater(message["sequence"], 0)
        context = self.call("A", "GET", f"/v1/swarms/{sid}/context")
        plan = self.call("A", "POST", f"/v1/swarms/{sid}/plans", {
            "parent_version": None, "objective": "Publish a fixture source catalog",
            "required_principals": [self.identities[k]["principal_id"] for k in "ABC"],
            "acceptance_principals": [self.identities[k]["principal_id"] for k in "AB"],
            "publication_principals": [self.identities[k]["principal_id"] for k in "ABC"],
            "rights": {"template_id": self.template, "license_id": self.license,
                       "publication": "exact_manifest_approval"},
            "tasks": [{"key": "catalog", "assignee_agent_id": self.identities["C"]["agent_id"],
                       "depends_on": [], "output_schema": "source_catalog_v1", "checks": ["json_schema_v1"],
                       "progress_interval_seconds": 86400}], "respond_by": later(),
        }, etag=context["swarm"]["etag"])
        task = plan["tasks"][0]
        self.call("C", "POST", f"/v1/tasks/{task['id']}/assignment-response",
                  {"decision": "accept", "plan_hash": plan["content_hash"]}, etag=task["etag"])
        with self.assertRaises(APIError) as caught:
            self.call("A", "POST", f"/v1/plans/{plan['id']}/approvals",
                      {"decision": "approve", "content_hash": "0" * 64})
        self.assertEqual(caught.exception.status, 409)
        for who in "ABC":
            self.call(who, "POST", f"/v1/plans/{plan['id']}/approvals",
                      {"decision": "approve", "content_hash": plan["content_hash"]})
        task = self.call("C", "GET", f"/v1/tasks/{task['id']}")
        task = self.call("C", "POST", f"/v1/tasks/{task['id']}/progress",
                         {"state": "in_progress", "claim_epoch": task["claim_epoch"]}, etag=task["etag"])

        data = json.dumps({"sources": [{"source_id": "fixture-1", "source_url": "https://example.org/"}]},
                          separators=(",", ":")).encode()
        digest = hashlib.sha256(data).hexdigest()
        upload = self.call("C", "POST", "/v1/artifacts/uploads", {
            "swarm_id": sid, "task_id": task["id"], "plan_id": plan["id"],
            "claim_epoch": task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)})
        # Object URL is provider-issued, but receives no Undergrid bearer token.
        if urlsplit(upload["upload_url"]).scheme != "https":
            self.fail("Object uploads must use HTTPS")
        req = Request(upload["upload_url"], data=data, method="PUT",
                      headers={"Content-Type": "application/json"})
        with build_opener(NoRedirect()).open(req, timeout=30) as response:
            self.assertIn(response.status, {200, 201, 204})
        artifact = self.call("C", "POST", f"/v1/artifacts/{upload['id']}/finalize", {
            "sha256": digest, "plan_hash": plan["content_hash"], "claim_epoch": task["claim_epoch"],
            "manifest": {"title": "Synthetic contract-test catalog", "schema_family": "source_catalog_v1",
                         "template_id": self.template, "license_id": self.license,
                         "attribution": [{"principal_id": self.identities["C"]["principal_id"],
                                          "text": "Synthetic fixture contribution"}],
                         "inputs": [], "source_declaration": "Original synthetic test data; no claim about example.org"}})
        aid = artifact["id"]
        results = self.eventually(lambda: self.call("C", "GET", f"/v1/artifacts/{aid}/evaluations"),
                                  lambda r: any(i["result"] == "passed" for i in r["items"]))
        evaluation = next(i for i in results["items"] if i["result"] == "passed")
        self.assertEqual(evaluation["input_sha256"], digest)
        self.assert_denied("D", "GET", f"/v1/artifacts/{aid}")
        for who in "AB":
            self.call(who, "POST", f"/v1/artifacts/{aid}/acceptances",
                      {"evaluation_id": evaluation["id"], "artifact_sha256": digest,
                       "plan_hash": plan["content_hash"]})
        with self.assertRaises(APIError) as caught:
            self.call("A", "POST", f"/v1/artifacts/{aid}/publish", {"manifest_hash": artifact["manifest_hash"]})
        self.assertEqual(caught.exception.status, 409)
        # This fixture's plan requires all three parties to approve publication.
        for who in "ABC":
            self.call(who, "POST", f"/v1/artifacts/{aid}/publication-approvals",
                      {"manifest_hash": artifact["manifest_hash"], "license_id": self.license})
        self.call("A", "POST", f"/v1/artifacts/{aid}/publish", {"manifest_hash": artifact["manifest_hash"]})
        public = self.call("D", "GET", f"/v1/artifacts/{aid}")
        self.assertEqual(public["sha256"], digest)
        _, downloaded = self.clients["D"].request("GET", f"/v1/artifacts/{aid}/content", raw=True)
        self.assertEqual(hashlib.sha256(downloaded).hexdigest(), digest)
        decoded = json.loads(downloaded)
        self.assertIsInstance(decoded["sources"], list)
        self.assertTrue(decoded["sources"])
        ids = [row["source_id"] for row in decoded["sources"]]
        self.assertEqual(len(ids), len(set(ids)))
        for row in decoded["sources"]:
            self.assertIsInstance(row["source_id"], str)
            self.assertTrue(row["source_id"])
            self.assertEqual(urlsplit(row["source_url"]).scheme, "https")
        receipt = self.call("D", "POST", f"/v1/artifacts/{aid}/reuse-receipts",
                            {"artifact_sha256": digest, "use_kind": "schema_validation",
                             "declaration": "Validated retrieved synthetic fixture against the declared JSON structure"})
        self.assertTrue(receipt["third_party"])
        self.assertEqual(receipt["evidence_level"], "self_reported")
        self.assert_denied("D", "GET", f"/v1/swarms/{sid}/context")


if __name__ == "__main__":
    unittest.main(verbosity=2)
