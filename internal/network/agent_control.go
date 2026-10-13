package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

func ownerAgent(ctx context.Context, tx pgx.Tx, principal, id string) (int64, error) {
	if !validID(id) {
		return 0, fail(404, "not_found")
	}
	var version int64
	var status string
	err := tx.QueryRow(ctx, "SELECT version,status FROM agents WHERE id=$1 AND principal_id=$2 FOR UPDATE", id, principal).Scan(&version, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fail(404, "not_found")
	}
	if err != nil {
		return 0, err
	}
	if status != "active" {
		return 0, fail(403, "inactive_agent")
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", principal, id); err != nil {
		return 0, err
	}
	return version, nil
}
func requireMatch(r *http.Request, version int64) error {
	if r.Header.Get("If-Match") == "" {
		return fail(428, "if_match_required")
	}
	if r.Header.Get("If-Match") != etag(version) {
		return fail(409, "version_conflict")
	}
	return nil
}
func (s *Service) agentOwnerEvent(ctx context.Context, tx pgx.Tx, principal, id string, version int64, kind string, facts any) error {
	origin := "registered"
	if s.environment != "production" {
		origin = "release_test"
	}
	_, err := tx.Exec(ctx, "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,type,facts,origin) VALUES($1,'agent',$2,$3,$4,$5,$6,$7)", NewID(), id, version, principal, kind, canonical(facts), origin)
	return err
}
func (s *Service) ownerAgentControl(r *http.Request, tx pgx.Tx, principal, id, action string) (int, any, error) {
	version, err := ownerAgent(r.Context(), tx, principal, id)
	if err != nil {
		return 0, nil, err
	}
	if r.Method != "POST" {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if action == "recover-credential" {
		return s.ownerRecovery(r, tx, principal, id, version)
	}
	status, result, err := idempotent(r, tx, actor{PrincipalID: principal, AgentID: id}, func() (int, any, error) { return s.ownerAgentControlOnce(r, tx, principal, id, action, version) })
	if err != nil {
		return 0, nil, err
	}
	response, ok := result.(map[string]any)
	if !ok {
		return 0, nil, fail(500, "invalid_control_result")
	}
	if action == "control-challenge" {
		challenge, ok := response["challenge"].(string)
		if !ok {
			return 0, nil, fail(500, "invalid_control_result")
		}
		var current bool
		if err = tx.QueryRow(r.Context(), "SELECT control_nonce_hash=$2 AND control_expires_at>now() AND version=$3 FROM agents WHERE id=$1", id, digest([]byte(challenge)), signedControlVersion(challenge)).Scan(&current); err != nil {
			return 0, nil, fail(409, "control_challenge_invalid")
		}
		if !current {
			return 0, nil, fail(409, "control_challenge_invalid")
		}
	} else if action == "control-proof" {
		var key []byte
		if err = tx.QueryRow(r.Context(), "SELECT signing_key FROM agents WHERE id=$1", id).Scan(&key); err != nil {
			return 0, nil, err
		}
		if digest(key) != response["signing_key_sha256"] {
			return 0, nil, fail(409, "control_proof_superseded")
		}
	}
	return status, result, nil
}
func (s *Service) ownerAgentControlOnce(r *http.Request, tx pgx.Tx, principal, id, action string, version int64) (int, any, error) {
	var err error
	if action != "control-challenge" && action != "control-proof" {
		return 0, nil, fail(404, "not_found")
	}
	if err = requireMatch(r, version); err != nil {
		return 0, nil, err
	}
	ctx := r.Context()
	if action == "control-challenge" {
		var in struct {
			PublicKey string `json:"public_key"`
		}
		if err = decode(r, &in); err != nil {
			return 0, nil, err
		}
		key, e := base64.RawURLEncoding.DecodeString(in.PublicKey)
		if e != nil || len(key) != ed25519.PublicKeySize {
			return 0, nil, fail(422, "ed25519_public_key_required")
		}
		nonce := make([]byte, 32)
		if _, err = rand.Read(nonce); err != nil {
			return 0, nil, err
		}
		expires := time.Now().UTC().Add(5 * time.Minute)
		version++
		// Sign exact server-provided bytes. Bind owner/agent/audience/version/key/expiry.
		challenge := string(canonical(map[string]any{"purpose": "undergrid-agent-key-control-v1", "audience": "undergrid-network", "principal_id": principal, "agent_id": id, "version": version, "public_key": in.PublicKey, "nonce": base64.RawURLEncoding.EncodeToString(nonce), "expires_at": expires}))
		_, err = tx.Exec(ctx, "UPDATE agents SET version=$2,control_nonce_hash=$3,control_expires_at=$4,control_pending_key=$5 WHERE id=$1", id, version, digest([]byte(challenge)), expires, key)
		if err != nil {
			return 0, nil, err
		}
		if err = s.agentOwnerEvent(ctx, tx, principal, id, version, "agent.control_challenged", map[string]any{"key_sha256": digest(key), "expires_at": expires}); err != nil {
			return 0, nil, err
		}
		return 201, map[string]any{"id": id, "version": version, "etag": etag(version), "challenge": challenge, "expires_at": expires, "algorithm": "Ed25519"}, nil
	}
	var in struct {
		Challenge string `json:"challenge"`
		Signature string `json:"signature"`
	}
	if err = decode(r, &in); err != nil {
		return 0, nil, err
	}
	var binding struct {
		Version     int64  `json:"version"`
		PrincipalID string `json:"principal_id"`
		AgentID     string `json:"agent_id"`
		Audience    string `json:"audience"`
		Purpose     string `json:"purpose"`
	}
	if json.Unmarshal([]byte(in.Challenge), &binding) != nil || binding.Version != version || binding.PrincipalID != principal || binding.AgentID != id || binding.Audience != "undergrid-network" || binding.Purpose != "undergrid-agent-key-control-v1" {
		return 0, nil, fail(409, "control_challenge_invalid")
	}
	var hash *string
	var expires *time.Time
	var key []byte
	if err = tx.QueryRow(ctx, "SELECT control_nonce_hash,control_expires_at,control_pending_key FROM agents WHERE id=$1", id).Scan(&hash, &expires, &key); err != nil {
		return 0, nil, err
	}
	signature, e := base64.RawURLEncoding.DecodeString(in.Signature)
	if hash == nil || expires == nil || !expires.After(time.Now()) || *hash != digest([]byte(in.Challenge)) {
		return 0, nil, fail(409, "control_challenge_invalid")
	}
	if e != nil || len(signature) != ed25519.SignatureSize || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, []byte(in.Challenge), signature) {
		return 0, nil, fail(422, "control_signature_invalid")
	}
	version++
	if _, err = tx.Exec(ctx, "UPDATE agents SET version=$2,signing_key=$3,key_control_verified_at=now(),control_nonce_hash=NULL,control_expires_at=NULL,control_pending_key=NULL,endpoint_control_verified_at=NULL WHERE id=$1", id, version, key); err != nil {
		return 0, nil, err
	}
	if err = s.agentOwnerEvent(ctx, tx, principal, id, version, "agent.key_control_verified", map[string]any{"key_sha256": digest(key)}); err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"id": id, "version": version, "etag": etag(version), "key_control_verified": true, "signing_key_sha256": digest(key), "endpoint_control_verified": false, "beneficial_ownership": "unknown"}, nil
}
func ownerAgentPath(path string) (string, string, bool) {
	parts := strings.Split(path, "/")
	if len(parts) == 6 && parts[1] == "v1" && parts[2] == "owner" && parts[3] == "agents" {
		return parts[4], parts[5], true
	}
	return "", "", false
}

func signedControlVersion(challenge string) int64 {
	var binding struct {
		Version int64 `json:"version"`
	}
	if json.Unmarshal([]byte(challenge), &binding) != nil {
		return 0
	}
	return binding.Version
}
