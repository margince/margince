// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// A typed link says how two records relate. Anonymizing a contact leaves the
// row standing, so both ways a contact is anonymized have to remove its links.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type linkedSubjects struct {
	erased, retained, unrelated ids.UUID
	deal                        ids.UUID
}

// seedLinkedSubjects links two contacts to one deal, one each way. It also
// links the deal to a company, a link that has to survive.
func seedLinkedSubjects(ctx context.Context, t *testing.T, tx pgx.Tx) linkedSubjects {
	t.Helper()
	s := linkedSubjects{erased: ids.NewV7(), retained: ids.NewV7(), unrelated: ids.NewV7(), deal: ids.NewV7()}
	pipeline, stage := ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO pipeline (id, name) VALUES ($1, 'Erase links')`, pipeline)
	mustExec(ctx, t, tx, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	mustExec(ctx, t, tx, `INSERT INTO deal (id, name, status, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Linked deal', 'open', $2, $3, 'seed', 'system')`, s.deal, pipeline, stage)
	for _, c := range []ids.UUID{s.erased, s.retained} {
		mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, 'Linked Subject', 'manual', 'test')`, c)
	}
	mustExec(ctx, t, tx, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Acme', 'seed', 'system')`, s.unrelated)
	link := `INSERT INTO record_link (from_type, from_id, to_type, to_id, link_type, captured_by)
		VALUES ($1, $2, $3, $4, 'related_to', 'test')`
	mustExec(ctx, t, tx, link, "contact", s.erased, "deal", s.deal)
	mustExec(ctx, t, tx, link, "deal", s.deal, "contact", s.erased)
	mustExec(ctx, t, tx, link, "contact", s.retained, "deal", s.deal)
	mustExec(ctx, t, tx, link, "deal", s.deal, "company", s.unrelated)
	return s
}

func linksOf(ctx context.Context, t *testing.T, tx pgx.Tx, contact ids.UUID) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM record_link
		 WHERE (from_type = 'contact' AND from_id = $1) OR (to_type = 'contact' AND to_id = $1)`, contact).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestErasingAContactRemovesItsLinksAndOnlyThose(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	s := seedLinkedSubjects(ctx, t, tx)
	if err := purgeContactDerivedRows(ctx, tx, ids.From[ids.ContactKind](s.erased), nil); err != nil {
		t.Fatalf("purging the subject's derived rows: %v", err)
	}
	if n := linksOf(ctx, t, tx, s.erased); n != 0 {
		t.Errorf("%d links still name the erased contact", n)
	}
	if n := linksOf(ctx, t, tx, s.retained); n != 1 {
		t.Errorf("another contact holds %d links, want its one", n)
	}
	var rest int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM record_link WHERE to_type = 'company'`).Scan(&rest); err != nil || rest != 1 {
		t.Errorf("the deal-to-company link count = %d, %v; want it untouched", rest, err)
	}
}

func TestAnonymizingAContactByRetentionRemovesItsLinks(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	s := seedLinkedSubjects(ctx, t, tx)
	if err := purgeAnonymizedContactJudgments(ctx, tx, s.erased, nil); err != nil {
		t.Fatalf("purging the anonymized contact's judgments: %v", err)
	}
	if n := linksOf(ctx, t, tx, s.erased); n != 0 {
		t.Errorf("%d links still name the anonymized contact", n)
	}
	if n := linksOf(ctx, t, tx, s.retained); n != 1 {
		t.Errorf("another contact holds %d links, want its one", n)
	}
}
