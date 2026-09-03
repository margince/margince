// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a list row says about the files that came with a message.
//
// The count was hardcoded to zero on every list projection, so the paperclip
// the canonical row draws could never appear — on a timeline, in Search or on
// the Worklist. These are the two claims that replaced it: a readable email
// reports what it carries, and a withheld one reports nothing, because knowing
// a contract arrived is knowing something about a message.

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedAttachment files one document against a message, the way capture leaves
// it. Written through the owner connection because the caller under test is a
// READER — a test that wrote its fixture as the reader would prove the reader
// can see its own writes and nothing about the projection.
func seedAttachment(t *testing.T, activity ids.UUID, filename string) {
	t.Helper()
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO attachment (entity_type, entity_id, filename, storage_key, source, captured_by)
		VALUES ('activity', $1, $2, $3, 'imap', 'connector:imap')`,
		activity, filename, "seed/"+filename+"/"+activity.String()); err != nil {
		t.Fatalf("seeding %s: %v", filename, err)
	}
}

func rowFor(page []crmcontracts.Activity, id ids.UUID) *crmcontracts.EmailSummary {
	for i := range page {
		if ids.UUID(page[i].Id) == id {
			return page[i].EmailSummary
		}
	}
	return nil
}

// A readable email reports what came with it, and a message with nothing
// attached reports zero rather than nothing at all.
func TestAListRowCarriesItsRealAttachmentCount(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	subject, body := "The signed contract", "Attached, as agreed."
	withFiles, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	bare := "Just a note"
	noFiles, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &bare, Body: &body, Direction: strPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	seedAttachment(t, ids.UUID(withFiles.Id), "contract.pdf")
	seedAttachment(t, ids.UUID(withFiles.Id), "annex.pdf")

	page, _, err := e.Activities.ListActivities(author, activities.ListActivitiesInput{
		EntityType: strPtr("person"), EntityID: &contact,
	})
	if err != nil {
		t.Fatalf("listing the timeline: %v", err)
	}

	carried := rowFor(page, ids.UUID(withFiles.Id))
	if carried == nil {
		t.Fatal("the message with files carried no email row at all")
	}
	if carried.AttachmentCount != 2 {
		t.Errorf("attachment_count = %d, want 2 — the paperclip on the row is drawn from this, "+
			"so a wrong number here is a badge that never appears", carried.AttachmentCount)
	}
	empty := rowFor(page, ids.UUID(noFiles.Id))
	if empty == nil {
		t.Fatal("the message without files carried no email row at all")
	}
	if empty.AttachmentCount != 0 {
		t.Errorf("a message with nothing attached reported %d files", empty.AttachmentCount)
	}
}

// A withheld row reports NO attachments, whatever came with the message.
//
// The file list is refused to a reader outside the audience, and the fact that
// files exist is the same fact in smaller print: a colleague who sees "2 files"
// beside a message they may not open has learned that a contract was exchanged.
func TestAWithheldRowReportsNoAttachments(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	subject, body := "Severance agreement", "The signed copy is attached."
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("outbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.UUID(logged.Id)
	seedAttachment(t, id, "severance.pdf")
	seedAttachment(t, id, "annex.pdf")

	// Admitted first, so the zero below is the audience's doing rather than a
	// count that never worked.
	open, _, err := e.Activities.ListActivities(colleague, activities.ListActivitiesInput{
		EntityType: strPtr("person"), EntityID: &contact,
	})
	if err != nil {
		t.Fatalf("colleague listing before limiting: %v", err)
	}
	if before := rowFor(open, id); before == nil || before.AttachmentCount != 2 {
		t.Fatalf("the colleague saw %v before limiting; the withheld case below would prove nothing",
			before)
	}

	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](id),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	after, _, err := e.Activities.ListActivities(colleague, activities.ListActivitiesInput{
		EntityType: strPtr("person"), EntityID: &contact,
	})
	if err != nil {
		t.Fatalf("colleague listing after limiting: %v", err)
	}
	held := rowFor(after, id)
	if held == nil {
		t.Fatal("the withheld row vanished; it stays discoverable with its content removed")
	}
	if held.DisplayStatus != crmcontracts.EmailAccessStatusWithheld {
		t.Fatalf("display_status = %q, want withheld", held.DisplayStatus)
	}
	if held.AttachmentCount != 0 {
		t.Errorf("a withheld row told the colleague that %d files came with a message they may "+
			"not read", held.AttachmentCount)
	}

	// The author is in the audience and still sees what they sent.
	mine, _, err := e.Activities.ListActivities(author, activities.ListActivitiesInput{
		EntityType: strPtr("person"), EntityID: &contact,
	})
	if err != nil {
		t.Fatalf("author listing: %v", err)
	}
	if own := rowFor(mine, id); own == nil || own.AttachmentCount != 2 {
		t.Errorf("the author lost the count on their own message: %v", own)
	}
}

// The single read agrees with the list. Two projections of one row that
// disagree about what came with a message is the drift this arc exists to
// remove — a reader opening a record from two directions must see one answer.
func TestTheSingleReadAgreesWithTheListAboutAttachments(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	subject, body := "The signed contract", "Attached, as agreed."
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.UUID(logged.Id)
	seedAttachment(t, id, "contract.pdf")

	one, err := e.Activities.GetActivity(author, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("single read: %v", err)
	}
	if one.EmailSummary == nil || one.EmailSummary.AttachmentCount != 1 {
		t.Errorf("the single read reported %v, want one file", one.EmailSummary)
	}
}
