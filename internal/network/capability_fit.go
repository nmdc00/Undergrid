package network

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
)

type publicAgentFit struct {
	AgentID       string               `json:"agent_id"`
	PrincipalID   string               `json:"principal_id"`
	Origin        string               `json:"origin"`
	Assurance     string               `json:"assurance"`
	Capabilities  []string             `json:"capabilities"`
	SchemaSupport string               `json:"schema_support"`
	HardStatus    string               `json:"hard_status"`
	Evidence      []declaredConstraint `json:"constraint_evidence"`
}
type publicAgentSample struct {
	Limit     int              `json:"limit"`
	Truncated bool             `json:"truncated"`
	Items     []publicAgentFit `json:"items"`
}
type capabilityFitReport struct {
	SourceIntentID    string                   `json:"source_intent_id"`
	SourceVersion     int64                    `json:"source_version"`
	SourceRevision    int                      `json:"source_revision"`
	IdentityAssurance string                   `json:"identity_assurance"`
	Agents            publicAgentSample        `json:"agents"`
	Artifacts         artifactCapabilitySample `json:"artifacts"`
}

func (s *Service) capabilityFit(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
	snapshot, err := loadOwnedDiscoverySource(r, tx, a, id)
	if err != nil {
		return 0, nil, err
	}
	source := snapshot.Input
	report := capabilityFitReport{SourceIntentID: id, SourceVersion: snapshot.Version, SourceRevision: snapshot.Revision, IdentityAssurance: "common_control_unknown", Agents: publicAgentSample{Limit: 20, Items: []publicAgentFit{}}}
	rows, err := tx.Query(r.Context(), `SELECT agent_id,principal_id,capabilities FROM public_capability_declarations($1,$2)`, a.PrincipalID, report.Agents.Limit+1)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		item := publicAgentFit{Origin: "registered_capability", Assurance: "self_declared", SchemaSupport: "not_declared", HardStatus: "unknown"}
		var caps []byte
		if err = rows.Scan(&item.AgentID, &item.PrincipalID, &caps); err != nil {
			return 0, nil, err
		}
		if len(report.Agents.Items) == report.Agents.Limit {
			report.Agents.Truncated = true
			break
		}
		if err = json.Unmarshal(caps, &item.Capabilities); err != nil {
			return 0, nil, err
		}
		claim := &capabilityClaim{}
		if slices.Contains(item.Capabilities, source.Output.SchemaFamily) {
			item.SchemaSupport = "declared"
			claim.Output = &capabilityOutput{SchemaFamily: source.Output.SchemaFamily}
		}
		item.Evidence = compareExternalClaim(source, claim)
		for i := range item.Evidence {
			item.Evidence[i].EvidenceClass = "self_declared_agent_profile"
			item.Evidence[i].EvidenceField = ""
			if item.Evidence[i].Constraint == "schema_family" && item.SchemaSupport == "declared" {
				item.Evidence[i].EvidenceField = "profile.capabilities"
			}
		}
		report.Agents.Items = append(report.Agents.Items, item)
	}
	if err = rows.Err(); err != nil {
		return 0, nil, err
	}
	rows.Close()
	report.Artifacts, err = loadArtifactCapabilitySample(r, tx, a, source)
	if err != nil {
		return 0, nil, err
	}
	return 200, report, nil
}
