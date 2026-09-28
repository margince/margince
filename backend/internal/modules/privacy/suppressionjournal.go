// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The suppression list, kept somewhere a restore cannot roll back.
//
// ReapplySuppressions re-erases whatever a restore brought back, and it reads
// the list to know what to look for. But the list lives in the SAME database:
// restore to a point before an erasure and the subject's rows come back AND
// the record that they were erased goes away with them. The pass then has
// nothing to reapply, and the one control standing between a restore and a
// resurrected subject quietly has no input.
//
// So the list is exported to the object store, which the restore procedure
// does not roll back. One object per suppressed identifier, keyed by the hash
// itself — the key IS the question a reader asks, so replay needs no listing
// and the store needs no new verb.
//
// An EXPORT rather than a write beside each erasure, and that is what makes it
// safe: a hook firing after the erasure's transaction commits loses its entry
// to a crash in between, and one firing before writes entries for erasures
// that then failed. A sync of the whole list is idempotent, repairs anything a
// previous run missed, and can be re-run before a backup without thinking
// about what changed since.

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// suppressionJournalPrefix is where the exported list lives.
//
// Deliberately OUTSIDE the <workspace>/<kind>/<id> namespace every other
// object uses. The list has no workspace and nothing for one to key on
// (storekit/suppression.go, ADR-0091 and ADR-0061), so filing it under a
// tenant would be claiming a scope it does not have — and worse, would put it
// inside the prefix a tenant-wide reset sweeps, which is the one operation
// that must never take it.
const suppressionJournalPrefix = "erasure-suppression/"

// suppressionJournalKey addresses one suppressed identifier. The hash is the
// key, so asking "is this address suppressed" is a Get rather than a scan,
// and the journal needs no enumeration to be useful.
func suppressionJournalKey(kind, valueHash string) string {
	return suppressionJournalPrefix + kind + "/" + valueHash
}

// ExportSuppressions copies the suppression list to the object store, and
// reports how many entries it wrote.
//
// Idempotent by construction: Put overwrites, so a re-run costs writes and
// changes nothing. Run it on a schedule and before a backup — the entries it
// adds are the ones a restore would otherwise lose.
//
// The object's body is the identifier's KIND and the date it was suppressed,
// and never the identifier: the whole point of the list is that the value is
// gone and only its fingerprint remains, so an export that carried the address
// would reconstitute what the erasure destroyed.
func (e *Eraser) ExportSuppressions(ctx context.Context) (int, error) {
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return 0, err
	}
	if e.blob == nil {
		// A deployment with no object store has nowhere to put the list, and
		// saying so is better than reporting an export that did not happen:
		// the whole value of this is that somebody can rely on it having run.
		return 0, fmt.Errorf("privacy: exporting the suppression list needs an object store")
	}
	type entry struct{ kind, hash, when string }
	var entries []entry
	if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT kind, value_hash, created_at::date::text FROM erasure_suppression`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row entry
			if err := rows.Scan(&row.kind, &row.hash, &row.when); err != nil {
				return err
			}
			entries = append(entries, row)
		}
		return rows.Err()
	}); err != nil {
		return 0, fmt.Errorf("privacy: reading the suppression list to export it: %w", err)
	}
	for _, row := range entries {
		body := []byte(row.kind + " " + row.when + "\n")
		if err := e.blob.Put(ctx, suppressionJournalKey(row.kind, row.hash),
			bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
			return 0, fmt.Errorf("privacy: exporting suppression %s: %w", row.hash, err)
		}
	}
	return len(entries), nil
}

// journalSuppressed reports whether the exported list names this hash.
//
// Consulted only for a candidate the database's own list did not already
// name, which is the case a restore creates: the rows came back and the list
// that would have caught them went with the rollback.
//
// A store that cannot answer is not a "no". Reporting one would let an outage
// read as "this subject was never erased", which is the exact failure this
// whole path exists to prevent, so the error travels.
func (e *Eraser) journalSuppressed(ctx context.Context, kind, valueHash string) (bool, error) {
	if e.blob == nil {
		return false, nil
	}
	reader, _, err := e.blob.Get(ctx, suppressionJournalKey(kind, valueHash))
	if err != nil {
		if errors.Is(err, blobstore.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("privacy: asking the exported suppression list about %s: %w", valueHash, err)
	}
	//craft:ignore swallowed-errors the body is not read; the object's existence is the whole answer
	_ = reader.Close()
	return true, nil
}
