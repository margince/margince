// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether the caller's own connections on a set of providers feed this product,
// in the three answers a reader of a zero needs: fed, not connected, or
// connected and broken. Classified by connectionConcern, so this answer and the
// capture-health lane cannot disagree about which connection is unhealthy.

import (
	"context"
	"slices"
)

// Reach is how far a user's connections on some providers reach this product.
type Reach string

const (
	// ReachLive means a connection is delivering and none is broken.
	ReachLive Reach = "live"
	// ReachNone means no connection exists, or every one was parked by its owner.
	ReachNone Reach = "none"
	// ReachBroken means a connection wants reauthorisation, is in error, or its sync
	// is failing — what it would carry is missing without the user choosing so.
	ReachBroken Reach = "broken"
)

// ReachOn answers Reach for the CALLING human's connections on these providers.
// It reads through Connections, so it carries that read's human-only refusal.
func (r *Registry) ReachOn(ctx context.Context, providers []string) (Reach, error) {
	views, err := r.Connections(ctx)
	if err != nil {
		return "", err
	}
	return reachOf(views, providers), nil
}

// reachOf decides it over views Connections already read unarchived, which is
// why `status = connected` alone is liveConnection's rule here. A failed history
// import is not broken: the connection still delivers what happens from now on.
func reachOf(views []ConnectionView, providers []string) Reach {
	reach := ReachNone
	for _, view := range views {
		if !slices.Contains(providers, view.Provider) {
			continue
		}
		switch connectionConcern(view) {
		case ConcernReauthRequired, ConcernConnectionError, ConcernSyncFailing:
			return ReachBroken
		}
		if view.Status == statusConnected {
			reach = ReachLive
		}
	}
	return reach
}
