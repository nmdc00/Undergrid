package network

import (
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type invitationInput struct {
	AgentID       string    `json:"agent_id"`
	Role          string    `json:"role"`
	RespondBy     time.Time `json:"respond_by"`
	ApplicationID string    `json:"application_id,omitempty"`
}

// Current target authority precedes retained receipt recovery. The caller holds
// current coordinator/room authority; NOWAIT foreign locks avoid actor-lock cycles.
func requireApplicationOfferTarget(r *http.Request, tx pgx.Tx, swarm string, input invitationInput) error {
	if input.ApplicationID == "" {
		return nil
	}
	if !validID(input.ApplicationID) || !validID(input.AgentID) {
		return fail(422, "invalid_application")
	}
	var current bool
	if err := tx.QueryRow(r.Context(), `SELECT lock_application_offer_target($1,$2,$3)`, swarm, input.ApplicationID, input.AgentID).Scan(&current); err != nil {
		return err
	}
	if !current {
		return fail(409, "application_not_current")
	}
	return nil
}
func preauthorizeApplicationOffer(r *http.Request, tx pgx.Tx, swarm string) error {
	request, err := readIdempotencyRequest(r, true)
	if err != nil {
		return err
	}
	restoreIdempotencyBody(r, request)
	var input invitationInput
	if err = decode(r, &input); err != nil {
		return err
	}
	restoreIdempotencyBody(r, request)
	return requireApplicationOfferTarget(r, tx, swarm, input)
}
