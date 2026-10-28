package network

import (
	"github.com/jackc/pgx/v5"
	"net/http"
)

type artifactCapabilityFit struct {
	ArtifactID   string               `json:"artifact_id"`
	PrincipalID  string               `json:"principal_id"`
	Version      int64                `json:"version"`
	SHA256       string               `json:"sha256"`
	ManifestHash string               `json:"manifest_hash"`
	Title        string               `json:"title"`
	SchemaFamily string               `json:"schema_family"`
	LicenseID    string               `json:"license_id"`
	Origin       string               `json:"origin"`
	Assurance    string               `json:"assurance"`
	ContentTrust string               `json:"content_trust"`
	Limitations  string               `json:"limitations"`
	HardStatus   string               `json:"hard_status"`
	Evidence     []declaredConstraint `json:"constraint_evidence"`
}
type artifactCapabilitySample struct {
	Limit     int                     `json:"limit"`
	Truncated bool                    `json:"truncated"`
	Items     []artifactCapabilityFit `json:"items"`
}

// Published licenses and content do not disappear merely because a producer's
// credential is revoked. State and existing artifact RLS own public eligibility.
func loadArtifactCapabilitySample(r *http.Request, tx pgx.Tx, a actor, source IntentInput) (artifactCapabilitySample, error) {
	sample := artifactCapabilitySample{Limit: 20, Items: []artifactCapabilityFit{}}
	rows, err := tx.Query(r.Context(), `SELECT id,principal_id,version,sha256,manifest_hash,manifest->>'title',manifest->>'schema_family',license_id FROM artifacts WHERE state='published' AND purpose='output' AND principal_id<>$1 ORDER BY id LIMIT $2`, a.PrincipalID, sample.Limit+1)
	if err != nil {
		return sample, err
	}
	defer rows.Close()
	for rows.Next() {
		item := artifactCapabilityFit{Origin: "registered_capability", Assurance: "published_manifest_declaration", ContentTrust: "untrusted", Limitations: artifactLimitations, HardStatus: "aligned"}
		if err = rows.Scan(&item.ArtifactID, &item.PrincipalID, &item.Version, &item.SHA256, &item.ManifestHash, &item.Title, &item.SchemaFamily, &item.LicenseID); err != nil {
			return sample, err
		}
		if len(sample.Items) == sample.Limit {
			sample.Truncated = true
			break
		}
		claim := &capabilityClaim{Output: &capabilityOutput{Kind: "json_dataset", SchemaFamily: item.SchemaFamily}, LicenseIDs: []string{item.LicenseID}}
		item.Evidence = compareExternalClaim(source, claim)
		for i := range item.Evidence {
			evidence := &item.Evidence[i]
			evidence.EvidenceClass = "published_manifest_declaration"
			switch evidence.EvidenceField {
			case "capability_claim.output.kind":
				evidence.EvidenceField = "artifact.media_type"
			case "capability_claim.output.schema_family":
				evidence.EvidenceField = "manifest.schema_family"
			case "capability_claim.license_ids":
				evidence.EvidenceField = "manifest.license_id"
			default:
				evidence.EvidenceField = ""
			}
			if evidence.Status == "declared_incompatible" {
				item.HardStatus = "incompatible"
			} else if evidence.Status == "unknown" && item.HardStatus != "incompatible" {
				item.HardStatus = "unknown"
			}
		}
		sample.Items = append(sample.Items, item)
	}
	return sample, rows.Err()
}
