package network

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"strings"
)

type groupCandidate struct {
	ID, PrincipalID string
	Input           IntentInput
}

// Compute bounded, deterministic compatible views, not connected components or
// exhaustive maximal cliques. The caller supplies current authorized candidates
// and actual pair evidence; similarity never substitutes for hard constraints.
// The source is present in every view. A principal occupies at most one place.
func computeCompatibleGroups(source groupCandidate, candidates []groupCandidate, pairCompatible func(groupCandidate, groupCandidate) bool) [][]groupCandidate {
	candidates = append([]groupCandidate(nil), candidates...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	if len(candidates) > 20 {
		candidates = candidates[:20]
	}
	result := [][]groupCandidate{}
	emitted := map[string]bool{}
	for _, seed := range candidates {
		if seed.PrincipalID == source.PrincipalID || seed.ID == source.ID || !pairCompatible(source, seed) {
			continue
		}
		seedProof := compareDemandConstraints(source.Input, seed.Input)
		if !seedProof.Compatible {
			continue
		}
		common := intersectLabels(declaredLicenses(source.Input.HardConstraints.LicenseIDs), declaredLicenses(seed.Input.HardConstraints.LicenseIDs))
		if len(common) == 0 {
			continue
		}
		aggregate := source.Input
		if seedProof.CombinedConstraints != nil {
			aggregate.HardConstraints = *seedProof.CombinedConstraints
		} else {
			aggregate.HardConstraints.LicenseIDs = common
		}
		group := []groupCandidate{source, seed}
		principals := map[string]bool{source.PrincipalID: true, seed.PrincipalID: true}
		for _, candidate := range candidates {
			if principals[candidate.PrincipalID] {
				continue
			}
			next := intersectLabels(common, declaredLicenses(candidate.Input.HardConstraints.LicenseIDs))
			if len(next) == 0 {
				continue
			}
			compatible := true
			for _, member := range group {
				if !compareDemandConstraints(member.Input, candidate.Input).Compatible || !pairCompatible(member, candidate) {
					compatible = false
					break
				}
			}
			if !compatible {
				continue
			}
			// Pairwise overlaps do not prove one feasible group. Accumulate
			// all typed constraints through the shared domain evaluator.
			combined := compareDemandConstraints(aggregate, candidate.Input)
			if !combined.Compatible {
				continue
			}
			if combined.CombinedConstraints != nil {
				aggregate.HardConstraints = *combined.CombinedConstraints
			} else {
				aggregate.HardConstraints.LicenseIDs = next
			}
			group = append(group, candidate)
			principals[candidate.PrincipalID] = true
			common = next
		}
		ids := make([]string, 0, len(group))
		for _, m := range group {
			ids = append(ids, m.ID)
		}
		sort.Strings(ids)
		key := strings.Join(ids, ",")
		if emitted[key] {
			continue
		}
		emitted[key] = true
		result = append(result, group)
		if len(result) == 10 {
			break
		}
	}
	return result
}

// A view is derived from current, revision-bound opportunities, never stored as
// membership or used to grant room/task authority. Semantic cross-member proof
// remains unavailable until the public pair-score projection is implemented.
func compatibleGroups(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	var raw []byte
	err := tx.QueryRow(r.Context(), `SELECT r.document FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 AND i.principal_id=$2 AND i.agent_id=$3 AND i.state='active' AND i.watch_enabled AND i.expires_at>clock_timestamp()`, id, a.PrincipalID, a.AgentID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	var input IntentInput
	if err = json.Unmarshal(raw, &input); err != nil {
		return 0, nil, err
	}
	source := groupCandidate{ID: id, PrincipalID: a.PrincipalID, Input: input}
	query := r.Clone(r.Context())
	target := *r.URL
	query.URL = &target
	query.URL.Path = "/v1/intents/" + id + "/opportunities"
	_, page, err := listOpportunities(query, tx, a)
	if err != nil {
		return 0, nil, err
	}
	ids := groupOpportunityIDs(page.(map[string]any)["items"].([]any))
	rows, err := tx.Query(r.Context(), `SELECT id::text,principal_id::text,projection FROM public_intents() WHERE id=ANY($1::uuid[]) ORDER BY id LIMIT 21`, ids)
	if err != nil {
		return 0, nil, err
	}
	candidates := []groupCandidate{}
	for rows.Next() {
		var candidate groupCandidate
		var document []byte
		if err = rows.Scan(&candidate.ID, &candidate.PrincipalID, &document); err != nil {
			rows.Close()
			return 0, nil, err
		}
		if err = json.Unmarshal(document, &candidate.Input); err != nil {
			rows.Close()
			return 0, nil, err
		}
		candidates = append(candidates, candidate)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, nil, err
	}
	groups := computeCompatibleGroups(source, candidates, func(a, b groupCandidate) bool { return CompareDemand(a.Input, b.Input).Compatible })
	items := []any{}
	for _, group := range groups {
		members := []any{}
		common := declaredLicenses(input.HardConstraints.LicenseIDs)
		for _, member := range group {
			members = append(members, map[string]any{"intent_id": member.ID, "principal_id": member.PrincipalID, "hard_constraints": member.Input.HardConstraints})
			common = intersectLabels(common, declaredLicenses(member.Input.HardConstraints.LicenseIDs))
		}
		items = append(items, map[string]any{"members": members, "common_license_ids": common, "comparison_mode": "exact_declared_outcome", "identity_assurance": "common_control_unknown"})
	}
	return 200, map[string]any{"items": items, "candidate_limit": 20, "group_limit": 10, "truncated": len(candidates) > 20 || len(groups) == 10, "limitations": []string{"bounded_nonexhaustive_views", "semantic_cross_member_evidence_pending", "principal_ids_do_not_prove_independent_owners"}}, nil
}

func groupOpportunityIDs(items []any) []string {
	ids := []string{}
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok || item["kind"] != "shared_demand" {
			continue
		}
		id, ok := item["candidate_intent_id"].(string)
		if ok && validID(id) {
			ids = append(ids, id)
		}
	}
	return ids
}
