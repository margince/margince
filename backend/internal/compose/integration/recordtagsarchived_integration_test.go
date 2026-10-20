// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Archiving a record drops its tag assignments, so an empty tag list on an
// archived record would say "this record has no tags" when the truth is "its
// tags were taken off with it". The siblings (the record's lists, applying a
// tag) answer not-found for the same state; the tag read answers it the same
// way rather than reporting a fact nobody established.
func TestRecordTagsOfAnArchivedRecordAnswerNotFoundLikeItsSiblings(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	ctx := e.Admin()

	cases := []struct {
		entity string
		seed   func() ids.UUID
	}{
		{"contact", func() ids.UUID { return e.SeedContact(t, "Archived Contact", &e.Rep1) }},
		{"company", func() ids.UUID { return e.SeedCompany(t, "Archived Company", &e.Rep1) }},
		{"lead", func() ids.UUID {
			return seedLead(t, e, "Archived Lead", "archived.lead@prospect.test", &e.Rep1).UUID
		}},
	}
	for _, tc := range cases {
		t.Run(tc.entity, func(t *testing.T) {
			id := tc.seed()
			if _, err := store.RecordTagsFor(ctx, tc.entity, id); err != nil {
				t.Fatalf("a live %s must still read its tags: %v", tc.entity, err)
			}
			archiveRow(t, e, tc.entity, id)
			_, err := store.RecordTagsFor(ctx, tc.entity, id)
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("tags of an archived %s answered %v, want not-found", tc.entity, err)
			}
		})
	}
}

func archiveRow(t *testing.T, e *Env, table string, id ids.UUID) {
	t.Helper()
	err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, execErr := tx.Exec(e.Admin(),
			`UPDATE `+pgx.Identifier{table}.Sanitize()+` SET archived_at = now() WHERE id = $1`, id)
		return execErr
	})
	if err != nil {
		t.Fatalf("archiving the %s: %v", table, err)
	}
}
