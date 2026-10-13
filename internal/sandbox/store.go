// Package sandbox owns TEST-only financial authority and facts. It never imports
// the network domain or receives its database connection or agent credentials.
package sandbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"math/big"
	"slices"
	"strings"
	"time"
)

var (
	ErrUnauthorized = errors.New("sandbox authorization required")
	ErrConflict     = errors.New("sandbox version, state or idempotency conflict")
	ErrLimit        = errors.New("sandbox policy limit exceeded")
	ErrInvalid      = errors.New("invalid sandbox command")
)

//go:embed migrations/001_authority.sql
var migration string

func Migrate(ctx context.Context, c *pgx.Conn) error {
	tx, err := c.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(74932519)"); err != nil {
		return err
	}
	var network bool
	if err = tx.QueryRow(ctx, "SELECT to_regnamespace('undergrid') IS NOT NULL").Scan(&network); err != nil {
		return err
	}
	if network {
		return errors.New("sandbox must not use network database")
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS public.undergrid_sandbox_migrations(version integer PRIMARY KEY, checksum text NOT NULL)"); err != nil {
		return err
	}
	var checksum string
	err = tx.QueryRow(ctx, "SELECT checksum FROM public.undergrid_sandbox_migrations WHERE version=1").Scan(&checksum)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err = tx.Exec(ctx, migration); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO public.undergrid_sandbox_migrations VALUES(1,$1)", hash([]byte(migration))); err != nil {
			return err
		}
	case err != nil:
		return err
	case checksum != hash([]byte(migration)):
		return errors.New("sandbox migration checksum mismatch")
	}
	if err = validateGroupRoles(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type Store struct{ dsn string }

func New(dsn string) *Store { return &Store{dsn: dsn} }

type Owner struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}
type Policy struct {
	Asset     string    `json:"asset"`
	Cap       string    `json:"cap_atomic"`
	Products  []string  `json:"products"`
	Payees    []string  `json:"payees"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Quote struct {
	Product   string    `json:"product"`
	Asset     string    `json:"asset"`
	Amount    string    `json:"amount_atomic"`
	Fee       string    `json:"fee_atomic"`
	Payee     string    `json:"payee"`
	Rail      string    `json:"rail"`
	TermsID   string    `json:"test_terms_id"`
	TermsHash string    `json:"test_terms_hash"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Payment struct {
	ID          string `json:"id"`
	Hash        string `json:"quote_hash"`
	State       string `json:"state"`
	Quote       Quote  `json:"quote"`
	Designation string `json:"designation"`
	ProviderKey string `json:"-"`
}
type identity struct {
	actor, credential string
	policy            Policy
	version           int64
	scopes            []string
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func positive(s string, zero bool) bool {
	if len(s) == 0 || len(s) > 78 || s[0] == '0' && len(s) > 1 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, _ := new(big.Int).SetString(s, 10)
	return n.Sign() > 0 || zero && n.Sign() == 0
}
func (s *Store) transaction(ctx context.Context, role string, f func(pgx.Tx) error) error {
	c, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	tx, err := c.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if role != "undergrid_sandbox_app" && role != "undergrid_sandbox_worker" {
		return ErrInvalid
	}
	if err = validateGroupRoles(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE "+role+"; SET LOCAL search_path=pg_catalog,undergrid_sandbox; SET LOCAL statement_timeout='5s'"); err != nil {
		return err
	}
	if err = f(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func ownerContext(ctx context.Context, tx pgx.Tx, o Owner) (identity, error) {
	var a identity
	if o.Issuer == "" || o.Subject == "" {
		return a, ErrUnauthorized
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('undergrid_sandbox.owner',$1,true)", hash(encode(o))); err != nil {
		return a, err
	}
	var id *string
	if err := tx.QueryRow(ctx, "SELECT identify_owner()").Scan(&id); err != nil {
		return a, err
	}
	if id == nil {
		return a, ErrUnauthorized
	}
	a.actor = *id
	_, err := tx.Exec(ctx, "SELECT set_config('undergrid_sandbox.actor',$1,true)", a.actor)
	if err != nil {
		return a, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT policy,version FROM sandbox_actors WHERE id=$1 FOR UPDATE", a.actor).Scan(&raw, &a.version)
	if err == nil {
		err = json.Unmarshal(raw, &a.policy)
	}
	return a, err
}
func agentContext(ctx context.Context, tx pgx.Tx, token, scope string) (identity, error) {
	var a identity
	if !strings.HasPrefix(token, "ugs_") {
		return a, ErrUnauthorized
	}
	var id *string
	if err := tx.QueryRow(ctx, "SELECT identify_token($1)", hash([]byte(token))).Scan(&id); err != nil {
		return a, err
	}
	if id == nil {
		return a, ErrUnauthorized
	}
	a.actor = *id
	if _, err := tx.Exec(ctx, "SELECT set_config('undergrid_sandbox.actor',$1,true)", a.actor); err != nil {
		return a, err
	}
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT policy,version FROM sandbox_actors WHERE id=$1 FOR UPDATE", a.actor).Scan(&raw, &a.version)
	if err != nil {
		return a, err
	}
	if err = json.Unmarshal(raw, &a.policy); err != nil {
		return a, err
	}
	err = tx.QueryRow(ctx, "SELECT id::text,scopes FROM sandbox_credentials WHERE digest=$1 AND revoked_at IS NULL AND expires_at>now() AND audience='undergrid-sandbox' FOR UPDATE", hash([]byte(token))).Scan(&a.credential, &a.scopes)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !slices.Contains(a.scopes, scope) {
		return a, ErrUnauthorized
	}
	return a, err
}
func (s *Store) Enroll(ctx context.Context, o Owner) (string, error) {
	id := randomID()
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		if o.Issuer == "" || o.Subject == "" {
			return ErrUnauthorized
		}
		owner := hash(encode(o))
		if _, err := tx.Exec(ctx, "SELECT set_config('undergrid_sandbox.owner',$1,true)", owner); err != nil {
			return err
		}
		return tx.QueryRow(ctx, "INSERT INTO sandbox_actors(id,owner_hash) VALUES($1,$2) ON CONFLICT(owner_hash) DO UPDATE SET version=sandbox_actors.version RETURNING id::text", id, owner).Scan(&id)
	})
	return id, err
}
func (s *Store) SetPolicy(ctx context.Context, o Owner, expected int64, p Policy) error {
	if !positive(p.Cap, true) || p.Asset != "USD" || len(p.Products) < 1 || len(p.Products) > 10 || len(p.Payees) < 1 || len(p.Payees) > 10 || !p.ExpiresAt.After(time.Now()) || p.ExpiresAt.After(time.Now().Add(90*24*time.Hour)) {
		return ErrInvalid
	}
	return s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := ownerContext(ctx, tx, o)
		if err != nil {
			return err
		}
		if expected != a.version {
			return ErrConflict
		}
		_, err = tx.Exec(ctx, "UPDATE sandbox_actors SET policy=$2,version=version+1 WHERE id=$1", a.actor, encode(p))
		return err
	})
}
func (s *Store) IssueCredential(ctx context.Context, o Owner, name string, scopes []string) (string, error) {
	if name == "" || len(name) > 128 || len(scopes) == 0 || len(scopes) > 3 {
		return "", ErrInvalid
	}
	for _, scope := range scopes {
		if !slices.Contains([]string{"payments:quote", "payments:authorize", "payments:read"}, scope) {
			return "", ErrInvalid
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := "ugs_" + hex.EncodeToString(b)
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := ownerContext(ctx, tx, o)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO sandbox_credentials(id,actor_id,agent_name,digest,audience,scopes,expires_at) VALUES($1,$2,$3,$4,'undergrid-sandbox',$5,now()+interval '30 days')", randomID(), a.actor, name, hash([]byte(token)), scopes)
		return err
	})
	if err != nil {
		return "", err
	}
	return token, nil
}
func (s *Store) RevokeCredentials(ctx context.Context, o Owner) error {
	return s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := ownerContext(ctx, tx, o)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE sandbox_credentials SET revoked_at=now() WHERE actor_id=$1 AND revoked_at IS NULL", a.actor)
		return err
	})
}
func loadPayment(ctx context.Context, tx pgx.Tx, id string) (Payment, error) {
	p := Payment{Designation: "TEST — NOT A LIVE COMMITMENT"}
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT id::text,quote_hash,state,quote,provider_key FROM sandbox_payments WHERE id=$1", id).Scan(&p.ID, &p.Hash, &p.State, &raw, &p.ProviderKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrUnauthorized
	}
	if err == nil {
		err = json.Unmarshal(raw, &p.Quote)
	}
	return p, err
}
func replay(ctx context.Context, tx pgx.Tx, a identity, op, key, digest string) (string, error) {
	if key == "" || len(key) > 128 {
		return "", ErrInvalid
	}
	var id, old string
	err := tx.QueryRow(ctx, "SELECT payment_id::text,request_hash FROM sandbox_idempotency WHERE credential_id=$1 AND operation=$2 AND key=$3", a.credential, op, key).Scan(&id, &old)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err == nil && old != digest {
		return "", ErrConflict
	}
	return id, err
}
func remember(ctx context.Context, tx pgx.Tx, a identity, op, key, digest, id string) error {
	_, err := tx.Exec(ctx, "INSERT INTO sandbox_idempotency VALUES($1,$2,$3,$4,$5,$6)", a.actor, a.credential, op, key, digest, id)
	return err
}
func (s *Store) CreateQuote(ctx context.Context, token, key string, q Quote) (Payment, error) {
	return s.createQuote(ctx, token, key, q, false)
}

