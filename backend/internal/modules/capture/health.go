// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Owner and fleet health share one classification of connection state.
// Silence is measured at the sync boundary, without reading correspondence.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	ConcernSyncStalled    = "sync_stalled"
	ConcernCaptureSilent  = "capture_silent"
)

// Connection statuses and the backfill terminal state this read derives from
// (capture_connection.status / capture_backfill.status vocabularies).
const (
	statusConnected      = "connected"
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

	// FailingSince is when this concern's failure streak began, nil for a
	// concern that is not a streak — a parked connection is a state, not a
	// duration. It is what separates "is anything wrong" from "has anything
	// been wrong for a while": a sync that postpones itself is never late by
	// any age reading, so a fleet screen reading only the condition cannot tell
	// an hour-old outage from a connection idling between ticks.
	FailingSince *time.Time
	// LastSyncedAt is the connection's last good sync, nil when it never had one.
	LastSyncedAt *time.Time
	// ChangedAt is the evidence that can reopen an attention card marked Done.
	ChangedAt *time.Time
}

// HealthConcerns reads the owner's concerns. Admin and ops use FleetConcernsTx
// behind job_health:read; neither read needs credentials or correspondence.
func (r *Registry) HealthConcerns(ctx context.Context) ([]Concern, error) {
	views, err := r.Connections(ctx)
	if err != nil {
		return nil, err
	}
	var concerns []Concern
	for _, view := range views {
		kind := view.Concern
		if kind == "" {
			continue
		}
		concern := Concern{
			ConnectionID: view.ID,
			Kind:         kind,
			Provider:     view.Provider,
		}
		if view.AccountLabel != nil {
			concern.AccountLabel = *view.AccountLabel
		}
		concern.FailingSince = view.ConcernSince
		concern.LastSyncedAt = view.LastSuccessAt
		concern.ChangedAt = view.LastSuccessAt
		if kind == ConcernCaptureSilent {
			concern.ChangedAt = view.ConcernSince
		}
		concerns = append(concerns, concern)
	}
	return concerns, nil
}

// ConcernKinds orders simultaneous concerns by their displayed priority.
var ConcernKinds = []string{
	ConcernReauthRequired, ConcernConnectionError, ConcernSyncFailing,
	ConcernBackfillFailed, ConcernSyncStalled, ConcernCaptureSilent,
}

const (
	stalledAfter   = 24 * time.Hour
	firstSyncGrace = time.Hour
	silentAfter    = 72 * time.Hour
)

func connectionConcern(view ConnectionView, now time.Time) string {
	switch view.Status {
	case statusDisconnected:
		return ""
	case statusReauthRequired:
		return ConcernReauthRequired
	case statusError:
		return ConcernConnectionError
	}
	if view.LastErrorClass != nil {
		return ConcernSyncFailing
	}
	if view.Backfill != nil && view.Backfill.Status == backfillStatusError {
		return ConcernBackfillFailed
	}
	if view.Status != statusConnected || view.ConnectedAt.IsZero() {
		return ""
	}
	if syncStalled(view, now) {
		return ConcernSyncStalled
	}
	if !DeliversMail(view.Provider) {
		return ""
	}
	since := deliverySince(view)
	if silenceDismissed(view) {
		return ""
	}
	if now.Sub(since) > silentAfter && now.Sub(view.ConnectedAt) > silentAfter {
		return ConcernCaptureSilent
	}
	return ""
}

func deliverySince(view ConnectionView) time.Time {
	if view.LastDeliveryAt != nil {
		return *view.LastDeliveryAt
	}
	return view.ConnectedAt
}

func classifyConnection(view *ConnectionView, now time.Time) {
	view.SyncStalled = syncStalled(*view, now)
	view.Concern = connectionConcern(*view, now)
	view.ConcernSince = view.FailingSince
	switch view.Concern {
	case "":
		view.ConcernSince = nil
	case ConcernBackfillFailed:
		view.ConcernSince = view.Backfill.CompletedAt
	case ConcernSyncStalled:
		since := view.ConnectedAt
		if view.LastSuccessAt != nil && view.LastSuccessAt.After(since) {
			since = *view.LastSuccessAt
		}
		view.ConcernSince = &since
	case ConcernCaptureSilent:
		since := deliverySince(*view)
		view.ConcernSince = &since
	}
}

func syncStalled(view ConnectionView, now time.Time) bool {
	return view.Status == statusConnected && !view.ConnectedAt.IsZero() &&
		now.Sub(view.ConnectedAt) > firstSyncGrace &&
		(view.LastSuccessAt == nil || now.Sub(*view.LastSuccessAt) > stalledAfter)
}

func silenceDismissed(view ConnectionView) bool {
	return view.SilenceDismissedSince != nil && (view.LastDeliveryAt == nil || view.LastDeliveryAt.Equal(*view.SilenceDismissedSince))
}
