// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The company page's one-line description: written on create, edited in
// place, cleared to absent, and inherited by a merge survivor that has none.
// A column the page renders unconditionally has to survive every one of those
// paths, not only the create.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOrganizationDescriptionRoundTripsThroughCreateAndUpdate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	written := "Supplies architectural glazing and modular walls to builders."
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Glazed Frog GmbH", Source: "manual",
		Description: &written,
	})
	if err != nil {
		t.Fatal(err)
	}
	if org.Description == nil || *org.Description != written {
		t.Fatalf("create returned description %v, want %q", org.Description, written)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	// A re-read proves the value came back from the column, not from the
	// input struct the create path happened to be holding.
	reread, err := e.store.GetOrganization(ctx, orgID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Description == nil || *reread.Description != written {
		t.Fatalf("re-read description = %v, want %q", reread.Description, written)
	}

	edited := "Supplies architectural glazing to builders and architects."
	updated, err := e.store.UpdateOrganization(ctx, orgID, UpdateOrganizationInput{
		Description: &edited,
	})
	if err != nil {
		t.Fatalf("edit description: %v", err)
	}
	if updated.Description == nil || *updated.Description != edited {
		t.Fatalf("edited description = %v, want %q", updated.Description, edited)
	}

	// An empty string is the clear, the same spelling every other nullable
	// text column on this record uses.
	empty := ""
	cleared, err := e.store.UpdateOrganization(ctx, orgID, UpdateOrganizationInput{
		Description: &empty,
	})
	if err != nil {
		t.Fatalf("clear description: %v", err)
	}
	if cleared.Description != nil && *cleared.Description != "" {
		t.Fatalf("cleared description = %v, want empty or nil", cleared.Description)
	}
}

// A nil Description leaves the column untouched, so an edit that only moves
// the lifecycle cannot silently wipe the line someone wrote.
func TestOrganizationDescriptionSurvivesAnUnrelatedEdit(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	written := "Runs the regional glazing supply network."
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Untouched GmbH", Source: "manual", Description: &written,
	})
	if err != nil {
		t.Fatal(err)
	}
	industry := "Building products"
	updated, err := e.store.UpdateOrganization(ctx,
		ids.From[ids.OrganizationKind](ids.UUID(org.Id)),
		UpdateOrganizationInput{Industry: &industry})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description == nil || *updated.Description != written {
		t.Fatalf("description after an unrelated edit = %v, want %q", updated.Description, written)
	}
}

// The survivorship rule is fill-where-blank: a survivor with no description
// inherits one, a survivor that already has one keeps its own.
func TestOrganizationDescriptionFillsOnlyABlankMergeSurvivor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	retiredLine := "Fabricates aluminium window systems."

	blankSurvivor, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Blank Survivor GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	retired, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Retired One GmbH", Source: "manual", Description: &retiredLine,
	})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := e.store.MergeOrganization(ctx,
		ids.From[ids.OrganizationKind](ids.UUID(retired.Id)),
		ids.From[ids.OrganizationKind](ids.UUID(blankSurvivor.Id)))
	if err != nil {
		t.Fatalf("merge into blank survivor: %v", err)
	}
	if merged.Description == nil || *merged.Description != retiredLine {
		t.Fatalf("blank survivor description = %v, want the retired record's %q", merged.Description, retiredLine)
	}

	ownLine := "Installs curtain walling on commercial builds."
	heldSurvivor, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Held Survivor GmbH", Source: "manual", Description: &ownLine,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondRetired, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Retired Two GmbH", Source: "manual", Description: &retiredLine,
	})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := e.store.MergeOrganization(ctx,
		ids.From[ids.OrganizationKind](ids.UUID(secondRetired.Id)),
		ids.From[ids.OrganizationKind](ids.UUID(heldSurvivor.Id)))
	if err != nil {
		t.Fatalf("merge into held survivor: %v", err)
	}
	if kept.Description == nil || *kept.Description != ownLine {
		t.Fatalf("held survivor description = %v, want its own %q", kept.Description, ownLine)
	}
}