// Catalog quotes derive expiry at the server. Retries bind the fixed catalog
// offer but preserve the first expiry rather than generating a new quote.
func (s *Store) createCatalogQuote(ctx context.Context, token, key string, q Quote) (Payment, error) {
	return s.createQuote(ctx, token, key, q, true)
}
func (s *Store) createQuote(ctx context.Context, token, key string, q Quote, catalog bool) (Payment, error) {
	var p Payment
	if q.Rail != "stripe_test" || q.Asset != "USD" || !positive(q.Amount, false) || !positive(q.Fee, true) || q.Product == "" || q.Payee == "" || q.TermsID == "" || len(q.TermsHash) != 64 || !q.ExpiresAt.After(time.Now()) || q.ExpiresAt.After(time.Now().Add(24*time.Hour)) {
		return p, ErrInvalid
	}
	if _, err := hex.DecodeString(q.TermsHash); err != nil {
		return p, ErrInvalid
	}
	q.ExpiresAt = q.ExpiresAt.UTC()
	raw := encode(q)
	digest := hash(raw)
	if catalog {
		offer := q
		offer.ExpiresAt = time.Time{}
		digest = hash(encode(offer))
	}
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := agentContext(ctx, tx, token, "payments:quote")
		if err != nil {
			return err
		}
		id, err := replay(ctx, tx, a, "quote", key, digest)
		if err != nil {
			return err
		}
		if id != "" {
			p, err = loadPayment(ctx, tx, id)
			return err
		}
		id = randomID()
		// Quote identity binds actor and credential, so identical offers to two agents are not interchangeable authorizations.
		quoteHash := hash(encode(struct {
			Quote                    Quote
			Actor, Credential, Nonce string
		}{q, a.actor, a.credential, id}))
		_, err = tx.Exec(ctx, "INSERT INTO sandbox_payments(id,actor_id,credential_id,quote,quote_hash,asset,amount,fee,provider_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", id, a.actor, a.credential, raw, quoteHash, q.Asset, q.Amount, q.Fee, "ugs-"+id)
		if err != nil {
			return err
		}
		if err = remember(ctx, tx, a, "quote", key, digest, id); err != nil {
			return err
		}
		p, err = loadPayment(ctx, tx, id)
		return err
	})
	return p, err
}
func (s *Store) GetPayment(ctx context.Context, token, id string) (Payment, error) {
	var p Payment
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		if _, err := agentContext(ctx, tx, token, "payments:read"); err != nil {
			return err
		}
		var err error
		p, err = loadPayment(ctx, tx, id)
		return err
	})
	return p, err
}
func permits(p Policy, q Quote) bool {
	return p.ExpiresAt.After(time.Now()) && p.Asset == q.Asset && slices.Contains(p.Products, q.Product) && slices.Contains(p.Payees, q.Payee)
}
func (s *Store) Authorize(ctx context.Context, token, id, quoteHash, key string) (Payment, error) {
	var p Payment
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := agentContext(ctx, tx, token, "payments:authorize")
		if err != nil {
			return err
		}
		digest := hash(encode([]string{id, quoteHash}))
		old, err := replay(ctx, tx, a, "authorize", key, digest)
		if err != nil {
			return err
		}
		if old != "" {
			p, err = loadPayment(ctx, tx, old)
			return err
		}
		p, err = loadPayment(ctx, tx, id)
		if err != nil {
			return err
		}
		var credential string
		if err = tx.QueryRow(ctx, "SELECT credential_id::text FROM sandbox_payments WHERE id=$1", id).Scan(&credential); err != nil {
			return err
		}
		if credential != a.credential || p.Hash != quoteHash || p.State != "quoted" || !p.Quote.ExpiresAt.After(time.Now()) {
			return ErrConflict
		}
		if !permits(a.policy, p.Quote) {
			return ErrUnauthorized
		}
		var allowed bool
		err = tx.QueryRow(ctx, `SELECT COALESCE(sum(amount+fee),0)+$2::numeric+$3::numeric <= $4::numeric FROM sandbox_payments WHERE actor_id=$1 AND asset=$5 AND reservation_period=(now() AT TIME ZONE 'UTC')::date AND state IN('authorized','submitting','pending','unknown','settled')`, a.actor, p.Quote.Amount, p.Quote.Fee, a.policy.Cap, p.Quote.Asset).Scan(&allowed)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrLimit
		}
		_, err = tx.Exec(ctx, "UPDATE sandbox_payments SET state='authorized',policy_version=$2,authority_proof=$3,reservation_period=(now() AT TIME ZONE 'UTC')::date,updated_at=now() WHERE id=$1", id, a.version, encode(map[string]any{"actor": a.actor, "credential": a.credential, "quote_hash": quoteHash, "policy_version": a.version, "designation": "TEST"}))
		if err != nil {
			return err
		}
		if err = remember(ctx, tx, a, "authorize", key, digest, id); err != nil {
			return err
		}
		p.State = "authorized"
		return nil
	})
	return p, err
}

