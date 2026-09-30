// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Which cited records still stand, asked of a real database.
//
// Stored advice outlives the record it was written from: a scan runs, the rep
// archives the email nine minutes later, and the card is still there quoting
// its subject and offering a "Create draft" the composer refuses. Retraction
// is what removes it, and retraction is only as good as the question it asks.
//
// The question is EXISTENCE, under the reader's discover scope — never "did
// the email-summary reader return a row". That reader is narrower twice over:
// it admits only `kind = 'email'`, and it gates on content because it prints a
// subject and a body preview. Reading "gone" out of its silence would retract
// advice about a live MEETING, and advice about a live email whose body this
// reader may not open. Both cases are here, because both are valid advice that
// a narrower probe would silently delete.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// logged is one activity of the given kind, linked to a contact, as the real
// writer writes it.
func logged(
	author context.Context, t *testing.T, e *Env, contact ids.UUID, kind, subject string,
) ids.UUID {
	t.Helper()
	body := "What did we agree?"
	row, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: kind, Subject: &subject, Body: &body, Direction: StrPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging a %s: %v", kind, err)
	}
	return ids.UUID(row.Id)
}

func standing(reader context.Context, t *testing.T, e *Env, cited ...ids.UUID) map[ids.UUID]bool {
	t.Helper()
	var out map[ids.UUID]bool
	err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		var readErr error
		out, readErr = activities.StandingActivities(reader, tx, cited)
		return readErr
	})
	if err != nil {
		t.Fatalf("asking which citations stand: %v", err)
	}
	return out
}

// An archived record stops standing, and its live neighbours do not. This is
// the defect: the advice quoting the archived one must go, and the rest stays.
func TestAnArchivedRecordStopsStandingAndItsNeighboursDoNot(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Frédéric Buyer", &e.Rep1)

	toArchive := logged(author, t, e, contact, "email", "Which locale wins?")
	survivor := logged(author, t, e, contact, "email", "Rollout dates")

	before := standing(author, t, e, toArchive, survivor)
	if !before[toArchive] || !before[survivor] {
		t.Fatalf("both mails should stand before the archive: %v", before)
	}

	if _, err := e.Activities.ArchiveActivity(author,
		ids.From[ids.ActivityKind](toArchive), nil); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	after := standing(author, t, e, toArchive, survivor)
	if after[toArchive] {
		t.Error("an archived mail still stands, so the advice quoting it survives the archive")
	}
	if !after[survivor] {
		t.Error("a live mail stopped standing, so valid advice about it would be retracted")
	}
}

// A live MEETING stands. A scan's findings cite emails, messages, calls and
// meetings alike, and the email-summary reader returns none of the last three.
// Keying retraction on that reader would delete every finding about a call or
// a meeting the moment it was replayed.
func TestALiveMeetingStandsAlthoughItCarriesNoEmailSummary(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Frédéric Buyer", &e.Rep1)

	meeting := logged(author, t, e, contact, "meeting", "Quarterly review")
	call := logged(author, t, e, contact, "call", "Pricing question")

	if got := standing(author, t, e, meeting, call); !got[meeting] || !got[call] {
		t.Errorf("a live meeting and call did not stand: %v — advice about them would be retracted", got)
	}

	// The narrower reader really does refuse them, so the case above is the
	// one that would have broken rather than a hypothetical.
	summaries, err := e.Activities.EmailSummariesByID(author, []ids.UUID{meeting, call})
	if err != nil {
		t.Fatalf("reading summaries: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("the email-summary reader answered for a meeting or call: %v — this test no longer proves the gap", summaries)
	}
}

// A mail whose AUDIENCE excludes a colleague still STANDS for them. The
// content reader withholds its subject and body, which is correct and is a
// different question: advice that a thread has gone unanswered is written from
// the row, not from what it said, and the colleague is entitled to it.
func TestAMailOutsideAColleaguesAudienceStillStandsForThem(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Frédéric Buyer", &e.Rep1)

	limited := logged(author, t, e, contact, "email", "Severance terms")
	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](limited),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	summaries, err := e.Activities.EmailSummariesByID(colleague, []ids.UUID{limited})
	if err != nil {
		t.Fatalf("reading summaries: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("the colleague got the limited mail's content; this test no longer proves the gap")
	}

	if got := standing(colleague, t, e, limited); !got[limited] {
		t.Errorf("a live mail stopped standing for a colleague who may not read its body: %v", got)
	}
}
