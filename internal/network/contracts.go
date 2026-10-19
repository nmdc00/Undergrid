package network

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

type Owner struct {
	Issuer, Subject string
	Verified        bool
}

type AgentProfile struct {
	Public            bool     `json:"public"`
	AcceptInvitations bool     `json:"accept_invitations"`
	Recruitment       bool     `json:"recruitment"`
	Capabilities      []string `json:"capabilities"`
}
type actor struct {
	PrincipalID, AgentID, CredentialID string
	Scopes                             []string
	grantedScopes                      []string
}
type problem struct {
	Status int
	Code   string
}

func (p problem) Error() string          { return p.Code }
func fail(status int, code string) error { return problem{status, code} }
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func canonical(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func etag(v int64) string { return fmt.Sprintf("\"%d\"", v) }

type Output struct {
	Kind         string `json:"kind"`
	SchemaFamily string `json:"schema_family"`
}
type Constraints struct {
	RequiredFields      []string             `json:"required_fields"`
	LicenseIDs          []string             `json:"license_ids"`
	Coverage            *CoverageConstraint  `json:"coverage,omitempty"`
	Freshness           *FreshnessConstraint `json:"freshness,omitempty"`
	Delivery            *DeliveryConstraint  `json:"delivery,omitempty"`
	InputSchemaFamilies *[]string            `json:"input_schema_families,omitempty"`
}
type Watch struct {
	Enabled  bool     `json:"enabled"`
	Channels []string `json:"channels"`
}
type IntentInput struct {
	SemanticMinimumSimilarity *float64          `json:"semantic_minimum_similarity,omitempty"`
	Objective                 string            `json:"objective"`
	Output                    Output            `json:"output"`
	HardConstraints           Constraints       `json:"hard_constraints"`
	Preferences               map[string]string `json:"preferences,omitempty"`
	Visibility                string            `json:"visibility"`
	PublicSummary             string            `json:"public_summary,omitempty"`
	Budget                    *json.RawMessage  `json:"budget"`
	Watch                     Watch             `json:"watch"`
	ExpiresAt                 time.Time         `json:"expires_at"`
}

func (i IntentInput) validate() error {
	if v := i.SemanticMinimumSimilarity; v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 1) {
		return fail(422, "invalid_semantic_threshold")
	}
	if i.Budget != nil {
		if err := ValidateBudget(*i.Budget); err != nil {
			return err
		}
	}
	if len(strings.TrimSpace(i.Objective)) < 3 || len(i.Objective) > 4000 {
		return fail(422, "invalid_objective")
	}
	if i.Output.Kind != "json_dataset" || i.Output.SchemaFamily != "source_catalog_v1" {
		return fail(422, "unsupported_output")
	}
	if i.Visibility != "private" && i.Visibility != "restricted" && i.Visibility != "redacted_public" {
		return fail(422, "invalid_visibility")
	}
	if i.Visibility == "redacted_public" && (len(i.PublicSummary) < 3 || len(i.PublicSummary) > 1000) {
		return fail(422, "public_summary_required")
	}
	if err := i.HardConstraints.validate(time.Now()); err != nil {
		return err
	}
	if len(i.Watch.Channels) != 1 || i.Watch.Channels[0] != "inbox" {
		return fail(422, "unsupported_channel")
	}
	if !i.ExpiresAt.After(time.Now()) {
		return fail(422, "future_expiry_required")
	}
	return nil
}

// Network budgets express interest only. Financial authority belongs to the
// isolated sandbox and is never inferred from this field or a message.
func ValidateBudget(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var budget struct {
		Amount   int64  `json:"amount_minor"`
		Currency string `json:"currency"`
		Kind     string `json:"kind"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&budget); err != nil {
		return fail(422, "invalid_budget")
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF || budget.Amount < 0 || budget.Amount > 1_000_000_000_000 || budget.Kind != "stated_budget" || (budget.Currency != "EUR" && budget.Currency != "USD") {
		return fail(422, "invalid_budget")
	}
	return nil
}

func validID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil
}
