package network

import (
	"testing"
	"time"
)

func TestSwarmCreationDoesNotGrantInvitationAuthority(t *testing.T) {
	s, _ := fixture(t)
	owner, target := client(t, s), collaborationClient(t, s)
	code, issued := owner.call("POST", "/v1/agents", "room-creator-agent", map[string]any{"name": "room-creator", "scopes": []string{"swarms:create"}})
	if code != 201 {
		t.Fatal(code, issued)
	}
	owner.token = issued["token"].(string)
	code, room := owner.call("POST", "/v1/swarms", "scoped-room", map[string]any{"objective": "Scoped room", "visibility": "private", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, who := target.call("GET", "/v1/me", "", nil)
	code, _ = owner.call("POST", "/v1/swarms/"+room["id"].(string)+"/invitations", "unauthorized-invite", map[string]any{"agent_id": who["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 403 {
		t.Fatalf("creation scope authorized invitations: %d", code)
	}
}