// A site read REPLACES a description no person authored. An agent creating the
// company from a meeting transcript writes a summary of the meeting into the
// header, and the crawl that knows what the company actually sells has to be
// able to correct it — under the old fill-where-blank rule the transcript's
// sentence won permanently, because whoever wrote first won.
func TestASiteReadReplacesAnAgentsDescription(t *testing.T) {
	e := setupDedupe(t)
	fromTranscript := "Discovery call: they run two shops and plan to internationalise."
	org, err := e.store.CreateOrganization(e.asAgent(), CreateOrganizationInput{
		DisplayName: "Transcript GmbH", Source: "manual", Description: &fromTranscript,
	})
	if err != nil {
		t.Fatal(err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	fromSite := "Sells bearings, rolling and linear motion technology online."
	applySiteReadDescription(e.as(), t, e, orgID, fromSite)

	reread, err := e.store.GetOrganization(e.as(), orgID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Description == nil || *reread.Description != fromSite {
		t.Fatalf("description after the site read = %v, want the site's %q", reread.Description, fromSite)
	}
}

// The same read leaves a person's sentence alone. This is the half that makes
// the replacement above safe, and it is the case the fill-where-blank rule used
// to cover for free.
func TestASiteReadLeavesAHumansDescriptionAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Typed GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	// Through the real editor, so the provenance the guard reads is the one
	// production writes rather than a row this test invented.
	typed := "Our own words about what we do, kept deliberately."
	if _, err := e.store.UpdateOrganization(ctx, orgID, UpdateOrganizationInput{Description: &typed}); err != nil {
		t.Fatal(err)
	}

	applySiteReadDescription(ctx, t, e, orgID, "Whatever the marketing page happens to claim.")

	reread, err := e.store.GetOrganization(ctx, orgID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Description == nil || *reread.Description != typed {
		t.Fatalf("description after the site read = %v, want the human's %q", reread.Description, typed)
	}
}

// A description a person supplies AT CREATE is theirs too. The edit path is
// the obvious one to protect and was protected first; a create that stamped
// nothing would leave the very first sentence somebody wrote unclaimed, and the
// next crawl would take it.
func TestASiteReadLeavesAHumansCreatedDescriptionAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	typed := "What we tell people we do, written when the record was made."
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Created With Words GmbH", Source: "manual", Description: &typed,
	})
	if err != nil {
		t.Fatal(err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	applySiteReadDescription(ctx, t, e, orgID, "Whatever the marketing page happens to claim.")

	reread, err := e.store.GetOrganization(ctx, orgID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Description == nil || *reread.Description != typed {
		t.Fatalf("description after the site read = %v, want the human's %q", reread.Description, typed)
	}
}

// A description inherited by a merge survivor keeps the author it had. The
// survivorship fill moves a person's sentence onto a record that never had one,
// and if the claim did not travel with it the next site read of the survivor
// would replace words a person wrote.
func TestAMergedInDescriptionKeepsItsHumanAuthor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	typed := "The sentence a person wrote on the record that lost the merge."
	retired, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Retired Author GmbH", Source: "manual", Description: &typed,
	})
	if err != nil {
		t.Fatal(err)
	}
	survivor, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Surviving Blank GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	survivorID := ids.From[ids.OrganizationKind](ids.UUID(survivor.Id))
	if _, err := e.store.MergeOrganization(ctx,
		ids.From[ids.OrganizationKind](ids.UUID(retired.Id)), survivorID); err != nil {
		t.Fatalf("merge: %v", err)
	}

	applySiteReadDescription(ctx, t, e, survivorID, "Whatever the marketing page happens to claim.")

	reread, err := e.store.GetOrganization(ctx, survivorID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Description == nil || *reread.Description != typed {
		t.Fatalf("survivor description after the site read = %v, want the inherited human sentence %q",
			reread.Description, typed)
	}
}

// An AGENT editing the description through update_record does not claim the
// column: the stamp records the authenticated principal, and only a human:<id>
// holds it. Without this the governed write surface would be a way to freeze a
// company header against the site read that could correct it.
func TestAnAgentsEditDoesNotClaimTheDescription(t *testing.T) {
	e := setupDedupe(t)
	org, err := e.store.CreateOrganization(e.as(), CreateOrganizationInput{
		DisplayName: "Agent Edited GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	fromAgent := "What the agent concluded from a call."
	if _, err := e.store.UpdateOrganization(e.asAgent(), orgID,
		UpdateOrganizationInput{Description: &fromAgent}); err != nil {
		t.Fatal(err)
	}

	fromSite := "What the website says the company sells."
	applySiteReadDescription(e.as(), t, e, orgID, fromSite)

	reread, err := e.store.GetOrganization(e.as(), orgID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Description == nil || *reread.Description != fromSite {
		t.Fatalf("description after the site read = %v, want the site's %q — an agent's edit must not "+
			"hold the column", reread.Description, fromSite)
	}
}

// An agent re-sending a human's description UNCHANGED does not take the column
// from them. The patch records an assignment without comparing, so "the request
// named this field" and "the value moved" are different questions; answering
// the first would let any agent write that echoes a record back strip the
// human's claim and hand the sentence to the next crawl.
func TestAnAgentEchoingAHumansDescriptionDoesNotTakeIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	typed := "The sentence a person wrote and an agent merely repeats."
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Echoed GmbH", Source: "manual", Description: &typed,
	})
	if err != nil {
		t.Fatal(err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	// The same value back, the way a round-tripping agent write sends it.
	same := typed
	if _, err := e.store.UpdateOrganization(e.asAgent(), orgID,
		UpdateOrganizationInput{Description: &same}); err != nil {
		t.Fatal(err)
	}

	applySiteReadDescription(ctx, t, e, orgID, "Whatever the marketing page happens to claim.")

	reread, err := e.store.GetOrganization(ctx, orgID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Description == nil || *reread.Description != typed {
		t.Fatalf("description after the site read = %v, want the human's %q — an agent echoing a value "+
			"unchanged must not become its author", reread.Description, typed)
	}
}

// A description that predates field-level stamping is protected by the row's
// own captured_by, which is what the 1787560000 backfill writes into
// field_provenance for every organization that already had one.
//
// The stamp is deleted here to reproduce the pre-migration shape, because a
// record created by the current code always has one and so cannot exhibit the
// case. What is asserted is the backfill's premise: the row's captured_by is a
// sound answer to who wrote the description when nothing else recorded it.
func TestABackfilledLegacyDescriptionIsHeldByItsRowAuthor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	typed := "Written long before anybody stamped a field."
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Legacy Words GmbH", Source: "manual", Description: &typed,
	})
	if err != nil {
		t.Fatal(err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
	stripDescriptionStamp(ctx, t, e, orgID)
	backfillDescriptionAuthors(ctx, t, e)

	applySiteReadDescription(ctx, t, e, orgID, "Whatever the marketing page happens to claim.")

	reread, err := e.store.GetOrganization(ctx, orgID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Description == nil || *reread.Description != typed {
		t.Fatalf("description after the site read = %v, want the legacy human sentence %q — the backfill "+
			"must claim it from the row's own captured_by", reread.Description, typed)
	}
}

// stripDescriptionStamp removes the field-level provenance a current create
// writes, leaving the row in the shape every pre-migration record is in.
func stripDescriptionStamp(ctx context.Context, t *testing.T, e *dedupeEnv, orgID ids.OrganizationID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM field_provenance
			WHERE object_type = 'organization' AND object_id = $1 AND field_name = 'description'`, orgID)
		return err
	}); err != nil {
		t.Fatalf("clearing the description stamp: %v", err)
	}
}

// backfillDescriptionAuthors runs the same statement migration 1787560000
// runs. It is spelled here rather than read from the file because the lane
// applies migrations before the test starts, so the only way to exercise the
// backfill against a row this test made is to run it again.
func backfillDescriptionAuthors(ctx context.Context, t *testing.T, e *dedupeEnv) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO field_provenance (object_type, object_id, field_name, source, captured_by, captured_at)
			SELECT 'organization', o.id, 'description', o.source, o.captured_by, o.created_at
			FROM organization o
			WHERE o.description IS NOT NULL
			  AND NOT EXISTS (
			    SELECT 1 FROM field_provenance fp
			    WHERE fp.object_type = 'organization'
			      AND fp.object_id = o.id
			      AND fp.field_name = 'description')`)
		return err
	}); err != nil {
		t.Fatalf("running the description-author backfill: %v", err)
	}
}

// applySiteReadDescription drives the real accept path, so what these cases
// prove is what a scheduled re-read of the site does.
func applySiteReadDescription(ctx context.Context, t *testing.T, e *dedupeEnv, orgID ids.OrganizationID, summary string) {
	t.Helper()
	if err := e.store.ApplyDeepRead(ctx, DeepReadProposal{
		OrganizationID: orgID,
		SourceURL:      "https://example.test/",
		SiteReadID:     ids.NewV7(),
		Fields: []DeepReadField{{
			Field: fieldOfferSummary, Value: summary,
			EvidenceSnippet: summary, SourceURL: "https://example.test/",
			Confidence: 0.9,
		}},
	}); err != nil {
		t.Fatalf("ApplyDeepRead: %v", err)
	}
}

// The 500-character bound is the database's, so it holds for every writer and
// not only the ones that remember to check.
func TestOrganizationDescriptionRefusesAPastedParagraph(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	tooLong := strings.Repeat("a", 501)
	if _, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Overlong GmbH", Source: "manual", Description: &tooLong,
	}); err == nil {
		t.Fatal("a 501-character description was accepted; the column CHECK must refuse it")
	}
}
