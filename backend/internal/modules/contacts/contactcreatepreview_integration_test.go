// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestContactCreationPreviewMatchesWriterAndAcquisition(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	in := CreateContactInput{FullName: "Preview Buyer", Source: "manual", Emails: []ContactEmailInput{{Email: "BUYER@EXAMPLE.TEST", EmailType: "work", IsPrimary: true}}}
	preview, err := e.store.PreviewContactCreationForActor(ctx, in, actor)
	if err != nil {
		t.Fatal(err)
	}
	if in.Emails[0].Email != "BUYER@EXAMPLE.TEST" {
		t.Fatal("preview mutated caller input")
	}
	if preview.Input.Emails[0].Email != "buyer@example.test" || preview.Acquisition.Kind != AcquiredUnknownLegacy || preview.ReviewContactID != nil {
		t.Fatalf("preview: %+v", preview)
	}
	created, err := e.store.CreateContact(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if created.OwnerId == nil || preview.Input.OwnerID == nil || ids.UUID(*created.OwnerId) != preview.Input.OwnerID.UUID {
		t.Fatal("owner differs")
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var kind string
		if err := tx.QueryRow(ctx, `SELECT kind FROM contact_acquisition_evidence WHERE contact_id = $1`, created.Id).Scan(&kind); err != nil {
			return err
		}
		if kind != preview.Acquisition.Kind {
			t.Errorf("acquisition: %s, want %s", kind, preview.Acquisition.Kind)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err = e.store.PreviewContactCreationForActor(ctx, in, actor)
	if _, ok := errors.AsType[*DuplicateEmailError](err); !ok {
		t.Fatalf("claimed email: %v", err)
	}
}

func TestContactCreationPreviewNamesDuplicateReviewWithoutCreatingIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	first, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Shared Buyer Name", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep))})
	if err != nil {
		t.Fatal(err)
	}
	actor.Permissions.RowScope = principal.RowScopeOwn
	ctx = principal.WithActor(ctx, actor)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return auth.EnsureWritable(ctx, tx, "contact", ids.UUID(first.Id))
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("review candidate unexpectedly writable: %v", err)
	}
	in := CreateContactInput{FullName: "Shared Buyer Name", Source: "manual", Acquisition: Acquisition{Kind: AcquiredReferral}}
	preview, err := e.store.PreviewContactCreationForActor(ctx, in, actor)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ReviewContactID == nil || preview.ReviewContactID.UUID != ids.UUID(first.Id) || preview.Acquisition.Kind != AcquiredReferral {
		t.Fatalf("review: %+v", preview)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM dedupe_candidate`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Error("preview wrote a review candidate")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.CreateContact(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM dedupe_candidate`).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("writer review candidates: %d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestContactCreationPreviewFindsTheLeadTheWriterPromotes(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	lead, inserted, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Existing Buyer"), Email: new("held@example.test"), Source: "manual"})
	if err != nil || !inserted {
		t.Fatalf("create lead: %v, %v", inserted, err)
	}
	in := CreateContactInput{FullName: "Confirmed Buyer", Source: "manual", Emails: []ContactEmailInput{{Email: "HELD@EXAMPLE.TEST", EmailType: "work", IsPrimary: true}}}
	preview, err := e.store.PreviewContactCreationForActor(ctx, in, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.MatchingLeadIDs) != 1 || preview.MatchingLeadIDs[0].UUID != ids.UUID(lead.Id) {
		t.Fatalf("matched leads: %+v", preview.MatchingLeadIDs)
	}
	if status, _, archived, _ := e.leadOutcome(ctx, t, ids.UUID(lead.Id)); status != "new" || archived {
		t.Fatal("preview promoted the lead")
	}
	hidden := actor
	hidden.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	hidden.Permissions.Objects["lead"] = principal.ObjectGrant{Create: true, Update: true}
	result, err := e.store.PreviewContactCreationForActor(principal.WithActor(ctx, hidden), in, actor)
	if !errors.Is(err, apperrors.ErrPermissionDenied) || len(result.MatchingLeadIDs) != 0 {
		t.Fatalf("withheld lead disclosed: %+v, %v", result, err)
	}
	contact, err := e.store.CreateContact(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	status, promotedTo, archived, trigger := e.leadOutcome(ctx, t, ids.UUID(lead.Id))
	if status != "promoted" || !archived || promotedTo == nil || *promotedTo != ids.UUID(contact.Id) || trigger != string(TriggerHumanQualify) {
		t.Fatalf("promotion differs: %s,%v,%v,%s", status, promotedTo, archived, trigger)
	}
}

