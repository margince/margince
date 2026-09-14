// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A cited message opens for the reader it belongs to, and for nobody else.
//
// The enrichment is what makes a citation pressable, so its gate is the one
// that decides whether pressing a chip on a brief can show a reader a
// conversation they were never in. The unit tests hold the batching and the
// collectors against a fake reader; only a real database holds the reader's own
// content clause and audience arm, which is where that decision actually lives.

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/briefevidence"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// citing is one brief sentence resting on one activity — the shape every
// producer emits, built here rather than run through a model lane so the test
// is about the enrichment and not about what a writer chose to cite.
func citing(id ids.UUID) []crmcontracts.CompanyBriefSentence {
	return []crmcontracts.CompanyBriefSentence{{
		Text: "They asked which locale wins and nobody has answered.",
		Evidence: []crmcontracts.CompanyBriefEvidence{{
			EntityType: crmcontracts.CompanyBriefEvidenceEntityTypeActivity,
			EntityId:   openapi_types.UUID(id),
		}},
	}}
}

// The reader who may read the message gets the row that opens it: the same
// subject, the same access word, naming the same activity.
func TestACitedMessageCarriesItsRowForAReaderWhoMayReadIt(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Frédéric Buyer", &e.Rep1)

	subject, body := "Translation fallback decision", "Which locale wins when the string is missing?"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	sentences := citing(ids.UUID(logged.Id))

	if err := briefevidence.Attach(author, activities.NewStore(e.DB()),
		briefevidence.FromSentences(sentences)); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	got := sentences[0].Evidence[0].EmailSummary
	if got == nil {
		t.Fatal("the author's own cited mail carried no row; the citation cannot be opened")
	}
	if got.Subject == nil || *got.Subject != subject {
		t.Errorf("subject = %v, want %q", got.Subject, subject)
	}
	if got.ActivityId != logged.Id {
		t.Errorf("the row names activity %v, want the cited %v", got.ActivityId, logged.Id)
	}
	if got.DisplayStatus != crmcontracts.EmailAccessStatusTeam {
		t.Errorf("display_status = %q, want team for an unlimited mail", got.DisplayStatus)
	}
}

// A colleague outside a limited message's audience gets no row for it, so the
// citation names a conversation and offers nothing to press.
//
// The admit case runs FIRST, against the same colleague and the same message,
// because a refusal test on its own passes against a reader who could never see
// anything.
func TestACitedMessageCarriesNoRowForAReaderOutsideItsAudience(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Frédéric Buyer", &e.Rep1)

	subject, body := "Severance terms", "the agreed figure is confidential"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("outbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.UUID(logged.Id)
	reader := activities.NewStore(e.DB())

	open := citing(id)
	if err := briefevidence.Attach(colleague, reader, briefevidence.FromSentences(open)); err != nil {
		t.Fatalf("attaching before limiting: %v", err)
	}
	if open[0].Evidence[0].EmailSummary == nil {
		t.Fatal("the colleague got no row for an UNLIMITED mail; the refusal below would prove nothing")
	}

	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](id),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	limited := citing(id)
	if err := briefevidence.Attach(colleague, reader, briefevidence.FromSentences(limited)); err != nil {
		t.Fatalf("attaching after limiting: %v", err)
	}
	if got := limited[0].Evidence[0].EmailSummary; got != nil {
		t.Errorf("a limited mail's row reached a colleague outside its audience: %+v", *got)
	}

	// The author is in its audience and keeps the row, so the message did not
	// simply become unreadable to everyone.
	mine := citing(id)
	if err := briefevidence.Attach(author, reader, briefevidence.FromSentences(mine)); err != nil {
		t.Fatalf("attaching for the author: %v", err)
	}
	if mine[0].Evidence[0].EmailSummary == nil {
		t.Error("the author lost the row for their own limited mail")
	}
}

// A seat with no activity grant reads no rows at all, and the response still
// renders: the citations are simply not openable.
func TestASeatWithoutTheActivityGrantGetsNoRows(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Frédéric Buyer", &e.Rep1)

	subject, body := "Translation fallback decision", "Which locale wins?"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	// Every grant the lifecycle needs EXCEPT activity:read. The contact grant
	// stays, so a seat that reads contacts and not their mail is what is tested
	// rather than a seat that reads nothing.
	ungranted := e.As(e.Rep2, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	})

	sentences := citing(ids.UUID(logged.Id))
	if err := briefevidence.Attach(ungranted, activities.NewStore(e.DB()),
		briefevidence.FromSentences(sentences)); err != nil {
		t.Fatalf("a seat without the grant must render, not fail: %v", err)
	}
	if got := sentences[0].Evidence[0].EmailSummary; got != nil {
		t.Errorf("a seat with no activity grant received a row: %+v", *got)
	}
}

// A citation naming a NOTE is not an email, and the reader's own kind clause is
// what says so. Nothing here filters by kind before the read — evidence carries
// no kind word — so this is the case that proves the filter is real.
func TestACitedNoteCarriesNoEmailRow(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Frédéric Buyer", &e.Rep1)

	subject := "Rang them about the fallback"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "note", Subject: &subject,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	// The note is readable — this is not a permission case.
	if _, err := e.Activities.GetActivity(author,
		ids.From[ids.ActivityKind](ids.UUID(logged.Id)), storekit.LiveOnly); err != nil {
		t.Fatalf("the author cannot read their own note; the fixture proves nothing: %v", err)
	}

	sentences := citing(ids.UUID(logged.Id))
	if err := briefevidence.Attach(author, activities.NewStore(e.DB()),
		briefevidence.FromSentences(sentences)); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if got := sentences[0].Evidence[0].EmailSummary; got != nil {
		t.Errorf("a cited note received an email row: %+v", *got)
	}
}
