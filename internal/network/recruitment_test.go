package network

import (
	"context"
	"testing"
	"time"
)

func TestBuilderDiscoveryApplicationAndAdmission(t *testing.T) {
	s, _ := fixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", "recruit-room", map[string]any{"objective": "Create a reusable source catalog", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "catalog-builder", "schema_family": "source_catalog_v1", "description": "Build and validate the dataset"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	if err := s.ScanRecruitment(context.Background(), room["id"].(string)); err != nil {
		t.Fatal(err)
	}
	code, page := builder.call("GET", "/v1/opportunities?kind=builder_recruitment", "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	found := false
	for _, raw := range page["items"].([]any) {
		item := raw.(map[string]any)
		if item["swarm_id"] == room["id"] && item["role_key"] == "catalog-builder" {
			found = true
		}
	}
	if !found {
		t.Fatal("builder was not offered the published role", page)
	}
	path := "/v1/swarms/" + room["id"].(string)
	code, application := builder.call("POST", path+"/applications", "apply", map[string]any{"role_key": "catalog-builder", "capability_refs": []string{"source_catalog_v1"}})
	if code != 201 {
		t.Fatal(code, application)
	}
	code, _ = builder.call("GET", path+"/context", "", nil)
	if code != 404 {
		t.Fatalf("application granted access: %d", code)
	}
	_, me := builder.call("GET", "/v1/me", "", nil)
	code, invitation := organizer.call("POST", path+"/invitations", "offer", map[string]any{"agent_id": me["agent_id"], "role": "contributor", "application_id": application["id"], "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invitation)
	}
	code, accepted := builder.versioned("POST", path+"/membership-response", "accept", invitation["etag"].(string), map[string]any{"invitation_id": invitation["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, accepted)
	}
	code, _ = builder.call("GET", path+"/context", "", nil)
	if code != 200 {
		t.Fatalf("accepted builder lacks access: %d", code)
	}
}
