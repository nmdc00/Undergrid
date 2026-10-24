package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// This bounded last-action record is server owned and private. ResultRevision
// distinguishes its historical assertion from any subsequent generic revision.
type partialSatisfactionAudit struct {
	EvidenceClass               string   `json:"evidence_class"`
	BasisRevision               int      `json:"basis_revision"`
	ResultRevision              int      `json:"result_revision"`
	BasisVersion                int64    `json:"basis_version"`
	PreviousFulfillmentRevision int      `json:"previous_fulfillment_revision,omitempty"`
	ActorPrincipalID            string   `json:"actor_principal_id"`
	ActorAgentID                string   `json:"actor_agent_id"`
	FulfilledFields             []string `json:"fulfilled_required_fields"`
	RemainingFields             []string `json:"remaining_required_fields"`
	LeadID                      string   `json:"lead_id"`
	SourceID                    string   `json:"source_id"`
	SourceVersion               string   `json:"source_version"`
	ProjectionHash              string   `json:"projection_hash"`
	ReportHash                  string   `json:"report_hash"`
	CommandDigest               string   `json:"command_digest"`
}
type intentRevisionDocument struct {
	IntentInput
	PartialSatisfaction *partialSatisfactionAudit `json:"partial_satisfaction,omitempty"`
}
type partialSatisfactionInput struct {
	LeadID            string   `json:"lead_id"`
	SourceVersion     string   `json:"source_version"`
	ProjectionHash    string   `json:"projection_hash"`
	ReportHash        string   `json:"report_hash"`
	IntentionRevision int      `json:"intention_revision"`
	FulfilledFields   []string `json:"fulfilled_required_fields"`
}

