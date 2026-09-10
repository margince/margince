// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// What the Art. 15 export says about a subject whose duplicate was merged into
// them.
//
// A merge retires one record and keeps the other. The consent module carries
// the retiring subject's STOPS onto the survivor deliberately, and just as
// deliberately leaves the originals where they are: the predecessor's row is
// evidence that THAT record's subject objected, and rewriting its subject would
// make the history say the objection was made about somebody else.
//
// So the data is split across two ids on purpose, and the export followed only
// one of them. subjectReach walks promoted_person_id — the LEAD twin of a
// promotion — and never merged_into_id, so a subject asking what is held about
// them was answered from the surviving row alone. Everything on the predecessor
// was omitted: the proof rows behind their consent, the bases their mail stood
// on, and the objection they made before the cleanup that merged them.
//
// Art. 15 owes what is HELD. The installation holds all of it.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedMergedPredecessor retires one person into another the way a merge does:
// the predecessor keeps its row and its data, and points at the survivor.
func seedMergedPredecessor(ctx context.Context, t *testing.T, e *sarIdentifierEnv, survivor ids.PersonID) ids.PersonID {
	t.Helper()
	var predecessor ids.UUID
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO person (full_name, source, captured_by, merged_into_id, archived_at)
		VALUES ('Merged Duplicate', 'test', 'human:x', $1, now())
		RETURNING id`, survivor).Scan(&predecessor); err != nil {
		t.Fatalf("seeding the merged-away person: %v", err)
	}
	return ids.From[ids.PersonKind](predecessor)
}

// TestTheExportFollowsAPredecessorIdentity is the whole slice: a subject whose
// duplicate was merged into them must be shown what is held under BOTH ids.
func TestTheExportFollowsAPredecessorIdentity(t *testing.T) {
	e := setupSARIdentifiers(t)
	predecessor := seedMergedPredecessor(e.ctx, t, e, e.person)

	// The objection the predecessor made. A6 carries a COPY onto the survivor
	// and keeps this one as evidence, so the export that reads only the
	// survivor shows the copy and never the act that produced it.
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO communication_suppression (person_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'marketing_objection', 'operator_ui', 'human:x', 'subject')`,
		predecessor); err != nil {
		t.Fatalf("seeding the predecessor's objection: %v", err)
	}
	// And the ground a message to them stood on, which stays where it was
	// written: nothing copies a basis forward.
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO communication_basis (person_id, kind, valid_from, captured_by)
		VALUES ($1, 'subject_initiated_correspondence', now() - interval '30 days', 'human:x')`,
		predecessor); err != nil {
		t.Fatalf("seeding the predecessor's basis: %v", err)
	}

	pkg, err := AssembleSAR(e.ctx, e.db, e.person)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	if len(pkg.CommunicationSuppression) == 0 {
		t.Error("the export carries no stop at all, though the record merged into this subject " +
			"holds an objection they made — a subject asking what is held is told the " +
			"installation holds nothing about the refusal it is still acting on")
	}
	if len(pkg.CommunicationBases) == 0 {
		t.Error("the export carries no communication basis, though the merged-away record holds " +
			"one — the subject is shown neither the mail nor the ground it stood on")
	}
}

// A LEAD PROMOTED INTO A RECORD THAT WAS LATER MERGED AWAY still belongs to the
// subject who survives.
//
// The lead's own proof rows stay keyed by lead_id — a promotion carries the
// consent forward and leaves the evidence where it was recorded — so the export
// reaches them through the lead twin. That twin points at the PREDECESSOR, so
// following only the survivor's promotions loses it.
func TestTheExportReachesALeadPromotedIntoAMergedAwayRecord(t *testing.T) {
	e := setupSARIdentifiers(t)
	predecessor := seedMergedPredecessor(e.ctx, t, e, e.person)

	var leadID ids.UUID
	if err := e.owner.QueryRow(e.ctx, `
		INSERT INTO lead (full_name, email, source, captured_by, promoted_person_id, promoted_at)
		VALUES ('Promoted Then Merged', 'twin@sar.test', 'test', 'human:x', $1, now())
		RETURNING id`, predecessor).Scan(&leadID); err != nil {
		t.Fatalf("seeding the lead twin: %v", err)
	}
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO communication_suppression (lead_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'marketing_objection', 'operator_ui', 'human:x', 'subject')`,
		leadID); err != nil {
		t.Fatalf("seeding the lead's objection: %v", err)
	}

	pkg, err := AssembleSAR(e.ctx, e.db, e.person)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	if len(pkg.CommunicationSuppression) == 0 {
		t.Error("the export carries no stop, though a lead promoted into a record later merged " +
			"into this subject holds an objection — two hops of ordinary cleanup and the " +
			"refusal disappears from what the subject is told we hold")
	}
}

// AND THE REACH DOES NOT WIDEN INTO A STRANGER. merged_into_id is followed in
// one direction only: records merged INTO this subject are theirs, and the
// record this subject was merged into belongs to somebody else.
func TestTheExportDoesNotReachTheRecordThisSubjectWasMergedInto(t *testing.T) {
	e := setupSARIdentifiers(t)

	var survivor ids.UUID
	if err := e.owner.QueryRow(e.ctx, `
		INSERT INTO person (full_name, source, captured_by)
		VALUES ('The Surviving Stranger', 'test', 'human:x') RETURNING id`).Scan(&survivor); err != nil {
		t.Fatalf("seeding the survivor: %v", err)
	}
	// This subject was merged INTO them, which is the opposite direction.
	if _, err := e.owner.Exec(e.ctx,
		`UPDATE person SET merged_into_id = $1 WHERE id = $2`, survivor, e.person); err != nil {
		t.Fatalf("retiring the subject: %v", err)
	}
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO communication_suppression (person_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'marketing_objection', 'operator_ui', 'human:x', 'subject')`,
		survivor); err != nil {
		t.Fatalf("seeding the stranger's objection: %v", err)
	}

	pkg, err := AssembleSAR(e.ctx, e.db, e.person)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	if len(pkg.CommunicationSuppression) != 0 {
		t.Errorf("the export carries %d stop(s) belonging to the record this subject was merged "+
			"INTO — that is somebody else's data in this subject's package",
			len(pkg.CommunicationSuppression))
	}
}
