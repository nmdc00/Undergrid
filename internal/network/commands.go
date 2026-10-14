package network

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type idempotencyRequest struct {
	key, operation, hash string
	raw                  []byte
}

// Both command subjects use the same wire fingerprint rules. If-Match remains
// part of agent command identity; owner agent creation has no version precondition.
func readIdempotencyRequest(r *http.Request, includeMatch bool) (idempotencyRequest, error) {
	var request idempotencyRequest
	request.key = r.Header.Get("Idempotency-Key")
	if len(request.key) < 1 || len(request.key) > 128 {
		return request, fail(400, "idempotency_key_required")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil {
		return request, err
	}
	if len(raw) > 65536 {
		return request, fail(413, "body_too_large")
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return request, fail(400, "invalid_json")
	}
	fingerprint := canonical(value)
	if includeMatch {
		fingerprint = append(fingerprint, []byte(r.Header.Get("If-Match"))...)
	}
	request.operation = r.Method + " " + r.URL.Path
	request.hash = digest(fingerprint)
	request.raw = raw
	return request, nil
}

func restoreIdempotencyBody(r *http.Request, request idempotencyRequest) {
	r.Body = io.NopCloser(strings.NewReader(string(request.raw)))
}

// Authenticated command transactions already hold the actor's authority locks.
// This common boundary persists one result with its effects, including queue work.
func idempotent(r *http.Request, tx pgx.Tx, a actor, perform func() (int, any, error)) (int, any, error) {
	request, err := readIdempotencyRequest(r, true)
	if err != nil {
		return 0, nil, err
	}
	var oldHash string
	var status int
	var response []byte
	err = tx.QueryRow(r.Context(), "SELECT request_hash,status,response FROM idempotency WHERE actor_id=$1 AND operation=$2 AND key=$3", a.AgentID, request.operation, request.key).Scan(&oldHash, &status, &response)
	if err == nil {
		if oldHash != request.hash {
			return 0, nil, fail(409, "idempotency_conflict")
		}
		var out any
		err = json.Unmarshal(response, &out)
		return status, out, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, err
	}
	restoreIdempotencyBody(r, request)
	status, result, err := perform()
	if err != nil {
		return 0, nil, err
	}
	_, err = tx.Exec(r.Context(), "INSERT INTO idempotency(actor_id,operation,key,request_hash,status,response) VALUES($1,$2,$3,$4,$5,$6)", a.AgentID, request.operation, request.key, request.hash, status, canonical(result))
	return status, result, err
}

// Owner commands that create their first agent use the verified principal as
// their command subject because no agent identity exists yet. The migration's
// RLS policy additionally requires ownerCommand's verified-owner transaction
// context and forbids an agent actor from reading or writing these rows.
func idempotentOwner(r *http.Request, tx pgx.Tx, principal string, perform func() (int, any, error)) (int, any, error) {
	request, err := readIdempotencyRequest(r, false)
	if err != nil {
		return 0, nil, err
	}
	var oldHash string
	var status int
	var response []byte
	err = tx.QueryRow(r.Context(), "SELECT request_hash,status,response FROM idempotency WHERE owner_principal_id=$1 AND operation=$2 AND key=$3", principal, request.operation, request.key).Scan(&oldHash, &status, &response)
	if err == nil {
		if oldHash != request.hash {
			return 0, nil, fail(409, "idempotency_conflict")
		}
		var out any
		err = json.Unmarshal(response, &out)
		return status, out, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, err
	}
	restoreIdempotencyBody(r, request)
	status, result, err := perform()
	if err != nil {
		return 0, nil, err
	}
	_, err = tx.Exec(r.Context(), "INSERT INTO idempotency(actor_id,owner_principal_id,operation,key,request_hash,status,response) VALUES(NULL,$1,$2,$3,$4,$5,$6)", principal, request.operation, request.key, request.hash, status, canonical(result))
	return status, result, err
}
