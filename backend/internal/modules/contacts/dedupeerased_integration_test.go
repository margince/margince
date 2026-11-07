// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestArchivedSubjectsCannotFileOrReopenDuplicatePairs(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "refile"}[existing], func(t *testing.T) {
			e := setupDedupe(t)
			ctx := asArchiver(e)
			left := e.plainContact(ctx, t, "Erased Person", "erased@dedupe.test")
			right := e.plainContact(ctx, t, "Other Person", "other@dedupe.test")
			var pair ids.UUID
			if existing {
				pair = e.fileOldRulePair(ctx, t, entityContact, left, right, 0.9)
			}
			if _, err := e.store.ArchiveContact(ctx, ids.From[ids.ContactKind](left), nil); err != nil {
				t.Fatal(err)
			}
			err := e.store.tx(ctx, func(tx pgx.Tx) error {
				if existing {
					if _, err := tx.Exec(ctx, `UPDATE dedupe_candidate SET evidence='[]', archived_at=now() WHERE id=$1`, pair); err != nil {
						return err
					}
				}
				filed, err := recordDedupeCandidate(ctx, tx, entityContact, left, right, 0.9,
					[]map[string]any{{evidenceFieldKey: fieldFullName, evidenceLeftKey: "Erased Person"}}, "manual", "human:"+e.rep.String())
				if err != nil {
					return err
				}
				if filed {
					t.Error("archived subject filed a candidate")
				}
				var count int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM dedupe_candidate WHERE left_contact_id=ANY($1) AND right_contact_id=ANY($1) AND (archived_at IS NULL OR evidence<>'[]'::jsonb)`, []ids.UUID{left, right}).Scan(&count); err != nil {
					return err
				}
				if count != 0 {
					t.Errorf("found %d pending or personal snapshots", count)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUndoCannotReopenAPairWithAnArchivedSubject(t *testing.T) {
	e := setupDedupe(t)
	ctx := asArchiver(e)
	left := e.plainContact(ctx, t, "Left Person", "left@undo-erased.test")
	right := e.plainContact(ctx, t, "Right Person", "right@undo-erased.test")
	pair := e.fileOldRulePair(ctx, t, entityContact, left, right, 0.9)
	if _, err := e.store.DisposeDedupeCandidate(ctx, pair, "not_a_duplicate", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ArchiveContact(ctx, ids.From[ids.ContactKind](left), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UndoDedupeDisposition(ctx, pair); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("undo = %v, want not found", err)
	}
	row, err := e.store.GetDedupeCandidate(ctx, pair)
	if err != nil {
		t.Fatal(err)
	}
	if row.Disposition != "not_a_duplicate" {
		t.Fatalf("disposition = %s", row.Disposition)
	}
}

func TestFilingHoldsBothSubjectsAgainstConcurrentErasure(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.plainContact(ctx, t, "Left Person", "left@lock-dedupe.test")
	right := e.plainContact(ctx, t, "Right Person", "right@lock-dedupe.test")
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		filed, err := recordDedupeCandidate(ctx, tx, entityContact, left, right, 0.9,
			[]map[string]any{{evidenceFieldKey: fieldFullName}}, "manual", "human:"+e.rep.String())
		if err != nil {
			return err
		}
		if !filed {
			t.Error("live pair was not filed")
		}
		for _, subject := range []ids.UUID{left, right} {
			err := e.store.tx(ctx, func(other pgx.Tx) error {
				var held ids.UUID
				return other.QueryRow(ctx, `SELECT id FROM contact WHERE id=$1 FOR NO KEY UPDATE NOWAIT`, subject).Scan(&held)
			})
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.Code != "55P03" {
				t.Errorf("subject %s lock=%v, want lock unavailable", subject, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
