package network

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Configure independently of session/cursor keys. Persisted nonces are not bearer
// secrets: immutable metadata and a separately protected service key are required.
func (s *Service) ConfigureCredentialIssuance(id string, key []byte) error {
	if id == "" || len(id) > 80 || len(key) != 32 || bytes.Equal(key, s.cursorKey) {
		return errors.New("dedicated 32-byte credential issuance key and ID required")
	}
	s.issuanceKey = append([]byte(nil), key...)
	s.issuanceKeyID = id
	return nil
}
func (s *Service) credentialToken(principal, agent, id string, generation int64, scopes []string, expires time.Time, nonce []byte) string {
	mac := hmac.New(sha256.New, s.issuanceKey)
	mac.Write(canonical(map[string]any{"purpose": "undergrid-network-credential-v1", "key_id": s.issuanceKeyID, "principal_id": principal, "agent_id": agent, "credential_id": id, "audience": "undergrid-network", "generation": generation, "scopes": scopes, "expires_at": expires.UTC(), "nonce": base64.RawURLEncoding.EncodeToString(nonce)}))
	return "ug_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) issueRecoverableCredential(ctx context.Context, tx pgx.Tx, principal, agent, predecessor string, scopes []string) (any, error) {
	if len(s.issuanceKey) != 32 {
		return nil, fail(503, "credential_issuance_unconfigured")
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	id := NewID()
	expires := s.credentialNow().Add(30 * 24 * time.Hour).Truncate(time.Microsecond)
	replayUntil := s.credentialNow().Add(15 * time.Minute)
	var generation int64
	if err := tx.QueryRow(ctx, "SELECT COALESCE(max(generation),0)+1 FROM credentials WHERE agent_id=$1 AND kind='api_token'", agent).Scan(&generation); err != nil {
		return nil, err
	}
	token := s.credentialToken(principal, agent, id, generation, scopes, expires, nonce)
	hash := sha256.Sum256([]byte(token))
	var previous any
	if predecessor != "" {
		previous = predecessor
	}
	_, err := tx.Exec(ctx, "INSERT INTO credentials(id,agent_id,digest,scopes,audience,expires_at,generation,issuance_nonce,issuance_key_id,issuance_replay_until,predecessor_id) VALUES($1,$2,$3,$4,'undergrid-network',$5,$6,$7,$8,$9,$10)", id, agent, hash[:], scopes, expires, generation, nonce, s.issuanceKeyID, replayUntil, previous)
	if err != nil {
		return nil, err
	}
	if err = credentialEvent(ctx, tx, principal, id, 1, "credential.issued", s.environment); err != nil {
		return nil, err
	}
	// The common idempotency table stores only this nonsecret reference result.
	return map[string]any{"id": agent, "principal_id": principal, "credential_id": id, "generation": generation, "etag": etag(generation), "expires_at": expires, "expires_in": 2592000}, nil
}
func (s *Service) regenerateCredential(ctx context.Context, tx pgx.Tx, principal, agent string, result any) (any, error) {
	ref, ok := result.(map[string]any)
	if !ok {
		return nil, fail(500, "invalid_credential_result")
	}
	id, ok := ref["credential_id"].(string)
	if !ok {
		return nil, fail(500, "invalid_credential_result")
	}
	var nonce, stored []byte
	var scopes []string
	var expires, replay time.Time
	var generation int64
	var keyID string
	err := tx.QueryRow(ctx, `SELECT c.issuance_nonce,c.digest,c.scopes,c.expires_at,c.generation,c.issuance_key_id,c.issuance_replay_until FROM credentials c JOIN agents a ON a.id=c.agent_id WHERE c.id=$1 AND c.agent_id=$2 AND c.kind='api_token' AND a.principal_id=$3 AND a.status='active' AND c.revoked_at IS NULL AND c.expires_at>now()`, id, agent, principal).Scan(&nonce, &stored, &scopes, &expires, &generation, &keyID, &replay)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fail(409, "credential_replay_unavailable")
	}
	if err != nil {
		return nil, err
	}
	if len(s.issuanceKey) != 32 || keyID != s.issuanceKeyID || !replay.After(s.credentialNow()) {
		return nil, fail(409, "credential_replay_unavailable")
	}
	if err = s.validateAgentScopes(ctx, tx, principal, scopes); err != nil {
		return nil, err
	}
	token := s.credentialToken(principal, agent, id, generation, scopes, expires, nonce)
	hash := sha256.Sum256([]byte(token))
	if !hmac.Equal(hash[:], stored) {
		return nil, fail(409, "credential_replay_unavailable")
	}
	output := map[string]any{}
	for k, v := range ref {
		output[k] = v
	}
	output["token"] = token
	return output, nil
}
func (s *Service) ownerRotation(r *http.Request, tx pgx.Tx, principal, id string) (int, any, error) {
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	var agent string
	var generation int64
	var oldScopes []string
	var revoked *time.Time
	var expires time.Time
	err := tx.QueryRow(r.Context(), "SELECT a.id,c.generation,c.scopes,c.revoked_at,c.expires_at FROM agents a JOIN credentials c ON c.agent_id=a.id WHERE c.id=$1 AND c.kind='api_token' AND a.principal_id=$2", id, principal).Scan(&agent, &generation, &oldScopes, &revoked, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	version, err := ownerAgent(r.Context(), tx, principal, agent)
	if err != nil {
		return 0, nil, err
	}
	// Principal/agent locks serialize rotation, revocation and recovery. Check old
	// credential again only in new execution; replay reauthorizes the new target.
	status, ref, err := idempotent(r, tx, actor{PrincipalID: principal, AgentID: agent}, func() (int, any, error) {
		if err := requireMatch(r, generation); err != nil {
			return 0, nil, err
		}
		if revoked != nil || !expires.After(time.Now()) {
			return 0, nil, fail(409, "credential_inactive")
		}
		var in struct {
			Scopes []string `json:"scopes"`
		}
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		if err := s.validateAgentScopes(r.Context(), tx, principal, in.Scopes); err != nil {
			return 0, nil, err
		}
		for _, scope := range in.Scopes {
			if !slices.Contains(oldScopes, scope) {
				return 0, nil, fail(403, "rotation_scope_escalation")
			}
		}
		if err := s.revokeCredentialSet(r.Context(), tx, principal, agent, id); err != nil {
			return 0, nil, err
		}
		out, err := s.issueRecoverableCredential(r.Context(), tx, principal, agent, id, in.Scopes)
		if err != nil {
			return 0, nil, err
		}
		if err = s.advanceAgentCredentialVersion(r.Context(), tx, principal, agent, version, "agent.credential_rotated"); err != nil {
			return 0, nil, err
		}
		return 201, out, nil
	})
	if err != nil {
		return 0, nil, err
	}
	result, err := s.regenerateCredential(r.Context(), tx, principal, agent, ref)
	return status, result, err
}
func (s *Service) ownerRecovery(r *http.Request, tx pgx.Tx, principal, agent string, version int64) (int, any, error) {
	status, ref, err := idempotent(r, tx, actor{PrincipalID: principal, AgentID: agent}, func() (int, any, error) {
		if err := requireMatch(r, version); err != nil {
			return 0, nil, err
		}
		var in struct {
			Scopes []string `json:"scopes"`
		}
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		if err := s.validateAgentScopes(r.Context(), tx, principal, in.Scopes); err != nil {
			return 0, nil, err
		}
		if err := s.revokeCredentialSet(r.Context(), tx, principal, agent, ""); err != nil {
			return 0, nil, err
		}
		out, err := s.issueRecoverableCredential(r.Context(), tx, principal, agent, "", in.Scopes)
		if err != nil {
			return 0, nil, err
		}
		if err = s.advanceAgentCredentialVersion(r.Context(), tx, principal, agent, version, "agent.credential_recovered"); err != nil {
			return 0, nil, err
		}
		return 201, out, nil
	})
	if err != nil {
		return 0, nil, err
	}
	result, err := s.regenerateCredential(r.Context(), tx, principal, agent, ref)
	return status, result, err
}
func (s *Service) revokeCredentialSet(ctx context.Context, tx pgx.Tx, principal, agent, id string) error {
	rows, err := tx.Query(ctx, "UPDATE credentials SET revoked_at=now() WHERE agent_id=$1 AND kind='api_token' AND revoked_at IS NULL AND ($2='' OR id::text=$2) RETURNING id", agent, id)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = credentialEvent(ctx, tx, principal, id, 2, "credential.revoked", s.environment); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) advanceAgentCredentialVersion(ctx context.Context, tx pgx.Tx, principal, agent string, version int64, kind string) error {
	_, err := tx.Exec(ctx, "UPDATE agents SET version=$2 WHERE id=$1", agent, version+1)
	if err != nil {
		return err
	}
	return s.agentOwnerEvent(ctx, tx, principal, agent, version+1, kind, map[string]any{})
}
func ownerRotationPath(path string) (string, bool) {
	parts := strings.Split(path, "/")
	if len(parts) == 6 && parts[1] == "v1" && parts[2] == "owner" && parts[3] == "credentials" && parts[5] == "rotate" {
		return parts[4], true
	}
	return "", false
}

func (s *Service) credentialNow() time.Time {
	if s.credentialClock != nil {
		return s.credentialClock().UTC()
	}
	return time.Now().UTC()
}
