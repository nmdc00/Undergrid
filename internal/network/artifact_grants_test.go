package network

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPrivateArtifactInputGrantsRequireExplicitScope(t *testing.T) {
	x := newArtifactScenario(t)
	data := []byte(`{"sources":[{"source_id":"private","source_url":"https://example.org/"}]}`)
	uploaded := x.upload(t, data, "private-input")
	code, source := x.actors[2].call("POST", "/v1/artifacts/"+uploaded["id"].(string)+"/finalize", "seal", x.finalBody(data))
	if code != 200 {
		t.Fatal(code, source)
	}
	code, out := x.actors[2].call("POST", "/v1/artifacts/"+source["id"].(string)+"/input-grants", "grant", map[string]any{})
	if code != 403 || out["error"].(map[string]any)["code"] != "insufficient_scope" {
		t.Fatalf("private input grant scope boundary missing: %d %v", code, out)
	}
}

type privateGrantScenario struct {
	source      artifactScenario
	target      artifactScenario
	input       map[string]any
	inputData   []byte
	scope       InputGrantScope
	coordinator testClient
}

func newPrivateGrantScenario(t *testing.T, survival bool) privateGrantScenario {
	return newPrivateGrantScenarioWithSourcePurpose(t, survival, "output")
}

func newPrivateGrantScenarioWithSourcePurpose(t *testing.T, survival bool, sourcePurpose string) privateGrantScenario {
	t.Helper()
	x := newArtifactScenario(t)
	template := x.s.templates["test-contribution-v1"]
	template.InputGrantPurposes = []string{"assembly", "evaluation"}
	template.SubmittedGrantSurvivesWithdrawal = survival
	if err := x.s.SetContributionTemplates([]ContributionTemplate{template}); err != nil {
		t.Fatal(err)
	}
	roomPath := "/v1/swarms/" + x.room["id"].(string)
	oldActors := append([]testClient(nil), x.actors...)
	agents := []string{}
	for i, c := range x.actors {
		scopes := []string{"intents:read", "inbox:read", "swarms:read", "swarms:create", "members:invite", "members:respond", "members:withdraw", "plans:propose", "plans:approve", "rights:accept", "tasks:accept", "tasks:progress", "artifacts:read", "artifacts:submit", "artifacts:accept", "artifacts:reuse", "publication:approve", "artifacts:grant"}
		code, out := c.call("POST", "/v1/agents", "scoped-input-agent-"+x.principals[i], map[string]any{"name": "scoped input agent", "scopes": scopes, "profile": map[string]any{"accept_invitations": true}})
		if code != 201 {
			t.Fatal(code, out)
		}
		c.token = out["token"].(string)
		x.actors[i] = c
		agents = append(agents, out["id"].(string))
		if i < 3 {
			code, invite := oldActors[0].call("POST", roomPath+"/invitations", "input-agent-"+agents[i], map[string]any{"agent_id": agents[i], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
			if code != 201 {
				t.Fatal(code, invite)
			}
			code, out = c.versioned("POST", roomPath+"/membership-response", "join", invite["etag"].(string), map[string]any{"invitation_id": invite["id"], "decision": "accept"})
			if code != 200 {
				t.Fatal(code, out)
			}
			if sourcePurpose == "progress" && i > 0 {
				code, out = oldActors[i].call("POST", roomPath+"/withdraw", "withdraw-old-progress-agent", map[string]any{})
				if code != 200 {
					t.Fatal("replace duplicate membership before progress audience fixture", code, out)
				}
			}
		}
	}
	_, snapshot := x.actors[0].call("GET", roomPath+"/context", "", nil)
	spec := x.plan["specification"].(map[string]any)
	spec["parent_version"] = 1
	spec["respond_by"] = time.Now().Add(time.Hour)
	spec["tasks"].([]any)[0].(map[string]any)["assignee_agent_id"] = agents[2]
	if sourcePurpose == "progress" {
		spec["acceptance_principals"] = []string{x.principals[1], x.principals[2]}
		spec["publication_principals"] = []string{x.principals[1], x.principals[2]}
		spec["tasks"] = append(spec["tasks"].([]any), map[string]any{
			"key": "in-audience-recipient", "assignee_agent_id": agents[1], "depends_on": []any{},
			"output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400,
		})
	}
	code, plan := x.actors[0].versioned("POST", roomPath+"/plans", "input-plan", snapshot["swarm"].(map[string]any)["etag"].(string), spec)
	if code != 201 {
		t.Fatal(code, plan)
	}
	x.plan = plan
	x.task = plan["tasks"].([]any)[0].(map[string]any)
	for _, c := range x.actors[:3] {
		code, out := c.call("POST", "/v1/plans/"+plan["id"].(string)+"/approvals", "approve", map[string]any{"decision": "approve", "content_hash": plan["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	code, out := x.actors[2].versioned("POST", "/v1/tasks/"+x.task["id"].(string)+"/assignment-response", "accept", x.task["etag"].(string), map[string]any{"decision": "accept", "plan_hash": plan["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	_, x.task = x.actors[2].call("GET", "/v1/tasks/"+x.task["id"].(string), "", nil)
	if sourcePurpose == "progress" {
		tasks := x.plan["tasks"].([]any)
		audienceTask := tasks[1].(map[string]any)
		code, out = x.actors[1].versioned("POST", "/v1/tasks/"+audienceTask["id"].(string)+"/assignment-response", "accept-audience-task", audienceTask["etag"].(string), map[string]any{"decision": "accept", "plan_hash": x.plan["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
		_, tasks[1] = x.actors[1].call("GET", "/v1/tasks/"+audienceTask["id"].(string), "", nil)
	}
	data := []byte(`{"sources":[{"source_id":"private-input","source_url":"https://example.org/"}]}`)
	var upload map[string]any
	if sourcePurpose == "progress" {
		upload = x.uploadForAudienceWithReviewer(t, data, "input", "progress", x.principals[1:3], x.principals[1])
	} else {
		upload = x.upload(t, data, "input")
	}
	code, input := x.actors[2].call("POST", "/v1/artifacts/"+upload["id"].(string)+"/finalize", "seal-input", x.finalBody(data))
	if code != 200 {
		t.Fatal(code, input)
	}
	if sourcePurpose == "progress" {
		x.startWorker(t)
		if evaluation := x.waitEvaluation(t, input["id"].(string)); evaluation["result"] != "passed" {
			t.Fatal("progress input did not verify", evaluation)
		}
	}
	builder := x.actors[3]
	coordinator := oldActors[3]
	code, room := coordinator.call("POST", "/v1/swarms", "target-room", map[string]any{"objective": "Private independently assembled output", "visibility": "private", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	targetPath := "/v1/swarms/" + room["id"].(string)
	_, targetContext := builder.call("GET", targetPath+"/context", "", nil)
	code, invitation := coordinator.call("POST", targetPath+"/invitations", "builder", map[string]any{"agent_id": agents[3], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invitation)
	}
	code, out = builder.versioned("POST", targetPath+"/membership-response", "join", invitation["etag"].(string), map[string]any{"invitation_id": invitation["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, out)
	}
	_, targetContext = builder.call("GET", targetPath+"/context", "", nil)
	targetSpec := map[string]any{"parent_version": nil, "objective": "Private independently assembled output", "required_principals": []string{x.principals[3]}, "acceptance_principals": []string{x.principals[3]}, "publication_principals": []string{x.principals[3]}, "rights": spec["rights"], "tasks": []any{map[string]any{"key": "assembled", "assignee_agent_id": agents[3], "depends_on": []any{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
	code, targetPlan := builder.versioned("POST", targetPath+"/plans", "target-plan", targetContext["swarm"].(map[string]any)["etag"].(string), targetSpec)
	if code != 201 {
		t.Fatal(code, targetPlan)
	}
	code, out = builder.call("POST", "/v1/plans/"+targetPlan["id"].(string)+"/approvals", "approve", map[string]any{"decision": "approve", "content_hash": targetPlan["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	targetTask := targetPlan["tasks"].([]any)[0].(map[string]any)
	code, out = builder.versioned("POST", "/v1/tasks/"+targetTask["id"].(string)+"/assignment-response", "accept", targetTask["etag"].(string), map[string]any{"decision": "accept", "plan_hash": targetPlan["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	_, targetTask = builder.call("GET", "/v1/tasks/"+targetTask["id"].(string), "", nil)
	y := x
	y.actors = []testClient{builder, builder, builder, builder}
	y.principals = []string{x.principals[3], x.principals[3], x.principals[3], x.principals[3]}
	y.room = room
	y.plan = targetPlan
	y.task = targetTask
	_, templates := builder.call("GET", "/v1/policies/contribution-templates", "", nil)
	policyHash := templates["templates"].([]any)[0].(map[string]any)["input_grant_policy_hash"].(string)
	scope := InputGrantScope{ArtifactSHA256: input["sha256"].(string), ManifestHash: input["manifest_hash"].(string), SourcePlanHash: plan["content_hash"].(string), SourceEpoch: int64(x.task["claim_epoch"].(float64)), TargetTaskID: targetTask["id"].(string), TargetPlanHash: targetPlan["content_hash"].(string), TargetEpoch: int64(targetTask["claim_epoch"].(float64)), RecipientAgent: agents[3], RecipientPrincipal: x.principals[3], Purpose: "assembly", TemplateID: "test-contribution-v1", TemplateHash: template.SHA256, GrantPolicyHash: policyHash, LicenseID: "CC0-1.0", SchemaFamily: "source_catalog_v1", ExpiresAt: time.Now().Add(time.Hour), SurviveWithdrawal: survival}
	return privateGrantScenario{source: x, target: y, input: input, inputData: data, scope: scope, coordinator: coordinator}
}

func (x privateGrantScenario) grant(t *testing.T, purpose string) []string {
	t.Helper()
	scope := x.scope
	scope.Purpose = purpose
	ids := []string{}
	grantors := x.source.actors[:3]
	if x.input["purpose"] == "progress" {
		grantors = x.source.actors[1:3]
	}
	for _, c := range grantors {
		code, out := c.call("POST", "/v1/artifacts/"+x.input["id"].(string)+"/input-grants", "grant-"+purpose, scope)
		if code != 201 {
			t.Fatal(code, out)
		}
		ids = append(ids, out["id"].(string))
	}
	return ids
}
func (x privateGrantScenario) derivedBody(data []byte) map[string]any {
	body := x.target.finalBody(data)
	manifest := body["manifest"].(map[string]any)
	manifest["inputs"] = []any{map[string]any{"artifact_id": x.input["id"], "sha256": x.input["sha256"], "license_id": "CC0-1.0"}}
	manifest["attribution"] = []any{map[string]any{"principal_id": x.source.principals[2], "text": "Synthetic fixture data"}, map[string]any{"principal_id": x.source.principals[3], "text": "Named assembler"}}
	return body
}
func TestPrivateArtifactNamedGrantsExposeOnlyInputAndPermitAssembly(t *testing.T) {
	x := newPrivateGrantScenario(t, true)
	sourcePath := "/v1/artifacts/" + x.input["id"].(string)
	if code, _ := x.target.actors[2].call("GET", sourcePath, "", nil); code != 404 {
		t.Fatal("ungranted private input exposed", code)
	}
	grants := x.grant(t, "assembly")
	x.grant(t, "evaluation")
	contextPath := sourcePath + "?purpose=assembly&task_id=" + x.scope.TargetTaskID
	code, shared := x.target.actors[2].call("GET", contextPath, "", nil)
	if code != 200 || shared["swarm_id"] != nil || shared["plan_id"] != nil || shared["task_id"] != nil {
		t.Fatal("input context disclosure", code, shared)
	}
	req := httptest.NewRequest("GET", sourcePath+"/content?purpose=assembly&task_id="+x.scope.TargetTaskID, nil)
	req.Header.Set("Authorization", "Bearer "+x.target.actors[2].token)
	w := httptest.NewRecorder()
	x.source.s.ServeHTTP(w, req)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), x.inputData) {
		t.Fatal("named input bytes", w.Code, w.Body.String())
	}
	for _, path := range []string{"/v1/swarms/" + x.source.room["id"].(string) + "/context", "/v1/plans/" + x.source.plan["id"].(string), "/v1/tasks/" + x.source.task["id"].(string)} {
		if code, _ := x.target.actors[2].call("GET", path, "", nil); code != 404 {
			t.Fatal("input grant exposed private coordination", path, code)
		}
	}
	data := []byte(`{"sources":[{"source_id":"derived","source_url":"https://example.org/"}]}`)
	upload := x.target.upload(t, data, "output")
	code, output := x.target.actors[2].call("POST", "/v1/artifacts/"+upload["id"].(string)+"/finalize", "seal-output", x.derivedBody(data))
	if code != 200 {
		t.Fatal(code, output)
	}
	x.source.startWorker(t)
	evaluation := x.target.waitEvaluation(t, output["id"].(string))
	if evaluation["result"] != "passed" {
		t.Fatal(evaluation)
	}
	var count int
	var grantHash string
	if err := x.source.db.QueryRow(context.Background(), "SELECT count(*),min(rights_grant_hash) FROM undergrid.artifact_inputs WHERE artifact_id=$1", output["id"]).Scan(&count, &grantHash); err != nil || count != 1 || grantHash == x.input["manifest_hash"] {
		t.Fatal("normalized private grant lineage", count, grantHash, err)
	}
	for _, id := range grants {
		code, out := x.target.actors[2].call("GET", "/v1/artifact-input-grants/"+id, "", nil)
		if code != 200 || out["state"] != "active" {
			t.Fatal(code, out)
		}
	}
	var grantCount int
	if err := x.source.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.approvals WHERE artifact_id=$1 AND purpose IN('assembly_grant','evaluation_grant')", x.input["id"]).Scan(&grantCount); err != nil || grantCount != 6 {
		t.Fatal("finite normalized grants", grantCount, err)
	}
	var consistent bool
	if err := x.source.db.QueryRow(context.Background(), `SELECT bool_and(grant_recipient_agent_id::text=grant_scope->>'recipient_agent_id' AND grant_recipient_principal_id::text=grant_scope->>'recipient_principal_id' AND grant_target_task_id::text=grant_scope->>'target_task_id' AND grant_expires_at=(grant_scope->>'expires_at')::timestamptz AND grant_survives_withdrawal=(grant_scope->>'survive_withdrawal')::boolean) FROM undergrid.approvals WHERE artifact_id=$1 AND grant_scope IS NOT NULL`, x.input["id"]).Scan(&consistent); err != nil || !consistent {
		t.Fatal("normalized grant scope", err)
	}
}

func TestPrivateArtifactGrantRejectsWrongExactScopes(t *testing.T) {
	x := newPrivateGrantScenario(t, true)
	for _, name := range []string{"input_hash", "manifest_hash", "source_epoch", "target_epoch", "target_hash", "audience", "template", "policy_hash", "license", "purpose", "expiry"} {
		t.Run(name, func(t *testing.T) {
			scope := x.scope
			switch name {
			case "input_hash":
				scope.ArtifactSHA256 = strings.Repeat("0", 64)
			case "manifest_hash":
				scope.ManifestHash = strings.Repeat("0", 64)
			case "source_epoch":
				scope.SourceEpoch++
			case "target_epoch":
				scope.TargetEpoch++
			case "target_hash":
				scope.TargetPlanHash = strings.Repeat("0", 64)
			case "audience":
				scope.RecipientAgent = NewID()
			case "template":
				scope.TemplateHash = strings.Repeat("0", 64)
			case "policy_hash":
				scope.GrantPolicyHash = strings.Repeat("0", 64)
			case "license":
				scope.LicenseID = "proprietary"
			case "purpose":
				scope.Purpose = "publication"
			case "expiry":
				scope.ExpiresAt = time.Now().Add(-time.Minute)
			}
			code, out := x.source.actors[2].call("POST", "/v1/artifacts/"+x.input["id"].(string)+"/input-grants", "wrong-"+name, scope)
			if code != 409 {
				t.Fatal("unbound input grant accepted", code, out)
			}
		})
	}
	var count int
	if err := x.source.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.approvals WHERE artifact_id=$1 AND grant_scope IS NOT NULL", x.input["id"]).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied grant mutated persistence", count, err)
	}
	x.grant(t, "evaluation")
	sourcePath := "/v1/artifacts/" + x.input["id"].(string)
	code, out := x.target.actors[2].call("GET", sourcePath+"?purpose=assembly&task_id="+x.scope.TargetTaskID, "", nil)
	if code != 403 {
		t.Fatal("evaluation grant broadened to assembly", code, out)
	}
	data := []byte(`{"sources":[{"source_id":"derived","source_url":"https://example.org/"}]}`)
	upload := x.target.upload(t, data, "output")
	code, out = x.target.actors[2].call("POST", "/v1/artifacts/"+upload["id"].(string)+"/finalize", "evaluation-only", x.derivedBody(data))
	if code != 403 {
		t.Fatal("evaluation-only grant assembled", code, out)
	}
}

func TestPrivateArtifactSubmittedGrantWithdrawalSurvivalIsExplicit(t *testing.T) {
	for _, survive := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_survival", true: "scoped_survival"}[survive], func(t *testing.T) {
			x := newPrivateGrantScenario(t, survive)
			x.grant(t, "assembly")
			code, out := x.source.actors[2].call("POST", "/v1/swarms/"+x.source.room["id"].(string)+"/withdraw", "depart", map[string]any{})
			if code != 200 {
				t.Fatal(code, out)
			}
			data := []byte(`{"sources":[{"source_id":"derived","source_url":"https://example.org/"}]}`)
			upload := x.target.upload(t, data, "output")
			path := "/v1/artifacts/" + upload["id"].(string)
			code, out = x.target.actors[2].call("POST", path+"/finalize", "after-departure", x.derivedBody(data))
			if survive {
				if code != 200 {
					t.Fatal("established scoped grant lost on departure", code, out)
				}
				x.source.startWorker(t)
				if evaluation := x.target.waitEvaluation(t, upload["id"].(string)); evaluation["result"] != "passed" {
					t.Fatal(evaluation)
				}
			} else {
				if code != 404 && code != 403 {
					t.Fatal("absent survival grant reused after departure", code, out)
				}
			}
			if code, _ = x.source.actors[2].call("GET", "/v1/swarms/"+x.source.room["id"].(string)+"/context", "", nil); code != 404 {
				t.Fatal("surviving input grant retained room membership", code)
			}
		})
	}
}

func TestPrivateArtifactGrantRevokeBlocksFutureAssemblyAndCannotReopen(t *testing.T) {
	x := newPrivateGrantScenario(t, true)
	ids := x.grant(t, "assembly")
	code, original := x.source.actors[0].call("POST", "/v1/artifacts/"+x.input["id"].(string)+"/input-grants", "grant-assembly", x.scope)
	if code != 201 || original["id"] != ids[0] {
		t.Fatal("grant replay duplicated", code, original)
	}
	code, out := x.target.actors[2].call("POST", "/v1/artifact-input-grants/"+ids[0]+"/revoke", "foreign-revoke", map[string]any{})
	if code != 403 {
		t.Fatal("recipient revoked another holder's grant", code, out)
	}
	code, out = x.source.actors[0].call("POST", "/v1/artifact-input-grants/"+ids[0]+"/revoke", "revoke", map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = x.source.actors[0].call("POST", "/v1/artifacts/"+x.input["id"].(string)+"/input-grants", "reopen", x.scope)
	if code != 409 {
		t.Fatal("revoked scope reopened", code, out)
	}
	data := []byte(`{"sources":[{"source_id":"derived","source_url":"https://example.org/"}]}`)
	upload := x.target.upload(t, data, "output")
	code, out = x.target.actors[2].call("POST", "/v1/artifacts/"+upload["id"].(string)+"/finalize", "after-revoke", x.derivedBody(data))
	if code != 404 && code != 403 {
		t.Fatal("revoked grant authorized future assembly", code, out)
	}
}

func TestPrivateArtifactDerivedPublicationStillNeedsAllExactRightsConsent(t *testing.T) {
	x := newPrivateGrantScenario(t, true)
	x.grant(t, "assembly")
	data := []byte(`{"sources":[{"source_id":"derived","source_url":"https://example.org/"}]}`)
	upload := x.target.upload(t, data, "output")
	path := "/v1/artifacts/" + upload["id"].(string)
	code, output := x.target.actors[2].call("POST", path+"/finalize", "seal-output", x.derivedBody(data))
	if code != 200 {
		t.Fatal(code, output)
	}
	x.source.startWorker(t)
	evaluation := x.target.waitEvaluation(t, upload["id"].(string))
	if evaluation["result"] != "passed" {
		t.Fatal(evaluation)
	}
	code, out := x.target.actors[2].call("POST", path+"/acceptances", "accept", map[string]any{"evaluation_id": evaluation["id"], "artifact_sha256": output["sha256"], "plan_hash": x.target.plan["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	consent := map[string]any{"manifest_hash": output["manifest_hash"], "license_id": "CC0-1.0"}
	code, out = x.target.actors[2].call("POST", path+"/publication-approvals", "builder-consent", consent)
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = x.coordinator.call("POST", path+"/publish", "grant-is-not-publication", map[string]any{"manifest_hash": output["manifest_hash"]})
	if code != 409 {
		t.Fatal("assembly consent implied public release", code, out)
	}
	targetPath := "/v1/swarms/" + x.target.room["id"].(string)
	for i, c := range x.source.actors[:3] {
		_, me := c.call("GET", "/v1/me", "", nil)
		code, invite := x.coordinator.call("POST", targetPath+"/invitations", "rights-"+me["agent_id"].(string), map[string]any{"agent_id": me["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
		if code != 201 {
			t.Fatal(code, invite)
		}
		code, out = c.versioned("POST", targetPath+"/membership-response", "join-output", invite["etag"].(string), map[string]any{"invitation_id": invite["id"], "decision": "accept"})
		if code != 200 {
			t.Fatal(code, out)
		}
		code, out = c.call("POST", path+"/publication-approvals", "exact-output-consent", consent)
		if code != 200 {
			t.Fatal(i, code, out)
		}
	}
	// Publisher and named assembler are distinct agents; publication gets only
	// internal lineage proof, never an input-content capability for the publisher.
	sourcePath := "/v1/artifacts/" + x.input["id"].(string)
	if code, _ = x.coordinator.call("GET", sourcePath+"?purpose=assembly&task_id="+x.scope.TargetTaskID, "", nil); code != 404 {
		t.Fatal("publisher inherited named input audience", code)
	}
	code, out = x.coordinator.call("POST", path+"/publish", "publish", map[string]any{"manifest_hash": output["manifest_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	if code, _ = x.coordinator.call("GET", sourcePath+"/content?purpose=assembly&task_id="+x.scope.TargetTaskID, "", nil); code != 404 {
		t.Fatal("publishing output exposed private source content", code)
	}
	var sourceState, outputState string
	var ids []string
	if err := x.source.db.QueryRow(context.Background(), "SELECT source.state,output.state,edge.grant_approval_ids FROM undergrid.artifact_inputs edge JOIN undergrid.artifacts source ON source.id=edge.input_artifact_id JOIN undergrid.artifacts output ON output.id=edge.artifact_id WHERE output.id=$1", output["id"]).Scan(&sourceState, &outputState, &ids); err != nil || sourceState == "published" || outputState != "published" || len(ids) != 3 {
		t.Fatal("private source/output publication relation", sourceState, outputState, ids, err)
	}
}

func TestPrivateArtifactGrantExpiryAndConfigurationChangesFailClosed(t *testing.T) {
	for _, name := range []string{"expiry", "template_scope_change", "target_epoch", "owner_policy"} {
		t.Run(name, func(t *testing.T) {
			x := newPrivateGrantScenario(t, true)
			if name == "expiry" {
				x.scope.ExpiresAt = time.Now().Add(2 * time.Second)
			}
			x.grant(t, "assembly")
			switch name {
			case "expiry":
				time.Sleep(time.Until(x.scope.ExpiresAt.Add(50 * time.Millisecond)))
			case "template_scope_change":
				template := x.source.s.templates[x.scope.TemplateID]
				template.InputGrantPurposes = []string{"assembly"}
				if err := x.source.s.SetContributionTemplates([]ContributionTemplate{template}); err != nil {
					t.Fatal(err)
				}
			case "target_epoch":
				if _, err := x.source.db.Exec(context.Background(), "UPDATE undergrid.tasks SET claim_epoch=claim_epoch+1 WHERE id=$1", x.scope.TargetTaskID); err != nil {
					t.Fatal(err)
				}
				x.target.task["claim_epoch"] = float64(2)
			case "owner_policy":
				actor := x.source.actors[0]
				_, policy := actor.call("GET", "/v1/owner/policy", "", nil)
				code, out := actor.versioned("POST", "/v1/owner/policy", "", policy["etag"].(string), map[string]any{"template_ids": []string{}, "license_ids": []string{}, "task_categories": []string{}, "max_active_tasks": 0, "expires_at": time.Now().Add(time.Hour)})
				if code != 200 {
					t.Fatal(code, out)
				}
			}
			code, out := x.target.actors[2].call("GET", "/v1/artifacts/"+x.input["id"].(string)+"?purpose=assembly&task_id="+x.scope.TargetTaskID, "", nil)
			if code < 400 {
				t.Fatal("stale grant retained input access", name, code, out)
			}
			data := []byte(`{"sources":[{"source_id":"derived","source_url":"https://example.org/"}]}`)
			upload := x.target.upload(t, data, "output")
			code, out = x.target.actors[2].call("POST", "/v1/artifacts/"+upload["id"].(string)+"/finalize", "stale-grant", x.derivedBody(data))
			if code < 400 {
				t.Fatal("stale grant authorized new assembly", name, code, out)
			}
		})
	}
}

func TestPrivateArtifactNamedGrantRecoverySurvivesRestartWithoutRoomDisclosure(t *testing.T) {
	x := newPrivateGrantScenario(t, true)
	ids := x.grant(t, "assembly")
	code, context := x.target.actors[2].call("GET", "/v1/agent-context", "", nil)
	if code != 200 || context["input_grants"] == nil || len(context["input_grants"].([]any)) != 3 {
		t.Fatal("named input grants missing from canonical recovery", code, context)
	}
	x.source.startWorker(t)
	deadline := time.Now().Add(10 * time.Second)
	seen := map[string]bool{}
	for time.Now().Before(deadline) {
		code, page := x.target.actors[2].call("GET", "/v1/inbox", "", nil)
		if code != 200 {
			t.Fatal(code, page)
		}
		for _, item := range page["events"].([]any) {
			event := item.(map[string]any)
			for _, id := range ids {
				if event["resource_path"] == "/v1/artifact-input-grants/"+id {
					seen[id] = true
				}
			}
		}
		if len(seen) == 3 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if len(seen) != 3 {
		t.Fatal("transactional named grant delivery missing", seen)
	}
	restarted := New(x.source.s.dsn, "development", nil)
	if err := restarted.SetContributionTemplates([]ContributionTemplate{x.source.s.templates[x.scope.TemplateID]}); err != nil {
		t.Fatal(err)
	}
	restarted.cursorKey = append([]byte(nil), x.source.s.cursorKey...)
	peer := x.target.actors[2]
	peer.s = restarted
	code, recovered := peer.call("GET", "/v1/agent-context", "", nil)
	if code != 200 || len(recovered["input_grants"].([]any)) != 3 {
		t.Fatal("restart lost named grant recovery", code, recovered)
	}
	code, out := x.source.actors[0].call("POST", "/v1/artifact-input-grants/"+ids[0]+"/revoke", "revoke", map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, recovered = peer.call("GET", "/v1/agent-context", "", nil)
	if code != 200 || len(recovered["input_grants"].([]any)) != 0 {
		t.Fatal("revoked grant survived recovery authority", code, recovered)
	}
}

func TestPrivateArtifactOldAgreementCannotGainImplicitInputGrantPowers(t *testing.T) {
	x := newArtifactScenario(t)
	data := []byte(`{"sources":[{"source_id":"old-agreement","source_url":"https://example.org/"}]}`)
	upload := x.upload(t, data, "old-input")
	code, input := x.actors[2].call("POST", "/v1/artifacts/"+upload["id"].(string)+"/finalize", "seal", x.finalBody(data))
	if code != 200 {
		t.Fatal(code, input)
	}
	template := x.s.templates["test-contribution-v1"]
	template.InputGrantPurposes = []string{"assembly"}
	if err := x.s.SetContributionTemplates([]ContributionTemplate{template}); err != nil {
		t.Fatal(err)
	}
	// Owner issues a new grant-scoped agent through self-service and deliberately
	// admits it to the original room; neither action reapproves the old plan.
	owner := x.actors[2]
	code, agent := owner.call("POST", "/v1/agents", "old-agreement-new-grant-agent", map[string]any{"name": "new grant agent", "scopes": []string{"rights:accept", "artifacts:grant", "members:respond"}, "profile": map[string]any{"accept_invitations": true}})
	if code != 201 {
		t.Fatal(code, agent)
	}
	code, invite := x.actors[0].call("POST", "/v1/swarms/"+x.room["id"].(string)+"/invitations", "admit-new", map[string]any{"agent_id": agent["id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invite)
	}
	owner.token = agent["token"].(string)
	code, out := owner.versioned("POST", "/v1/swarms/"+x.room["id"].(string)+"/membership-response", "join", invite["etag"].(string), map[string]any{"invitation_id": invite["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, out)
	}
	scope := InputGrantScope{ArtifactSHA256: input["sha256"].(string), ManifestHash: input["manifest_hash"].(string), SourcePlanHash: x.plan["content_hash"].(string), SourceEpoch: 1, Purpose: "assembly", TemplateID: template.ID, TemplateHash: template.SHA256, GrantPolicyHash: inputGrantPolicyHash(template), LicenseID: "CC0-1.0", SchemaFamily: "source_catalog_v1", ExpiresAt: time.Now().Add(time.Hour), SurviveWithdrawal: false}
	code, out = owner.call("POST", "/v1/artifacts/"+input["id"].(string)+"/input-grants", "old-consent", scope)
	if code != 409 || out["error"].(map[string]any)["code"] != "exact_scoped_agreement_required" {
		t.Fatal("old agreement silently gained new input scope", code, out)
	}
}

func TestPrivateArtifactGrantRevocationSerializesWithReadAndSeal(t *testing.T) {
	for _, operation := range []string{"read", "seal"} {
		t.Run(operation, func(t *testing.T) {
			x := newPrivateGrantScenario(t, true)
			ids := x.grant(t, "assembly")
			data := []byte(`{"sources":[{"source_id":"derived","source_url":"https://example.org/"}]}`)
			var upload map[string]any
			if operation == "seal" {
				upload = x.target.upload(t, data, "output")
			}
			revocation, err := x.source.db.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer revocation.Rollback(context.Background())
			if _, err = revocation.Exec(context.Background(), "SELECT id FROM undergrid.principals WHERE id=$1 FOR UPDATE", x.source.principals[0]); err != nil {
				t.Fatal(err)
			}
			if _, err = revocation.Exec(context.Background(), "UPDATE undergrid.approvals SET grant_state='revoked' WHERE id=$1", ids[0]); err != nil {
				t.Fatal(err)
			}
			var locker int
			if err = revocation.QueryRow(context.Background(), "SELECT pg_backend_pid()").Scan(&locker); err != nil {
				t.Fatal(err)
			}
			observer, err := pgx.Connect(context.Background(), os.Getenv("UNDERGRID_TEST_DATABASE_URL"))
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Close(context.Background())
			done := make(chan int, 1)
			go func() {
				if operation == "seal" {
					code, _ := x.target.actors[2].call("POST", "/v1/artifacts/"+upload["id"].(string)+"/finalize", "race", x.derivedBody(data))
					done <- code
				} else {
					req := httptest.NewRequest("GET", "/v1/artifacts/"+x.input["id"].(string)+"/content?purpose=assembly&task_id="+x.scope.TargetTaskID, nil)
					req.Header.Set("Authorization", "Bearer "+x.target.actors[2].token)
					w := httptest.NewRecorder()
					x.source.s.ServeHTTP(w, req)
					done <- w.Code
				}
			}()
			blocked := false
			until := time.Now().Add(3 * time.Second)
			for time.Now().Before(until) {
				var n int
				if err = observer.QueryRow(context.Background(), "SELECT count(*) FROM pg_stat_activity WHERE usename='undergrid_artifact_api_runner' AND $1=ANY(pg_blocking_pids(pid))", locker).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n > 0 {
					blocked = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !blocked {
				select {
				case code := <-done:
					t.Fatal("protected grant operation completed without issuer lock", operation, code)
				default:
					t.Fatal("protected grant operation did not take issuer authority lock", operation)
				}
			}
			if err = revocation.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			select {
			case code := <-done:
				if code != 403 && code != 404 {
					t.Fatal("preceding grant revocation lost race", operation, code)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("grant operation did not recover")
			}
			if operation == "seal" {
				var state string
				var links int
				if err = x.source.db.QueryRow(context.Background(), "SELECT state,(SELECT count(*) FROM undergrid.artifact_inputs WHERE artifact_id=a.id) FROM undergrid.artifacts a WHERE id=$1", upload["id"]).Scan(&state, &links); err != nil || state != "quarantine" || links != 0 {
					t.Fatal("revocation race created unauthorized lineage", state, links, err)
				}
			}
		})
	}
}

func TestPrivateArtifactGrantOwnerExpiryIsRecheckedAfterAuthorityWait(t *testing.T) {
	x := newPrivateGrantScenario(t, true)
	holder := x.source.actors[0]
	_, current := holder.call("GET", "/v1/owner/policy", "", nil)
	expiry := time.Now().Add(2 * time.Second)
	code, out := holder.versioned("POST", "/v1/owner/policy", "", current["etag"].(string), map[string]any{"template_ids": []string{x.scope.TemplateID}, "license_ids": []string{"CC0-1.0"}, "task_categories": []string{"json_dataset"}, "max_active_tasks": 5, "expires_at": expiry})
	if code != 200 {
		t.Fatal(code, out)
	}
	x.grant(t, "assembly")
	authority, err := x.source.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Rollback(context.Background())
	if _, err = authority.Exec(context.Background(), "SELECT id FROM undergrid.principals WHERE id=$1 FOR UPDATE", x.source.principals[0]); err != nil {
		t.Fatal(err)
	}
	var locker int
	if err = authority.QueryRow(context.Background(), "SELECT pg_backend_pid()").Scan(&locker); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("GET", "/v1/artifacts/"+x.input["id"].(string)+"/content?purpose=assembly&task_id="+x.scope.TargetTaskID, nil)
		req.Header.Set("Authorization", "Bearer "+x.target.actors[2].token)
		w := httptest.NewRecorder()
		x.source.s.ServeHTTP(w, req)
		done <- w.Code
	}()
	observer, err := pgx.Connect(context.Background(), os.Getenv("UNDERGRID_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close(context.Background())
	blocked := false
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err = observer.QueryRow(context.Background(), "SELECT count(*) FROM pg_stat_activity WHERE usename='undergrid_artifact_api_runner' AND $1=ANY(pg_blocking_pids(pid))", locker).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			blocked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("private content read did not wait on grant issuer authority")
	}
	time.Sleep(time.Until(expiry.Add(100 * time.Millisecond)))
	if err = authority.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 404 && code != 403 {
			t.Fatal("transaction-start time retained expired owner grant authority", code)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("private input read did not resume")
	}
}
