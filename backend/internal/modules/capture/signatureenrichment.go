// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// One mailbox's own answer to the nightly signature pass.
//
// The tenant default lives in the settings table (SignatureEnrich); this is the
// override beside it, and the two are separate on purpose: an organization can
// turn the whole thing off, or leave it on and let one mailbox opt out, without
// those being the same knob. It is the granularity a works-council negotiation
// asks for, which is why it ships before it is demanded.
//
// Tri-state, and the third state is the reason it is a pointer: nil hands the
// question back to the default, so a later change to that default moves this
// mailbox with it. A mailbox that answered keeps its answer whatever the
// default becomes.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SetSignatureEnrichment records this mailbox's answer and returns the
// connection as it now stands.
//
// Scoped to the CALLER'S OWN mailbox by the same `user_id` predicate every
// other read and write on this table uses: capture is per-user (RC-8), so a
// connection belonging to somebody else is not this caller's to change and does
// not exist as far as this statement is concerned.
func (r *Registry) SetSignatureEnrichment(ctx context.Context, name string, enabled *bool) (ConnectionView, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return ConnectionView{}, fmt.Errorf("capture: only a human sets a mailbox's signature-enrichment answer: %w", apperrors.ErrPermissionDenied)
	}
	if _, ok := r.connectors[name]; !ok {
		return ConnectionView{}, ErrNoConnection
	}

	var before *bool
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		// The prior answer comes out of the same statement that replaces it, so
		// the audit row's before-image is the row as it actually stood rather
		// than a value read in a separate query something else could have moved
		// in between.
		row := tx.QueryRow(ctx, `
			UPDATE capture_connection
			   SET signature_enrich_enabled = $3
			 WHERE user_id = $1 AND provider = $2 AND archived_at IS NULL
			RETURNING (SELECT c.signature_enrich_enabled
			             FROM capture_connection c WHERE c.id = capture_connection.id)`,
			actor.UserID, name, enabled)
		if err := row.Scan(&before); err != nil {
			return err
		}
		// Audit-only, like every other capture-configuration change beside it:
		// the closed public-event catalog carries no type for a posture, and
		// inventing one would put a mailbox's own setting on the outbound bus.
		_, err := storekit.Audit(ctx, tx, "update", captureSettingsObject, storekit.MustWorkspace(ctx),
			signatureEnrichmentImage(name, before), signatureEnrichmentImage(name, enabled))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ConnectionView{}, ErrNoConnection
	}
	if err != nil {
		return ConnectionView{}, fmt.Errorf("capture: setting the mailbox's signature-enrichment answer: %w", err)
	}
	return r.connectionFor(ctx, name)
}

// connectionFor reads back one of the caller's own connections, so the write
// above answers with the same shape the list surface serves rather than a
// second, thinner one a client would have to tell apart.
func (r *Registry) connectionFor(ctx context.Context, name string) (ConnectionView, error) {
	views, err := r.Connections(ctx)
	if err != nil {
		return ConnectionView{}, err
	}
	for _, v := range views {
		if v.Provider == name {
			return v, nil
		}
	}
	return ConnectionView{}, ErrNoConnection
}

// signatureEnrichmentImage is the audit row's view of the answer: the mailbox
// it belongs to, and what it says. `null` is spelled as a value rather than
// omitted, because "follows the default" is a choice somebody made and an
// absent key would read as a field the write did not touch.
func signatureEnrichmentImage(provider string, enabled *bool) map[string]any {
	return map[string]any{
		auditKeyProvider:           provider,
		"signature_enrich_enabled": enabled,
	}
}

// auditKeyProvider names the mailbox an audited posture change belongs to.
const auditKeyProvider = "provider"
