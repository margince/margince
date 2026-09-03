// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// When a `no_record` hold stops being true, and when it never does.
//
// The reason reads like "this row has no link", and that is not what it
// records. The capture ladder writes it only on a TERMINAL no-record outcome:
// a suppressed newsletter, a role mailbox, a sender a prior verdict settled, a
// thread judged the mailbox owner's private life. Each of those is a decision
// about the SENDER, and a link can arrive on such a row long afterwards — a
// project attribution, a hand relink, a cohort promotion. Lifting on the mere
// presence of a link would republish a mailbox owner's private correspondence
// to the whole workspace.
//
// A MEETING reached the same ladder for no such reason. Attendance is a list,
// so the calendar mapper leaves the counterparty unset, the gate concludes
// "captured, named nobody", and the limiter holds a record whose only defect is
// that nothing had filed it yet.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fileUnder plants an activity_link the way a repair pass does, so a test can
// ask what the recompute makes of a row that has since been filed.
func fileUnder(t *testing.T, e *integration.SearchEnv, activity ids.UUID, person ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2) ON CONFLICT DO NOTHING`, activity, person)
		return err
	}); err != nil {
		t.Fatalf("filing the activity under a person: %v", err)
	}
}

// A suppressed sender's message is held because of WHO SENT IT. Filing it under
// a record later says nothing about that, and must not open it.
func TestASuppressedSendersHoldSurvivesBeingFiled(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	sync(t, email("dse@eu.docusign.net", "DocuSign EU", captureOwner, "held-then-filed@docusign.net", ""))
	activityID := oneActivityID(t, e)
	if _, reason := audienceOf(t, e, activityID); reason != activities.ReasonNoRecord {
		t.Fatalf("the ladder did not hold the link-less message: reason %q", reason)
	}

	// Somebody files it — a project attribution, a hand relink, a cohort pass.
	filedUnder := e.SeedID(t, `INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Someone', 'manual', 'human:x')`)
	fileUnder(t, e, activityID, filedUnder)

	recompute(t, e, activityID)
	got, reason := audienceOf(t, e, activityID)
	if got != "participants" || reason != activities.ReasonNoRecord {
		t.Fatalf("filing a suppressed sender's mail opened it: audience %q reason %q — "+
			"the hold is about who sent it, and a link says nothing about that", got, reason)
	}
}

// The reason the two holds are separate words rather than one word read two
// ways: a MEETING can carry a judged hold too.
//
// The sink admits a record by its counterparty SHAPE, never by its kind
// (capture.Sink.Upsert), so a meeting-shaped record arriving with a mail
// counterparty reaches the same ladder mail does — the private-thread branch
// and the suppression registry included. Such a meeting is held for a real
// reason about a real person, and telling it apart from a structurally
// counterparty-less one by KIND would open exactly that message the moment
// anything filed it.
//
// This drives the recompute against a row stamped the way that ladder stamps
// one, which is the state the two paths differ on.
func TestAJudgedHoldSurvivesBeingFiledWhateverTheKind(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	sync(t, email("dse@eu.docusign.net", "DocuSign EU", captureOwner, "judged-meeting@docusign.net", ""))
	activityID := oneActivityID(t, e)

	// The same row, wearing the kind a calendar connector writes. The hold on
	// it was placed by a judgement about its sender, and that is what the
	// reason records.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET kind = 'meeting' WHERE id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("restating the activity as a meeting: %v", err)
	}
	filedUnder := e.SeedID(t, `INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Someone', 'manual', 'human:x')`)
	fileUnder(t, e, activityID, filedUnder)

	recompute(t, e, activityID)
	got, reason := audienceOf(t, e, activityID)
	if got != "participants" || reason != activities.ReasonNoRecord {
		t.Fatalf("a judged hold on a meeting-kind row opened when it was filed: audience %q reason %q — "+
			"the kind says nothing about whether somebody was judged", got, reason)
	}
}
