// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What capture knows about an address whose contact a confidentiality hold
// kept to its owner, read by the release that may end the hold.
//
// The release asks about every live message with the address, not only the
// messages a ledger row names. A ledger row is written once per address, so a
// confidential thread that arrives later has none.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// releasableKinds are the sender verdicts whose record is an ordinary
// business contact. vendor_pitch is one: a pitch the workspace answered makes
// the same contact a `contact` answer does.
var releasableKinds = []string{KindContact, KindVendorPitch}

// ReleasableAddressSQL is the test that the address in column may stop
// holding its contact. It has at least one ledger row, and every row is a
// `real` answer of a releasable kind. No live message with it is held
// (CounterpartyHeldOn). Any other status or kind, unknown included, keeps it.
func ReleasableAddressSQL(column string) string {
	return fmt.Sprintf(`(EXISTS (SELECT 1 FROM capture_pending_counterparty rw WHERE rw.email = %[1]s)
		AND NOT EXISTS (
		    SELECT 1 FROM capture_pending_counterparty rw
		     WHERE rw.email = %[1]s
		       AND NOT (rw.status = '%[2]s' AND coalesce(rw.kind, '') IN ('%[3]s')))
		AND NOT EXISTS (
		    SELECT 1 FROM activity ra
		     WHERE ra.counterparty_email = %[1]s AND ra.archived_at IS NULL
		       AND %[4]s))`,
		column, PendingStatusReal, strings.Join(releasableKinds, "', '"), CounterpartyHeldOn("ra"))
}

// WithheldAddressSQL is the test that a `contact` answer for the address in
// column was withheld from the workspace (MarkWithheldFromWorkspaceTx). It is
// the evidence that the hold is a confidentiality hold capture placed.
func WithheldAddressSQL(column string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM capture_pending_counterparty ww
		WHERE ww.email = %s AND ww.withheld_from_workspace AND ww.status = '%s')`, column, PendingStatusReal)
}

// WithheldUnansweredOutboundTx reports whether a withheld answer for one of
// the addresses was mail the owner sent, to an address that has not answered.
// Such a contact stays the owner's for that reason once its hold ends.
func WithheldUnansweredOutboundTx(ctx context.Context, tx pgx.Tx, emails []string) (bool, error) {
	var unanswered bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM capture_pending_counterparty w
		    JOIN activity a ON a.id = w.activity_id AND a.restricted_at IS NULL
		   WHERE w.email = ANY($2) AND w.withheld_from_workspace AND w.status = $1
		     AND a.direction = 'outbound' AND a.counterparty_outbound_attested
		     AND NOT `+strings.ReplaceAll(addressAnswered(), "$1", "w.email")+`)`,
		PendingStatusReal, foldAll(emails)).Scan(&unanswered)
	if err != nil {
		return false, fmt.Errorf("capture: reading whether a withheld address answered: %w", err)
	}
	return unanswered, nil
}

// WithheldSendersOnThreadTx lists up to limit addresses whose contact was
// withheld because of a message on this thread. A thread that stops holding
// may end their holds.
func WithheldSendersOnThreadTx(ctx context.Context, tx pgx.Tx, threadKey string, limit int) ([]string, error) {
	if threadKey == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.email
		  FROM activity a
		  JOIN capture_pending_counterparty p ON p.activity_id = a.id
		 WHERE a.thread_key = $1 AND a.restricted_at IS NULL AND p.withheld_from_workspace
		 ORDER BY p.email
		 LIMIT $2`, threadKey, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: listing the senders a thread withheld: %w", err)
	}
	emails, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("capture: listing the senders a thread withheld: %w", err)
	}
	return emails, nil
}

// Hold locks.
//
// A release reads every message, thread verdict and ledger row of an address,
// then publishes. A writer that creates or lifts a hold in between would make
// it publish on evidence that is no longer true. So the release and those
// writers share locks. A writer that inserts a held message takes its
// address's lock (lockAddressHoldTx). A writer that opens or answers a thread
// question takes the thread's lock (LockThreadHoldTx). The release takes every
// address lock, then every thread lock, each in sorted order, then share-locks
// the existing rows. Updates of those rows by any writer wait on the row lock.
const (
	addressHoldLock = "capture_address_hold"
	threadHoldLock  = "capture_thread_hold"
)

func lockAddressHoldTx(ctx context.Context, tx pgx.Tx, email string) error {
	if folded := correspondence.Fold(email); folded != "" {
		return storekit.LockWriteIdentity(ctx, tx, addressHoldLock, folded)
	}
	return nil
}

// lockActivityInsertTx takes the locks a captured message's insert serializes
// on.
//
// Attested outbound is the only write that turns "does this workspace
// correspond with them" from no to yes. A verdict acts on that answer in its
// own transaction, so the two serialize on the correspondence lock. Inbound
// mail cannot change that answer, so only the attested row takes it.
//
// Any message may be born held and keep a contact held, so every insert takes
// its address's hold lock, which a release waits on (LockHoldEvidenceTx).
func lockActivityInsertTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) error {
	if rec.Counterparty.SentByOwner() {
		if err := storekit.LockWriteIdentity(ctx, tx, correspondence.LockEntity,
			mailidentity.CorrespondenceLockKey(rec.Counterparty.Email)); err != nil {
			return err
		}
	}
	return lockAddressHoldTx(ctx, tx, rec.Counterparty.Email)
}

// LockThreadHoldTx takes the hold lock of one thread.
func LockThreadHoldTx(ctx context.Context, tx pgx.Tx, threadKey string) error {
	if threadKey == "" {
		return nil
	}
	return storekit.LockWriteIdentity(ctx, tx, threadHoldLock, threadKey)
}

// LockHoldEvidenceTx locks what a release reads about the addresses, in the
// order the writers above agree on.
func LockHoldEvidenceTx(ctx context.Context, tx pgx.Tx, emails []string) error {
	folded := foldAll(emails)
	for _, email := range folded {
		if err := lockAddressHoldTx(ctx, tx, email); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT thread_key FROM activity
		 WHERE counterparty_email = ANY($1) AND thread_key IS NOT NULL AND thread_key <> ''
		 ORDER BY thread_key`, folded)
	if err != nil {
		return fmt.Errorf("capture: reading the threads of a held contact: %w", err)
	}
	threads, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("capture: reading the threads of a held contact: %w", err)
	}
	for _, thread := range threads {
		if err := LockThreadHoldTx(ctx, tx, thread); err != nil {
			return err
		}
	}
	for _, lock := range []struct {
		stmt string
		keys []string
	}{
		{`SELECT 1 FROM capture_thread_verdict WHERE thread_key = ANY($1) ORDER BY id FOR SHARE`, threads},
		{`SELECT 1 FROM activity WHERE counterparty_email = ANY($1) ORDER BY id FOR SHARE`, folded},
		{`SELECT 1 FROM capture_pending_counterparty WHERE email = ANY($1) ORDER BY id FOR SHARE`, folded},
	} {
		if _, err := tx.Exec(ctx, lock.stmt, lock.keys); err != nil {
			return fmt.Errorf("capture: locking a held contact's evidence: %w", err)
		}
	}
	return nil
}

// foldAll folds, drops empty and sorts addresses, so every locker takes them
// in one order.
func foldAll(emails []string) []string {
	out := make([]string, 0, len(emails))
	for _, email := range emails {
		if f := correspondence.Fold(email); f != "" {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
