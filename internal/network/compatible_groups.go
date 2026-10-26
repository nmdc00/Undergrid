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
// membership or used to grant room/task authority. Semantic edges require current
// profile-bound source evidence and current public cross-member cosine scores.
func (s *Service) compatibleGroups(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
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
	_, page, err := s.listOpportunities(query, tx, a)
	if err != nil {
		return 0, nil, err
	}
	opportunities := page.(map[string]any)["items"].([]any)
	ids := groupOpportunityIDs(opportunities)
	rows, err := tx.Query(r.Context(), `SELECT p.id::text,p.principal_id::text,p.projection,COALESCE(m.projection_hash,'') FROM public_intents() p LEFT JOIN public_semantic_metadata() m ON m.id=p.id WHERE p.id=ANY($1::uuid[]) ORDER BY p.id LIMIT 21`, ids)
	if err != nil {
		return 0, nil, err
	}
	candidates := []groupCandidate{}
	projectionHashes := map[string]string{}
	for rows.Next() {
		var candidate groupCandidate
		var document []byte
		var projectionHash string
		if err = rows.Scan(&candidate.ID, &candidate.PrincipalID, &document, &projectionHash); err != nil {
			rows.Close()
			return 0, nil, err
		}
		if err = json.Unmarshal(document, &candidate.Input); err != nil {
			rows.Close()
			return 0, nil, err
		}
		projectionHashes[candidate.ID] = projectionHash
		candidates = append(candidates, candidate)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, nil, err
	}
	truncated := len(candidates) > 20
	if truncated {
		candidates = candidates[:20]
	}
	// Read the current operator profile without a provider request. Configuration
	// changes cannot reuse old evidence or a private disclosure acknowledgement.
	provider, err := semanticConfiguration(s.environment)
	if err != nil {
		return 0, nil, err
	}
	scores := map[string]float64{}
	key := func(a, b string) string {
		if a > b {
			a, b = b, a
		}
		return a + ":" + b
	}
	threshold := 1.0
	if provider != nil {
		_, _, allowed := provider.selectedProjection(input, string(canonical(intentPublicProjection(input))))
		if allowed {
			threshold = provider.minimumSimilarity(input)
			for _, value := range opportunities {
				item := value.(map[string]any)
				if item["kind"] != "shared_demand" {
					continue
				}
				var proof Compatibility
				data, marshalErr := json.Marshal(item["evidence"])
				if marshalErr != nil {
					return 0, nil, marshalErr
				}
				if err = json.Unmarshal(data, &proof); err != nil {
					return 0, nil, err
				}
				if proof.Compatible && proof.SemanticProfileHash == provider.hash && proof.Similarity != nil {
					candidate, ok := item["candidate_intent_id"].(string)
					if ok && proof.CandidateProjectionHash == projectionHashes[candidate] && proof.CandidateProjectionHash != "" {
						scores[key(id, candidate)] = *proof.Similarity
					}
				}
			}
			publicIDs := []string{}
			for _, candidate := range candidates {
				publicIDs = append(publicIDs, candidate.ID)
			}
			pairs, queryErr := tx.Query(r.Context(), "SELECT left_id::text,right_id::text,similarity,left_projection_hash,right_projection_hash FROM public_semantic_pair_scores($1::uuid[],$2)", publicIDs, provider.hash)
			if queryErr != nil {
				return 0, nil, queryErr
			}
			for pairs.Next() {
				var left, right, leftHash, rightHash string
				var score float64
				if err = pairs.Scan(&left, &right, &score, &leftHash, &rightHash); err != nil {
					pairs.Close()
					return 0, nil, err
				}
				if projectionHashes[left] == leftHash && projectionHashes[right] == rightHash {
					scores[key(left, right)] = score
				}
			}
			err = pairs.Err()
			pairs.Close()
			if err != nil {
				return 0, nil, err
			}
		}
	}
	pairCompatible := func(a, b groupCandidate) bool {
		if CompareDemand(a.Input, b.Input).Compatible {
			return true
		}
		score, exists := scores[key(a.ID, b.ID)]
		return exists && score >= threshold
	}
	groups := computeCompatibleGroups(source, candidates, pairCompatible)
	items := []any{}
	for _, group := range groups {
		members := []any{}
		common := declaredLicenses(input.HardConstraints.LicenseIDs)
		for _, member := range group {
			members = append(members, map[string]any{"intent_id": member.ID, "principal_id": member.PrincipalID, "hard_constraints": member.Input.HardConstraints})
			common = intersectLabels(common, declaredLicenses(member.Input.HardConstraints.LicenseIDs))
		}
		mode := "exact_declared_outcome"
		for i, left := range group {
			for _, right := range group[i+1:] {
				if !CompareDemand(left.Input, right.Input).Compatible {
					mode = "pairwise_semantic_and_hard_constraints"
				}
			}
		}
		items = append(items, map[string]any{"members": members, "common_license_ids": common, "comparison_mode": mode, "identity_assurance": "common_control_unknown"})
	}
	return 200, map[string]any{"items": items, "candidate_limit": 20, "group_limit": 10, "truncated": truncated || len(groups) == 10, "limitations": []string{"bounded_nonexhaustive_views", "principal_ids_do_not_prove_independent_owners"}}, nil
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
