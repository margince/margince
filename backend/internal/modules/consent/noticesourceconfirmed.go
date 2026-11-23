// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auditattribution"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ConfirmedNoticeSource is the source a reviewer last confirmed for a contact.
type ConfirmedNoticeSource struct {
	CaseID      ids.UUID
	Kind        string
	ObtainedAt  *time.Time
	ConfirmedAt time.Time
	// ConfirmedBy is nil when the confirm's audit row is gone, or the reader
	// may not know who acted.
	ConfirmedBy *auditattribution.Attribution
}

// The confirm is the one audit row on a case that writes source_kind, so its
// newest such row says when and by whom the standing source was confirmed.
const confirmedNoticeSourceSQL = `
	SELECT n.id, n.source_kind, n.source_obtained_at, confirm.id, COALESCE(confirm.occurred_at, n.updated_at)
	  FROM privacy_notice_case n
	  LEFT JOIN LATERAL (
	    SELECT c.id, c.occurred_at FROM audit_log c
	     WHERE c.entity_type = 'privacy_notice_case' AND c.entity_id = n.id AND c.after ? 'source_kind'
	     ORDER BY c.occurred_at DESC, c.id DESC LIMIT 1) confirm ON true
	 WHERE n.contact_id = $1 AND n.source_kind IS NOT NULL
	 ORDER BY COALESCE(confirm.occurred_at, n.updated_at) DESC, n.id DESC
	 LIMIT 1`

const confirmerSQL = `SELECT ` + auditattribution.AttributionColumns + `
	  FROM audit_log a` + auditattribution.ActorNameJoins + auditattribution.AgentClientNameJoin + `
	 WHERE a.id = $1`

// ConfirmedNoticeSourceTx answers the contact's latest confirmed source, or
// nil when no source review was ever confirmed.
func (s *Store) ConfirmedNoticeSourceTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (*ConfirmedNoticeSource, error) {
	if err := requireNoticeSourceRead(ctx, tx, contactID); err != nil {
		return nil, err
	}
	var out ConfirmedNoticeSource
	var confirm *ids.UUID
	err := tx.QueryRow(ctx, confirmedNoticeSourceSQL, contactID).
		Scan(&out.CaseID, &out.Kind, &out.ObtainedAt, &confirm, &out.ConfirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no confirmed source, which the 360 says by omitting it
	}
	if err != nil {
		return nil, fmt.Errorf("read the confirmed notice source: %w", err)
	}
	if confirm == nil || !auditattribution.ReaderMayKnowWhoActed(ctx) {
		return &out, nil
	}
	var who auditattribution.Attribution
	if err := tx.QueryRow(ctx, confirmerSQL, *confirm).Scan(who.ScanTargets()...); err != nil {
		return nil, fmt.Errorf("read who confirmed the notice source: %w", err)
	}
	out.ConfirmedBy = &who
	return &out, nil
}
