// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The capture-health read: the caller's own connections that need the
// caller's hand — a mailbox wanting re-authentication, a connection in
// error, a sync failing, a backfill that ended in error. One concern per
// CONNECTION, carrying the worst condition on it, so a user with one broken
// mailbox sees one card. Derived entirely from Connections' own view — the
// same rows the settings screen lists — so this read and that screen cannot
// come to disagree about which mailbox is unhealthy.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The capture-concern vocabulary, worst first: a connection carries the first
// of these that applies and no other.
const (
	// ConcernReauthRequired: the provider revoked or expired the
	// grant, and only the user can renew it.
	ConcernReauthRequired = "reauth_required"
	// ConcernConnectionError: the connection is in its error state —
	// still selectable for sync, but its last attempts did not deliver.
	ConcernConnectionError = "connection_error"
	// ConcernSyncFailing: the connection reads as connected but its
	// sync sidecar recorded a failure class on the last attempt.
	ConcernSyncFailing = "sync_failing"
	// ConcernBackfillFailed: the latest history import ended in error.
	ConcernBackfillFailed = "backfill_failed"
)

// Connection statuses and the backfill terminal state this read derives from
// (capture_connection.status / capture_backfill.status vocabularies).
const (
	statusReauthRequired = "reauth_required"
	statusError          = "error"
	statusDisconnected   = "disconnected"
	backfillStatusError  = "error"
)

// Concern is one connection needing its owner's hand.
type Concern struct {
	ConnectionID ids.UUID
	Kind         string
	Provider     string
	// AccountLabel is the display-only mailbox address when the connector
	// reported one; empty otherwise. Display, never routing.
	AccountLabel string
	// ErrorClass is the sync sidecar's failure class where one was recorded.
	ErrorClass string
}

// HealthConcerns answers the CALLING human's unhealthy connections, in the
// order Connections lists them. Capture is per-user (RC-8), so there is no
// admin view here: each user is shown their own mailboxes and nobody else's.
// A principal with no human behind it has no mailboxes of its own, which is a
// refusal rather than an empty answer — the caller renders it as withheld.
func (r *Registry) HealthConcerns(ctx context.Context) ([]Concern, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return nil, apperrors.ErrPermissionDenied
	}
	views, err := r.Connections(ctx)
	if err != nil {
		return nil, err
	}
	var concerns []Concern
	for _, view := range views {
		kind, errorClass := connectionConcern(view)
		if kind == "" {
			continue
		}
		concern := Concern{
			ConnectionID: view.ID,
			Kind:         kind,
			Provider:     view.Provider,
			ErrorClass:   errorClass,
		}
		if view.AccountLabel != nil {
			concern.AccountLabel = *view.AccountLabel
		}
		concerns = append(concerns, concern)
	}
	return concerns, nil
}

// connectionConcern classifies one connection: the worst condition on it, or
// "" for a healthy one. A deliberately disconnected connection raises nothing
// — the user chose that state, and a card would nag them about their own
// decision.
func connectionConcern(view ConnectionView) (kind, errorClass string) {
	switch view.Status {
	case statusDisconnected:
		// A parked connection keeps its last sidecar facts; nagging about a
		// mailbox the user turned off would be reporting their own decision.
		return "", ""
	case statusReauthRequired:
		return ConcernReauthRequired, ""
	case statusError:
		if view.LastErrorClass != nil {
			return ConcernConnectionError, *view.LastErrorClass
		}
		return ConcernConnectionError, ""
	}
	if view.LastErrorClass != nil {
		return ConcernSyncFailing, *view.LastErrorClass
	}
	if view.Backfill != nil && view.Backfill.Status == backfillStatusError {
		return ConcernBackfillFailed, ""
	}
	return "", ""
}
