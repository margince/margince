// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// assertedProvenancePrefix marks a row whose content a caller STATED rather
// than a connector READ. storekit.CapturedBy stamps an ordinary API call
// `human:<uuid>`; connectorProvenance stamps a capture `connector:<name>:<uuid>`.
// Both are server-derived from the authenticated principal, so neither can be
// set from a request body — which is what makes the distinction trustworthy.
const assertedProvenancePrefix = "human:"

// AssertedTakeOver writes a connector's reading of a message over a row a
// caller asserted. activities.TakeOverAssertedActivityTx is the implementation;
// compose injects it, because capture never imports a sibling — the same shape
// AudienceRecomputer travels.
type AssertedTakeOver func(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	subject, body, wasCapturedBy, capturedBy string,
) error

// WithAssertedTakeOver returns a copy that lets a connector take over a row an
// importer asserted. A sink without one leaves an asserted incumbent alone,
// which is the behaviour that predates the take-over.
func (s *Sink) WithAssertedTakeOver(take AssertedTakeOver) *Sink {
	out := *s
	out.takeOverAsserted = take
	return &out
}

// assertedIncumbent answers whether the row a capture just collided with was
// ASSERTED by a caller rather than OBSERVED by a connector.
//
// An import states what its source system remembered about a message. A
// connector read the message itself. When both describe one Message-ID the
// connector's copy is the better one — an exporting CRM stores its own
// rendering, with the HTML stripped, the quoted history rewritten and inline
// images dropped.
//
// This also decides a safety question, which is why it tests provenance rather
// than trusting the collision. A Message-ID is a header the sender types, so
// any caller can post an activity under one they guessed. Were the connector to
// yield to such a row, the real message would never land and the forger would
// have suppressed somebody else's mail. Taking the row over instead makes a
// planted row a nuisance rather than a denial: the genuine content overwrites
// it on arrival.
// It returns the provenance it read as well as its verdict, because the
// take-over needs that string for its audit's before-image and this is the one
// place that has already paid for reading it.
func assertedIncumbent(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (string, bool, error) {
	var capturedBy string
	if err := tx.QueryRow(ctx,
		`SELECT captured_by FROM activity WHERE id = $1`, id).Scan(&capturedBy); err != nil {
		return "", false, fmt.Errorf("capture: reading whether %s was asserted: %w", id, err)
	}
	return capturedBy, strings.HasPrefix(capturedBy, assertedProvenancePrefix), nil
}