func partialSatisfactionID(path string) (string, bool) {
	const prefix = "/v1/intents/"
	const suffix = "/partial-satisfaction"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return id, validID(id)
}
func (s *Service) partialSatisfaction(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
	if r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if !slices.Contains(a.Scopes, "intents:write") {
		return 0, nil, insufficientScope(a, "intents:write")
	}
	request, err := readIdempotencyRequest(r, true)
	if err != nil {
		return 0, nil, err
	}
	var input partialSatisfactionInput
	if bytes.Equal(bytes.TrimSpace(request.raw), []byte("null")) {
		return 0, nil, fail(400, "invalid_json")
	}
	if err = decodeIntentJSON(request.raw, &input); err != nil {
		return 0, nil, err
	}
	if !validID(input.LeadID) || input.IntentionRevision < 1 || len(input.SourceVersion) == 0 || len(input.SourceVersion) > 256 || len(input.ProjectionHash) != 64 || len(input.ReportHash) != 64 {
		return 0, nil, fail(422, "invalid_partial_satisfaction")
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	var currentRevision int
	var version int64
	var raw []byte
	err = tx.QueryRow(r.Context(), `SELECT i.current_revision,i.version,r.document FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 AND i.principal_id=$2 AND i.agent_id=$3 AND i.state='active' AND i.watch_enabled AND i.expires_at>clock_timestamp() FOR UPDATE OF i`, id, a.PrincipalID, a.AgentID).Scan(&currentRevision, &version, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	var current intentRevisionDocument
	if err = json.Unmarshal(raw, &current); err != nil {
		return 0, nil, err
	}
	commandDigest := digest(canonical([]string{a.AgentID, request.operation, request.key, request.hash}))
	replay := input.IntentionRevision != currentRevision
	if replay {
		audit := current.PartialSatisfaction
		if audit == nil || audit.BasisRevision != input.IntentionRevision || audit.ResultRevision != currentRevision || audit.CommandDigest != commandDigest || r.Header.Get("If-Match") != etag(audit.BasisVersion) {
			return 0, nil, fail(409, "version_conflict")
		}
	} else if r.Header.Get("If-Match") != etag(version) {
		return 0, nil, fail(409, "version_conflict")
	}
	var basisRaw []byte
	if err = tx.QueryRow(r.Context(), "SELECT document FROM intent_revisions WHERE intent_id=$1 AND revision=$2", id, input.IntentionRevision).Scan(&basisRaw); err != nil {
		return 0, nil, err
	}
	var basis IntentInput
	if err = json.Unmarshal(basisRaw, &basis); err != nil {
		return 0, nil, err
	}
	// Literal distinct fields only: ambiguous historical requirements are not
	// silently normalized as part of a fulfillment decision.
	required := map[string]bool{}
	for _, field := range basis.HardConstraints.RequiredFields {
		if strings.TrimSpace(field) == "" || required[field] {
			return 0, nil, fail(422, "ambiguous_required_fields")
		}
		required[field] = true
	}
	selected := map[string]bool{}
	for _, field := range input.FulfilledFields {
		if !required[field] || selected[field] {
			return 0, nil, fail(422, "invalid_fulfilled_fields")
		}
		selected[field] = true
	}
	if len(selected) == 0 || len(selected) >= len(required) {
		return 0, nil, fail(422, "partial_remainder_required")
	}
	remaining := []string{}
	for _, field := range basis.HardConstraints.RequiredFields {
		if !selected[field] {
			remaining = append(remaining, field)
		}
	}
	var allowed bool
	if err = tx.QueryRow(r.Context(), "SELECT lock_partial_satisfaction_source($1,$2)", id, input.LeadID).Scan(&allowed); err != nil {
		return 0, nil, err
	}
	if !allowed {
		return 0, nil, fail(404, "not_found")
	}
	var lead externalLead
	var claim []byte
	err = tx.QueryRow(r.Context(), "SELECT * FROM external_fit_source($1,$2)", id, input.LeadID).Scan(&lead.ID, &lead.SourceID, &lead.SourceVersion, &lead.Origin, &lead.CanonicalURL, &lead.Publisher, &lead.PermissionNotice, &lead.Metadata, &lead.ObservedAt, &lead.CheckedAt, &claim)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	projected := externalLeadProjection(lead)
	report := externalFitReportValue(id, input.IntentionRevision, basis, lead, claim)
	if projected.SourceVersion != input.SourceVersion || projected.ProjectionHash != input.ProjectionHash || report["report_hash"] != input.ReportHash {
		return 0, nil, fail(409, "source_projection_conflict")
	}
	restoreIdempotencyBody(r, request)
	return idempotent(r, tx, a, func() (int, any, error) {
		// A replay without its retained receipt must not execute against a head that
		// already contains its effects. This is a consistency failure, not new work.
		if replay {
			return 0, nil, fail(409, "partial_receipt_missing")
		}
		next := basis
		next.HardConstraints.RequiredFields = remaining
		prior := 0
		if current.PartialSatisfaction != nil {
			prior = current.PartialSatisfaction.ResultRevision
		}
		audit := &partialSatisfactionAudit{EvidenceClass: "owner_declared_partial_satisfaction", BasisRevision: currentRevision, ResultRevision: currentRevision + 1, BasisVersion: version, PreviousFulfillmentRevision: prior, ActorPrincipalID: a.PrincipalID, ActorAgentID: a.AgentID, FulfilledFields: append([]string{}, input.FulfilledFields...), RemainingFields: remaining, LeadID: lead.ID, SourceID: lead.SourceID, SourceVersion: input.SourceVersion, ProjectionHash: input.ProjectionHash, ReportHash: input.ReportHash, CommandDigest: commandDigest}
		if err = writeIntentRevision(r.Context(), tx, id, currentRevision+1, version+1, "active", next, audit); err != nil {
			return 0, nil, err
		}
		result, err := finishIntentMutation(r.Context(), tx, a, id, version+1, "intent.partially_satisfied", s.environment)
		return 200, result, err
	})
}
func (s *Service) ownerPartialSatisfaction(r *http.Request, tx pgx.Tx, principal, id string) (int, any, error) {
	var agent string
	err := tx.QueryRow(r.Context(), "SELECT agent_id FROM intents WHERE id=$1 AND principal_id=$2", id, principal).Scan(&agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	a, err := lockOwnerAgent(r.Context(), tx, principal, agent)
	if err != nil {
		return 0, nil, err
	}
	a.Scopes = []string{"intents:write"}
	return s.partialSatisfaction(r, tx, a, id)
}