func TestContactCreationPreviewKeepsTheWholeVisibleLeadScope(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	in := CreateContactInput{FullName: "Shared Identity", Source: "manual"}
	var expected []ids.UUID
	for _, email := range []string{"first-held@example.test", "second-held@example.test"} {
		lead, inserted, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Held Buyer"), Email: &email, Title: new("Director"), Phone: new("+493012345678"), Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep))})
		if err != nil || !inserted {
			t.Fatalf("create lead: %v,%v", inserted, err)
		}
		expected = append(expected, ids.UUID(lead.Id))
		in.Emails = append(in.Emails, ContactEmailInput{Email: email, EmailType: "work"})
	}
	preview, err := e.store.PreviewContactCreationForActor(ctx, in, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.MatchingLeadIDs) != len(expected) {
		t.Fatalf("partial lead scope: %+v", preview.MatchingLeadIDs)
	}
	if len(preview.Promotions) != 2 || preview.Promotions[0].Patch.After()["title"] != "Director" || len(preview.Promotions[0].Phones) != 1 || !preview.Promotions[1].Patch.Empty() || len(preview.Promotions[1].Phones) != 0 {
		t.Fatalf("cumulative promotion fields: %+v", preview.Promotions)
	}
	for i, id := range expected {
		if preview.MatchingLeadIDs[i].UUID != id {
			t.Errorf("lead order %d: %v, want %v", i, preview.MatchingLeadIDs[i], id)
		}
	}
	own := actor
	own.Permissions.RowScope = principal.RowScopeOwn
	result, err := e.store.PreviewContactCreationForActor(principal.WithActor(ctx, own), in, own)
	if err != nil || len(result.MatchingLeadIDs) != len(expected) || len(result.Promotions) != 0 {
		t.Fatalf("workspace-readable leads must remain visible with own write scope: %+v,%v", result, err)
	}
	contact, err := e.store.CreateContact(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if contact.Title == nil || *contact.Title != "Director" || contact.ConvertedFromLeadId == nil || ids.UUID(*contact.ConvertedFromLeadId) != expected[0] {
		t.Fatalf("contact fields differ: %+v", contact)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM contact_phone WHERE contact_id=$1 AND phone=$2 AND archived_at IS NULL`, contact.Id, "+493012345678").Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("carried phone count: %d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range expected {
		status, target, archived, _ := e.leadOutcome(ctx, t, id)
		if status != "promoted" || !archived || target == nil || *target != ids.UUID(contact.Id) {
			t.Fatalf("lead %s not carried: %s,%v,%v", id, status, target, archived)
		}
	}
}

func TestContactCreationDoesNotPromisePromotionsOutsideCallerAuthority(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead, inserted, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Existing Buyer"), Email: new("no-promotion@example.test"), Source: "manual"})
	if err != nil || !inserted {
		t.Fatalf("lead: %v,%v", inserted, err)
	}
	creator, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	creator.Permissions.Objects = maps.Clone(creator.Permissions.Objects)
	creator.Permissions.Objects["lead"] = principal.ObjectGrant{}
	in := CreateContactInput{FullName: "Confirmed Buyer", Source: "manual", Emails: []ContactEmailInput{{Email: "no-promotion@example.test", EmailType: "work"}}}
	preview, err := e.store.PreviewContactCreationForActor(ctx, in, creator)
	if err != nil || len(preview.MatchingLeadIDs) != 0 || len(preview.Promotions) != 0 {
		t.Fatalf("borrowed reviewer authority: %+v,%v", preview, err)
	}
	if _, err := e.store.CreateContact(principal.WithActor(ctx, creator), in); err != nil {
		t.Fatal(err)
	}
	status, target, archived, _ := e.leadOutcome(ctx, t, ids.UUID(lead.Id))
	if status != "new" || target != nil || archived {
		t.Fatalf("unexpected promotion: %s,%v,%v", status, target, archived)
	}
}

func TestContactPreviewDoesNotRevealWhetherAnUnreadableLeadMatches(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	grant := actor.Permissions.Objects["lead"]
	grant.Read = false
	actor.Permissions.Objects["lead"] = grant
	limited := principal.WithActor(ctx, actor)
	lead, inserted, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Held Buyer"), Email: new("hidden-held@example.test"), Source: "manual"})
	if err != nil || !inserted {
		t.Fatalf("lead: %v,%v", inserted, err)
	}
	for _, address := range []string{"hidden-held@example.test", "no-lead@example.test"} {
		in := CreateContactInput{FullName: "New Contact", Source: "manual", Emails: []ContactEmailInput{{Email: address, EmailType: "work"}}}
		preview, err := e.store.PreviewContactCreationForActor(limited, in, actor)
		if !errors.Is(err, apperrors.ErrPermissionDenied) || len(preview.MatchingLeadIDs) != 0 {
			t.Fatalf("address=%s preview=%+v err=%v", address, preview, err)
		}
	}
	contact, err := e.store.CreateContact(limited, CreateContactInput{FullName: "Confirmed Buyer", Source: "manual", Emails: []ContactEmailInput{{Email: "hidden-held@example.test", EmailType: "work"}}})
	if err != nil {
		t.Fatal(err)
	}
	status, target, archived, _ := e.leadOutcome(ctx, t, ids.UUID(lead.Id))
	if status != "promoted" || !archived || target == nil || *target != ids.UUID(contact.Id) {
		t.Fatalf("writer changed: %s,%v,%v", status, target, archived)
	}
}
