package network

import (
 "context"
 "reflect"
 "testing"
)

func TestOwnerAgentListAcknowledgesDeclaredAndLegacyProfiles(t *testing.T) {
 s, db := fixture(t)
 a, foreign := client(t, s), client(t, s)
 profile := map[string]any{"public": true, "accept_invitations": false, "recruitment": true, "capabilities": []any{"source_catalog_v1"}}
 code, created := a.call("POST", "/v1/agents", "reviewed-builder-profile", map[string]any{"name":"Reviewed builder", "scopes":[]string{"intents:read"}, "profile":profile})
 if code != 201 { t.Fatalf("register profile: %d %#v",code,created) }
 code, page := a.call("GET", "/v1/agents", "", nil)
 if code != 200 { t.Fatal(code,page) }
 for _, raw := range page["items"].([]any) {
  item := raw.(map[string]any)
  expected := map[string]any{"public":false,"accept_invitations":false,"recruitment":false,"capabilities":[]any{}}
  if item["id"] == created["id"] { expected = profile }
  if !reflect.DeepEqual(item["profile"],expected) { t.Fatalf("saved profile acknowledgement: got %#v want %#v",item["profile"],expected) }
 }
 // Historical empty profiles are read compatibly without rewriting stored rows.
 if _,err := db.Exec(context.Background(),"UPDATE undergrid.agents SET profile='{}' WHERE id=$1",created["id"]); err != nil {t.Fatal(err)}
 _, legacy := a.call("GET","/v1/agents","",nil)
 for _,raw := range legacy["items"].([]any) { item:=raw.(map[string]any); if item["id"]==created["id"] && !reflect.DeepEqual(item["profile"],map[string]any{"public":false,"accept_invitations":false,"recruitment":false,"capabilities":[]any{}}) {t.Fatalf("legacy profile not normalized: %#v",item)} }
 var stored string
 if err:=db.QueryRow(context.Background(),"SELECT profile::text FROM undergrid.agents WHERE id=$1",created["id"]).Scan(&stored);err!=nil||stored!="{}" {t.Fatalf("read mutated legacy profile: %s %v",stored,err)}
 _, other := foreign.call("GET","/v1/agents","",nil)
 for _, raw := range other["items"].([]any) { if raw.(map[string]any)["id"]==created["id"] {t.Fatal("foreign profile leaked")}}
}
