package network

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOwnerAgentListAcknowledgesDeclaredAndLegacyProfiles(t *testing.T) {
	s, db, _, _ := externalNotificationFixture(t)
	a, foreign := client(t, s), client(t, s)
	profile := map[string]any{"public": true, "accept_invitations": false, "recruitment": true, "capabilities": []any{"source_catalog_v1"}}
	code, created := a.call("POST", "/v1/agents", "reviewed-builder-profile", map[string]any{"name": "Reviewed builder", "scopes": []string{"intents:read"}, "profile": profile})
	if code != 201 {
		t.Fatalf("register profile: %d %#v", code, created)
	}
	code, page := a.call("GET", "/v1/agents", "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	if len(page["items"].([]any)) != 2 {
		t.Fatalf("owner agent count: %#v", page)
	}
	foundCreated := false
	for _, raw := range page["items"].([]any) {
		item := raw.(map[string]any)
		expected := map[string]any{"public": false, "accept_invitations": false, "recruitment": false, "capabilities": []any{}}
		if item["id"] == created["id"] {
			foundCreated = true
			expected = profile
		}
		if !reflect.DeepEqual(item["profile"], expected) {
			t.Fatalf("saved profile acknowledgement: got %#v want %#v", item["profile"], expected)
		}
	}
	if !foundCreated {
		t.Fatal("created agent missing from owner list")
	}
	// Historical empty profiles are read compatibly without rewriting stored rows.
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.agents SET profile='{}' WHERE id=$1", created["id"]); err != nil {
		t.Fatal(err)
	}
	code, legacy := a.call("GET", "/v1/agents", "", nil)
	if code != 200 || len(legacy["items"].([]any)) != 2 {
		t.Fatalf("legacy owner list:%d %#v", code, legacy)
	}
	foundCreated = false
	for _, raw := range legacy["items"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == created["id"] {
			foundCreated = true
		}
		if item["id"] == created["id"] && !reflect.DeepEqual(item["profile"], map[string]any{"public": false, "accept_invitations": false, "recruitment": false, "capabilities": []any{}}) {
			t.Fatalf("legacy profile not normalized: %#v", item)
		}
	}
	if !foundCreated {
		t.Fatal("legacy created agent missing")
	}
	var stored string
	if err := db.QueryRow(context.Background(), "SELECT profile::text FROM undergrid.agents WHERE id=$1", created["id"]).Scan(&stored); err != nil || stored != "{}" {
		t.Fatalf("read mutated legacy profile: %s %v", stored, err)
	}
	code, other := foreign.call("GET", "/v1/agents", "", nil)
	if code != 200 || len(other["items"].([]any)) != 1 {
		t.Fatalf("foreign own list:%d %#v", code, other)
	}
	if strings.Contains(string(canonical(other)), created["credential_id"].(string)) || strings.Contains(string(canonical(other)), created["token"].(string)) || strings.Contains(string(canonical(other)), "source_catalog_v1") {
		t.Fatal("foreign list leaked profile or credential")
	}
	for _, raw := range other["items"].([]any) {
		if raw.(map[string]any)["id"] == created["id"] {
			t.Fatal("foreign profile leaked")
		}
	}
}

func TestOwnerDeclaredBuilderOptInsRemainIndependent(t *testing.T) {
	s, db, worker, _ := externalNotificationFixture(t)
	organizer := collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", "owner-profile-role", map[string]any{"objective": "Reviewed declared capability recruitment", "visibility": "restricted", "recruitment_roles": []RecruitmentRole{{Key: "builder", SchemaFamily: "source_catalog_v1", Description: "Build the catalogue"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	cases := []struct {
		name                    string
		public, recruit, invite bool
		capability              string
		eligible                bool
	}{
		{"recruit without unsolicited invitations", true, true, false, "source_catalog_v1", true},
		{"invitable recruitment builder", true, true, true, "source_catalog_v1", true},
		{"unlisted", false, true, true, "source_catalog_v1", false},
		{"recruitment disabled", true, false, true, "source_catalog_v1", false},
		{"different capability", true, true, true, "other_declared_capability", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner := client(t, s)
			profile := map[string]any{"public": tc.public, "recruitment": tc.recruit, "accept_invitations": tc.invite, "capabilities": []string{tc.capability}}
			code, created := owner.call("POST", "/v1/agents", "declared-builder", map[string]any{"name": tc.name, "scopes": []string{"intents:read", "inbox:read", "swarms:read", "members:respond"}, "profile": profile})
			if code != 201 {
				t.Fatal(code, created)
			}
			owner.token = created["token"].(string)
			if err := worker.ScanRecruitment(context.Background(), room["id"].(string)); err != nil {
				t.Fatal(err)
			}
			var offers, events, inbox int
			if err := db.QueryRow(context.Background(), `SELECT count(*),count(e.id),count(i.event_id) FROM undergrid.opportunities o LEFT JOIN undergrid.events e ON e.aggregate_id=o.id AND e.type='opportunity.available' LEFT JOIN undergrid.inbox i ON i.event_id=e.id WHERE o.source_swarm_id=$1 AND o.recipient_agent_id=$2 AND o.kind='builder_recruitment'`, room["id"], created["id"]).Scan(&offers, &events, &inbox); err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.eligible {
				want = 1
			}
			if offers != want || events != want || inbox != want {
				t.Fatalf("optins yielded offers/events/inbox=%d/%d/%d want%d", offers, events, inbox, want)
			}
			code, invitation := organizer.call("POST", "/v1/swarms/"+room["id"].(string)+"/invitations", "profile-invite-"+created["id"].(string), map[string]any{"agent_id": created["id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
			if !tc.invite {
				if code != 404 {
					t.Fatalf("invitation off accepted offer:%d %#v", code, invitation)
				}
			} else if code != 201 {
				t.Fatalf("invitation optin failed:%d %#v", code, invitation)
			}
			var accepted, offered int
			if err := db.QueryRow(context.Background(), "SELECT count(*) FILTER (WHERE state='accepted'),count(*) FILTER (WHERE state='offered') FROM undergrid.memberships WHERE swarm_id=$1 AND agent_id=$2", room["id"], created["id"]).Scan(&accepted, &offered); err != nil {
				t.Fatal(err)
			}
			expectedOffered := 0
			if tc.invite {
				expectedOffered = 1
			}
			if accepted != 0 || offered != expectedOffered {
				t.Fatalf("optin offer/admission states: accepted=%d offered=%d want offered=%d", accepted, offered, expectedOffered)
			}
		})
	}
}
