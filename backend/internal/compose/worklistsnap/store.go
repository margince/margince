// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package worklistsnap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Row is one identity in a frozen walk: which lane raised it, and which record.
//
// The pair, because that is what names a row on this queue. The lanes mint ids
// independently — a task and a waiting message can carry the same underlying
// record's id — and a batch row carries a synthetic key its own lane mints. An
// id alone would resume into a row the reader was never looking at.
type Row struct {
	Source string `json:"source"`
	RowID  string `json:"row_id"`
}

// Buckets is the frozen partition, carried so the headline does not climb as
// work arrives behind a reader who is still paging.
//
// Stored as the four counts rather than as the rows they came from: the walk
// needs to state what it STARTED with, and recomputing that from the surviving
// rows would give the falling number the response reports separately.
type Buckets struct {
	Urgent   int `json:"urgent"`
	DueToday int `json:"due_today"`
	Planned  int `json:"planned"`
	Review   int `json:"review"`
}

// Snapshot is one walk, as it was minted.
type Snapshot struct {
	ID        ids.UUID
	AsOf      time.Time
	ExpiresAt time.Time
	Buckets   Buckets
	Rows      []Row
}

// Life is how long a walk stays resumable.
//
// A walk is one sitting. Past this the token is refused and the client starts a
// fresh snapshot, which is a better answer than resuming somebody into a day
// that ended hours ago — the rows would still be re-read live, but the ORDER
// would be yesterday's judgement about what mattered.
const Life = 4 * time.Hour

// KeptPerReader bounds what one reader's snapshots cost.
//
// Exported so the integration test asserts the sweep against the STORE's own
// ceiling rather than a number repeated in the test, which would keep passing
// after somebody changed one of the two.
//
// A snapshot outlives the page that minted it and nothing but expiry removes
// one, so a rep who refreshes twenty times in a morning would otherwise leave
// twenty rows behind. Three is past what a person can be walking at once: two
// tabs and the one they forgot.
const KeptPerReader = 3

// maxRows bounds one walk's stored identity list.
//
// The lanes bound themselves well below this, so it is a ceiling on cost rather
// than a product rule — what stops one pathological day writing a document
// instead of a list. A day past it is stored to the bound and walks that far.
const maxRows = 1000

// Service writes and reads the frozen walks.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// New binds the store to its pool and its clock.
//
// The clock is injected for the reason every clock in this tree is: expiry is
// the whole behaviour here, and a test that had to wait four hours to prove it
// would prove it once and then be deleted.
func New(pool *pgxpool.Pool, now func() time.Time) *Service {
	return &Service{pool: pool, now: now}
}

// Freeze records the walk a first page just assembled.
//
// It returns the snapshot's id, which the cursor carries. A failure here is
// returned rather than swallowed: a page that silently failed to freeze would
// hand back a cursor naming a snapshot that does not exist, and the reader's
// next page would be refused with nothing on the first one to explain it.
func (s *Service) Freeze(
	ctx context.Context, fingerprint string, asOf time.Time, buckets Buckets, rows []Row,
) (ids.UUID, error) {
	reader, err := readerOf(ctx)
	if err != nil {
		return ids.UUID{}, err
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	encodedRows, err := json.Marshal(rows)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("worklistsnap: recording a walk's rows: %w", err)
	}
	encodedBuckets, err := json.Marshal(buckets)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("worklistsnap: recording a walk's figures: %w", err)
	}
	now := s.now()
	var id ids.UUID
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		// The reader's old walks go first, in the same transaction: a sweep that
		// ran separately could leave a reader at their bound if it failed, and
		// nothing else deletes these rows.
		if err := sweep(ctx, tx, reader, now); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO worklist_snapshot
			       (reader_id, params_fingerprint, as_of, created_at, expires_at, buckets, rows)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			reader, fingerprint, asOf, now, now.Add(Life), encodedBuckets, encodedRows,
		).Scan(&id)
	})
	if err != nil {
		return ids.UUID{}, fmt.Errorf("worklistsnap: freezing a walk: %w", err)
	}
	return id, nil
}

// Resume reads back a walk this reader started.
//
// It refuses rather than answering empty in every case where the walk cannot be
// continued honestly: a snapshot belonging to somebody else, one that has
// expired, one minted under a different question. Each is ErrNotFound, so a
// stolen token cannot be told from an expired one — an attacker learns nothing
// about whether a walk exists, and the client's remedy is the same either way.
func (s *Service) Resume(ctx context.Context, id ids.UUID, fingerprint string) (Snapshot, error) {
	reader, err := readerOf(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	var (
		out            Snapshot
		encodedRows    []byte
		encodedBuckets []byte
		storedPrint    string
	)
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, as_of, expires_at, params_fingerprint, buckets, rows
			  FROM worklist_snapshot
			 WHERE id = $1 AND reader_id = $2 AND expires_at > $3`,
			id, reader, s.now(),
		).Scan(&out.ID, &out.AsOf, &out.ExpiresAt, &storedPrint, &encodedBuckets, &encodedRows)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, apperrors.ErrNotFound
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("worklistsnap: resuming a walk: %w", err)
	}
	// The question, checked here as well as at the token. The cursor carries its
	// own fingerprint and the caller compares it, but a snapshot id could be
	// lifted onto a token minted for a different scope — so the walk itself says
	// which question it answers, and refuses to be resumed into another.
	if storedPrint != fingerprint {
		return Snapshot{}, apperrors.ErrNotFound
	}
	if err := json.Unmarshal(encodedRows, &out.Rows); err != nil {
		return Snapshot{}, fmt.Errorf("worklistsnap: reading a walk's rows: %w", err)
	}
	if err := json.Unmarshal(encodedBuckets, &out.Buckets); err != nil {
		return Snapshot{}, fmt.Errorf("worklistsnap: reading a walk's figures: %w", err)
	}
	return out, nil
}

// sweep removes this reader's expired walks and keeps the newest few.
//
// Both bounds in one statement because they answer one question — what this
// reader's snapshots are allowed to cost — and two statements would let a
// failure between them leave the ledger half-tidied.
func sweep(ctx context.Context, tx pgx.Tx, reader ids.UUID, now time.Time) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM worklist_snapshot
		 WHERE reader_id = $1
		   AND (expires_at <= $2
		     OR id NOT IN (SELECT id FROM worklist_snapshot
		                    WHERE reader_id = $1
		                    ORDER BY created_at DESC
		                    LIMIT $3))`,
		reader, now, KeptPerReader-1,
	); err != nil {
		return fmt.Errorf("worklistsnap: sweeping a reader's old walks: %w", err)
	}
	return nil
}

// readerOf is whose walk this is.
//
// A snapshot binds ONE reader, so a principal with no human behind it has no
// walk to hold: refused rather than written against a zero id, which would be a
// row every agent shared and a walk any of them could resume into.
func readerOf(ctx context.Context) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}
