package network

import (
	"bytes"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"time"
)

func (s *Service) ownerApplicationOffer(r *http.Request, tx pgx.Tx, a actor, swarm, application string) (int, any, error) {
	if err := roomAuthority(r.Context(), tx, a, swarm, true); err != nil {
		return 0, nil, err
	}
	request, err := readIdempotencyRequest(r, true)
	if err != nil {
		return 0, nil, err
	}
	restoreIdempotencyBody(r, request)
	var input struct {
		ApplicationVersion int64     `json:"application_version"`
		Role               string    `json:"role"`
		RespondBy          time.Time `json:"respond_by"`
		HistoryPolicy      string    `json:"history_policy"`
	}
	if err = decode(r, &input); err != nil {
		return 0, nil, err
	}
	restoreIdempotencyBody(r, request)
	if input.HistoryPolicy != "full" {
		return 0, nil, fail(422, "full_history_required")
	}
	if input.ApplicationVersion < 1 {
		return 0, nil, fail(422, "invalid_application_version")
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	var target string
	err = tx.QueryRow(r.Context(), `SELECT agent_id FROM memberships WHERE id=$1 AND swarm_id=$2 AND origin='application'`, application, swarm).Scan(&target)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	invitation := invitationInput{AgentID: target, ApplicationID: application, Role: input.Role, RespondBy: input.RespondBy}
	if err = requireApplicationOfferTarget(r, tx, swarm, invitation); err != nil {
		return 0, nil, err
	}
	return idempotent(r, tx, a, func() (int, any, error) {
		var roomVersion, applicationVersion int64
		if err := tx.QueryRow(r.Context(), `SELECT version FROM swarms WHERE id=$1`, swarm).Scan(&roomVersion); err != nil {
			return 0, nil, err
		}
		if r.Header.Get("If-Match") != etag(roomVersion) {
			return 0, nil, fail(409, "version_conflict")
		}
		if err := tx.QueryRow(r.Context(), `SELECT version FROM memberships WHERE id=$1 AND swarm_id=$2 FOR UPDATE NOWAIT`, application, swarm).Scan(&applicationVersion); err != nil {
			return 0, nil, err
		}
		if input.ApplicationVersion != applicationVersion {
			return 0, nil, fail(409, "application_version_conflict")
		}
		command := r.Clone(r.Context())
		command.Body = io.NopCloser(bytes.NewReader(canonical(invitation)))
		return s.invite(command, tx, a, swarm)
	})
}
