// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PromiseBasis is one open promise: the message it was made in and the
// contact it was made to.
type PromiseBasis struct {
	PromiseID          ids.UUID
	SourceActivityID   ids.UUID
	RecipientContactID ids.UUID
}

// PromiseFollowUp is our later message that may have kept a promise.
type PromiseFollowUp struct {
	EmailID   ids.UUID
	At        time.Time
	Precision string
	Subject   string
}

// PromiseFollowUpsTx finds, per promise, our newest mail on the promise's own
// thread to the contact it was made to. It was sent after the promise and by asOf.
//
// The answer only raises "Done?"; it never settles the promise. So an inbound
// message, our mail to someone else, a Bcc copy and other threads never count.
// A promise with no thread has no follow-up. The card cites the mail, so it
// must be one the reader may open.
func PromiseFollowUpsTx(ctx context.Context, tx pgx.Tx, promises []PromiseBasis, asOf time.Time) (map[ids.UUID]PromiseFollowUp, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	found := map[ids.UUID]PromiseFollowUp{}
	if len(promises) == 0 {
		return found, nil
	}
	promiseIDs := make([]ids.UUID, 0, len(promises))
	sources := make([]ids.UUID, 0, len(promises))
	recipients := make([]ids.UUID, 0, len(promises))
	for _, promise := range promises {
		promiseIDs = append(promiseIDs, promise.PromiseID)
		sources = append(sources, promise.SourceActivityID)
		recipients = append(recipients, promise.RecipientContactID)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	basis := fmt.Sprintf(`unnest($%d::uuid[], $%d::uuid[], $%d::uuid[]) AS basis(promise_id, source_id, recipient_id)`,
		arg(promiseIDs), arg(sources), arg(recipients))
	until := arg(asOf)
	readable, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if readable == "" {
		readable = scopeUnbounded
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT basis.promise_id, later.id, later.occurred_at, later.occurred_precision, later.subject
  FROM %[1]s
  JOIN activity src ON src.id = basis.source_id AND src.thread_key IS NOT NULL
  CROSS JOIN LATERAL (
    SELECT a.id, a.occurred_at, a.occurred_precision, coalesce(a.subject, '') AS subject FROM activity a
     WHERE %[2]s
       AND %[3]s
       AND %[4]s
     ORDER BY a.occurred_at DESC, a.id DESC
     LIMIT 1) later`, basis, promiseFollowUpSQL("a", "src", fmt.Sprintf("$%d", until)),
		promiseRecipientAddressedSQL("a", "basis.recipient_id"), readable), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading mail that may have kept a promise: %w", err)
	}
	var promise ids.UUID
	var sent PromiseFollowUp
	if _, err := pgx.ForEachRow(rows, []any{&promise, &sent.EmailID, &sent.At, &sent.Precision, &sent.Subject}, func() error {
		found[promise] = sent
		return nil
	}); err != nil {
		return nil, fmt.Errorf("activities: reading mail that may have kept a promise: %w", err)
	}
	return found, nil
}

// promiseFollowUpSQL admits the mail under later as our follow-up on the
// promise made in src. It is our attested, non-bulk mail later on src's thread,
// sent by until. The scan's settled citations read the same rule.
func promiseFollowUpSQL(later, src, until string) string {
	return fmt.Sprintf(`%[1]s.thread_key = %[2]s.thread_key
       AND %[1]s.kind = %[2]s.kind AND %[1]s.kind = 'email'
       AND %[1]s.channel_provider IS NOT DISTINCT FROM %[2]s.channel_provider
       AND %[1]s.direction = 'outbound' AND %[1]s.counterparty_outbound_attested AND NOT %[1]s.bulk_mail_attested
       AND %[1]s.archived_at IS NULL
       AND %[1]s.occurred_at > %[2]s.occurred_at -- occurred_at raw: a later mail in the thread, both instants
       AND %[3]s`, later, src, occurredday.BegunSQL(later, until))
}

// promiseRecipientAddressedSQL is whether the mail under row names the
// contact's address on its counterparty or a To/Cc row. A Bcc copy is not mail
// they saw, and a linked contact the headers do not name was not written to.
func promiseRecipientAddressedSQL(row, contact string) string {
	addresses := `SELECT lower(btrim(known.email)) FROM contact_email known
	     WHERE known.contact_id = ` + contact + ` AND known.archived_at IS NULL`
	return `((` + row + `.counterparty_email IN (` + addresses + `)
	    AND NOT EXISTS (SELECT 1 FROM activity_participant blind
	         WHERE blind.activity_id = ` + row + `.id AND blind.role = 'bcc'
	           AND lower(btrim(blind.address)) = ` + row + `.counterparty_email))
	  OR EXISTS (SELECT 1 FROM activity_participant told
	       WHERE told.activity_id = ` + row + `.id AND told.role IN ('to', 'cc')
	         AND lower(btrim(told.address)) IN (` + addresses + `)))`
}
