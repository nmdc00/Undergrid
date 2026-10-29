package network

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Signature validation precedes database locks. Local credential/policy validity
// is still rechecked by authenticateAgentRequest in the command transaction.
func (s *Service) verifyNetworkRequest(r *http.Request) (*http.Request, error) {
	if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/artifacts/") && strings.HasSuffix(r.URL.Path, "/upload") {
		return r, nil
	}
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") {
		return r, nil
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	if token == "" || strings.HasPrefix(token, "ug_") {
		return r, nil
	}
	if s.oauth == nil {
		return r, fail(401, "invalid_credential")
	}
	verified, err := s.oauth.verify(r.Context(), token)
	if err != nil {
		return r, fail(401, "invalid_credential")
	}
	verified.Resource = s.oauth.resource
	return r.WithContext(context.WithValue(r.Context(), oauthResourceContextKey, verified)), nil
}

// Callers own tx setup and commit. Protocol receipts can be appended in this
// same transaction; no adapter may execute then persist a receipt separately.
func (s *Service) executeAgentCommandTx(r *http.Request, tx pgx.Tx) (actor, int, any, error) {
	a, err := authenticateAgentRequest(r, tx)
	if err != nil {
		return a, 0, nil, err
	}
	status, result, err := s.agentCommand(r, tx, a)
	return a, status, result, err
}

func authenticateAgentRequest(r *http.Request, tx pgx.Tx) (actor, error) {
	if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/artifacts/") && strings.HasSuffix(r.URL.Path, "/upload") {
		return authenticateArtifactUpload(r, tx)
	}
	return authenticate(r.Context(), tx, r.Header.Get("Authorization"), r.Method, r.URL.Path)
}

func (s *Service) beginNetworkTransaction(ctx context.Context) (*pgx.Conn, pgx.Tx, error) {
	db, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return nil, nil, err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		db.Close(ctx)
		return nil, nil, err
	}
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE undergrid_app; SET LOCAL search_path=undergrid,pg_catalog; SET LOCAL statement_timeout='5s'"); err != nil {
		tx.Rollback(ctx)
		db.Close(ctx)
		return nil, nil, err
	}
	return db, tx, nil
}
