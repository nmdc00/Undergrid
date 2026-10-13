package network

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Authenticated command transactions already hold the actor's authority locks.
// This common boundary persists one result with its effects, including queue work.
func idempotent(r *http.Request, tx pgx.Tx, a actor, perform func() (int, any, error)) (int, any, error) {
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 1 || len(key) > 128 {
		return 0, nil, fail(400, "idempotency_key_required")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil {
		return 0, nil, err
	}
	if len(raw) > 65536 {
		return 0, nil, fail(413, "body_too_large")
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return 0, nil, fail(400, "invalid_json")
	}
	hash := digest(append(canonical(value), []byte(r.Header.Get("If-Match"))...))
	operation := r.Method + " " + r.URL.Path
	var oldHash string
	var status int
	var response []byte
	err = tx.QueryRow(r.Context(), "SELECT request_hash,status,response FROM idempotency WHERE actor_id=$1 AND operation=$2 AND key=$3", a.AgentID, operation, key).Scan(&oldHash, &status, &response)
	if err == nil {
		if oldHash != hash {
			return 0, nil, fail(409, "idempotency_conflict")
		}
		var out any
		err = json.Unmarshal(response, &out)
		return status, out, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, err
	}
	r.Body = io.NopCloser(strings.NewReader(string(raw)))
	status, result, err := perform()
	if err != nil {
		return 0, nil, err
	}
	_, err = tx.Exec(r.Context(), "INSERT INTO idempotency(actor_id,operation,key,request_hash,status,response) VALUES($1,$2,$3,$4,$5,$6)", a.AgentID, operation, key, hash, status, canonical(result))
	return status, result, err
}