func (s *Store) GetPolicy(ctx context.Context, o Owner) (Policy, int64, error) {
	var policy Policy
	var version int64
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := ownerContext(ctx, tx, o)
		policy, version = a.policy, a.version
		return err
	})
	return policy, version, err
}

// Group roles must never provide a login or bypass the application RLS contract.
// The runtime repeats this check because role attributes can change after migration.
type roleCatalog interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func groupRoleSafe(ctx context.Context, q roleCatalog, name string) (bool, error) {
	var safe bool
	err := q.QueryRow(ctx, `SELECT NOT (rolsuper OR rolbypassrls OR rolcanlogin OR rolcreaterole OR rolcreatedb OR rolreplication)
 AND NOT CASE WHEN to_regrole('undergrid_app') IS NOT NULL THEN pg_has_role($1,'undergrid_app','MEMBER') ELSE false END
 AND NOT CASE WHEN to_regrole('undergrid_worker') IS NOT NULL THEN pg_has_role($1,'undergrid_worker','MEMBER') ELSE false END
 FROM pg_roles WHERE rolname=$1`, name).Scan(&safe)
	return safe, err
}
func validateGroupRoles(ctx context.Context, q roleCatalog) error {
	for _, name := range []string{"undergrid_sandbox_app", "undergrid_sandbox_worker"} {
		safe, err := groupRoleSafe(ctx, q, name)
		if err != nil || !safe {
			return errors.New("sandbox group roles require NOLOGIN and no elevated privileges")
		}
	}
	return nil
}
func ValidateGroupRoles(ctx context.Context, c *pgx.Conn) error { return validateGroupRoles(ctx, c) }
