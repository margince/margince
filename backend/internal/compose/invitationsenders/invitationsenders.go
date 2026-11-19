// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package invitationsenders is the data task that names the colleague who sent
// a received invitation stored before capture recorded the sender.
package invitationsenders

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/storedmail"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask parses each candidate's stored original again and asks capture's
// own rule (capture.InvitationSenderTx). A row whose original is gone is
// never guessed at.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795094620_invitation_senders", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795094620_invitation_senders"),
		Requires: []string{"core/1792969126"},
		Steps: []datatask.Step{{
			Name: "name_invitation_senders", Limit: 100, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimate, Batch: batch,
		}},
	}
}

// scope is received Gmail invitations that name no sender and keep their
// original. The sender rule needs Gmail's authentication result, which the
// other transports do not carry.
var scope = `FROM activity a JOIN raw_capture rc ON ` + storekit.StoredOriginalSQL("a", "rc") + `
 WHERE a.kind = 'email' AND a.direction = 'inbound' AND a.has_calendar_part
   AND a.invitation_sender IS NULL AND a.sent_by_colleague IS NULL
   AND a.archived_at IS NULL AND a.restricted_at IS NULL AND a.counterparty_email IS NOT NULL
   AND split_part(a.captured_by, ':', 1) = 'connector' AND split_part(a.captured_by, ':', 2) = 'gmail'`

// importerSQL is the seat whose mailbox captured the row, from its provenance.
const importerSQL = `CASE WHEN split_part(a.captured_by, ':', 3) ~ '^[0-9a-f-]{36}$'
	THEN split_part(a.captured_by, ':', 3)::uuid END`

type storedInvitation struct {
	ID       ids.UUID
	Sender   string
	Importer *ids.UUID
	Payload  []byte
}

func estimate(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	return storedmail.Count(ctx, q, scope, maximum)
}

func batch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	return storedmail.Page(ctx, tx, in, "a.id, a.counterparty_email, "+importerSQL+", "+originalstore.SQL("rc"), scope,
		func(row *storedInvitation) (ids.UUID, *[]byte) { return row.ID, &row.Payload },
		func(ctx context.Context, tx pgx.Tx, row storedInvitation) (bool, error) {
			return nameSender(ctx, tx, row, in.RunID)
		})
}

func nameSender(ctx context.Context, tx pgx.Tx, row storedInvitation, runID ids.UUID) (bool, error) {
	raw, err := capture.DecodeStoredOriginal(row.Payload)
	if err != nil {
		return false, nil //nolint:nilerr // An unreadable stored original is not evidence of a sender.
	}
	parsed, err := mailmap.Parse(raw, "")
	if err != nil {
		return false, nil //nolint:nilerr // An unparseable stored original is not evidence of a sender.
	}
	// The original must be this row's message: its From is the stored counterparty.
	if !strings.EqualFold(parsed.Counterparty(), row.Sender) {
		return false, nil
	}
	importer := ids.Nil
	if row.Importer != nil {
		importer = *row.Importer
	}
	seat, err := capture.InvitationSenderTx(ctx, tx, parsed.ToRecord("gmail", raw), importer)
	if err != nil || seat.IsZero() {
		return false, err
	}
	return activities.StampInvitationSenderTx(ctx, tx, row.ID, seat, runID)
}
