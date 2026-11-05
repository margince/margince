// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
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
		return auth.EnsureWritable(ctx, tx, contactEntity, ids.UUID(first.Id))
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

func TestAContactCreateOverALiveLeadIsRefusedAndWritesNothing(t *testing.T) {
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
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	in := CreateContactInput{FullName: "Confirmed Buyer", Source: "manual", Emails: []ContactEmailInput{{Email: "HELD@EXAMPLE.TEST", EmailType: "work", IsPrimary: true}}}
	_, err = e.store.PreviewContactCreationForActor(ctx, in, actor)
	if held := leadHeldRefusal(t, err); len(held.LeadIDs) != 1 || held.LeadIDs[0] != leadID || !held.Convertible || held.Field != fieldEmails {
		t.Fatalf("preview refusal: %+v", held)
	}
	_, err = e.store.CreateContact(ctx, in)
	if held := leadHeldRefusal(t, err); len(held.LeadIDs) != 1 || held.LeadIDs[0] != leadID || !held.Convertible {
		t.Fatalf("writer refusal: %+v", held)
	}
	e.assertNoContactHolds(ctx, t, "held@example.test")
	if status, _, archived, _ := e.leadOutcome(ctx, t, ids.UUID(lead.Id)); status != "new" || archived {
		t.Fatalf("a refused create changed the lead: %s, archived %v", status, archived)
	}
}

func TestAContactWriteNeverCombinesDistinctLeadHolders(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	in := CreateContactInput{FullName: "Shared Identity", Source: "manual"}
	var expected []ids.LeadID
	for _, email := range []string{"first-held@example.test", "second-held@example.test"} {
		lead, inserted, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Held Buyer"), Email: &email, Title: new("Director"), Phone: new("+493012345678"), Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep))})
		if err != nil || !inserted {
			t.Fatalf("create lead: %v,%v", inserted, err)
		}
		expected = append(expected, ids.From[ids.LeadKind](ids.UUID(lead.Id)))
		in.Emails = append(in.Emails, ContactEmailInput{Email: email, EmailType: "work"})
	}
	_, err := e.store.CreateContact(ctx, in)
	if held := leadHeldRefusal(t, err); len(held.LeadIDs) != 2 || held.LeadIDs[0] != expected[0] || held.LeadIDs[1] != expected[1] {
		t.Fatalf("the refusal names every holder oldest first: %+v", held)
	}
	in.ConvertLeadIDs = []ids.LeadID{expected[1], expected[0]}
	_, err = e.store.CreateContact(ctx, in)
	if held := leadHeldRefusal(t, err); held.Convertible || len(held.LeadIDs) != 2 {
		t.Fatalf("an ambiguous pair was offered as one conversion: %+v", held)
	}
	for _, id := range expected {
		status, target, archived, _ := e.leadOutcome(ctx, t, id.UUID)
		if status != "new" || archived || target != nil {
			t.Fatalf("ambiguous lead %s changed: %s,%v,%v", id, status, target, archived)
		}
	}
}

func TestAConvertThatNamesOtherLeadsThanTheHoldersIsAskedAgain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, email := range []string{"one@example.test", "two@example.test"} {
		if _, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Held Buyer"), Email: &email, Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	in := CreateContactInput{FullName: "Held Buyer", Source: "manual", Emails: []ContactEmailInput{{Email: "one@example.test", EmailType: "work"}, {Email: "two@example.test", EmailType: "work"}}}
	_, err := e.store.CreateContact(ctx, in)
	held := leadHeldRefusal(t, err)
	in.ConvertLeadIDs = held.LeadIDs[:1]
	_, err = e.store.CreateContact(ctx, in)
	if again := leadHeldRefusal(t, err); len(again.LeadIDs) != 2 {
		t.Fatalf("a convert naming one of two holders: %+v", again)
	}
	e.assertNoContactHolds(ctx, t, "one@example.test")
}

func TestALeadHeldRefusalOffersNoConvertWithoutThePromotionGrants(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Existing Buyer"), Email: new("no-promotion@example.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	creator, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	creator.Permissions.Objects = maps.Clone(creator.Permissions.Objects)
	creator.Permissions.Objects[leadEntity] = principal.ObjectGrant{Read: true}
	limited := principal.WithActor(ctx, creator)
	in := CreateContactInput{FullName: "Confirmed Buyer", Source: "manual", Emails: []ContactEmailInput{{Email: "no-promotion@example.test", EmailType: "work"}}}
	_, err = e.store.CreateContact(limited, in)
	if held := leadHeldRefusal(t, err); held.Convertible || len(held.LeadIDs) != 1 {
		t.Fatalf("a caller who may not promote: %+v", held)
	}
	in.ConvertLeadIDs = []ids.LeadID{ids.From[ids.LeadKind](ids.UUID(lead.Id))}
	if _, err := e.store.CreateContact(limited, in); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a convert without the grants: %v", err)
	}
	if status, target, archived, _ := e.leadOutcome(ctx, t, ids.UUID(lead.Id)); status != "new" || target != nil || archived {
		t.Fatalf("unexpected promotion: %s,%v,%v", status, target, archived)
	}
}

func TestALeadHeldRefusalNamesNoLeadTheCallerCannotRead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	grant := actor.Permissions.Objects[leadEntity]
	grant.Read = false
	actor.Permissions.Objects[leadEntity] = grant
	limited := principal.WithActor(ctx, actor)
	if _, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Held Buyer"), Email: new("hidden-held@example.test"), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	in := CreateContactInput{FullName: "New Contact", Source: "manual", Emails: []ContactEmailInput{{Email: "hidden-held@example.test", EmailType: "work"}}}
	_, err := e.store.PreviewContactCreationForActor(limited, in, actor)
	if held := leadHeldRefusal(t, err); len(held.LeadIDs) != 0 || held.Convertible {
		t.Fatalf("preview disclosed a lead the caller cannot read: %+v", held)
	}
	_, err = e.store.CreateContact(limited, in)
	if held := leadHeldRefusal(t, err); len(held.LeadIDs) != 0 || held.Convertible || held.Field != "" {
		t.Fatalf("writer disclosed a lead the caller cannot read: %+v", held)
	}
	rec := httptest.NewRecorder()
	writeStoreErr(rec, httptest.NewRequest(http.MethodPost, "/v1/contacts", nil), err)
	var body struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if jsonErr := json.Unmarshal(rec.Body.Bytes(), &body); jsonErr != nil || body.Code != CodeLeadHoldsEmail || len(body.Details) != 0 {
		t.Fatalf("the wire answer for a hidden lead holder: %s", rec.Body.String())
	}
	e.assertNoContactHolds(ctx, t, "hidden-held@example.test")
}

func TestAnAgentCannotConvertALeadThroughAContactCreate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Existing Buyer"), Email: new("agent-held@example.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	agent, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	agent.Type, agent.ID, agent.OnBehalfOf = principal.PrincipalAgent, "agent:tools", agent.UserID
	in := CreateContactInput{
		FullName: "Confirmed Buyer", Source: "manual",
		Emails:         []ContactEmailInput{{Email: "agent-held@example.test", EmailType: "work"}},
		ConvertLeadIDs: []ids.LeadID{ids.From[ids.LeadKind](ids.UUID(lead.Id))},
	}
	_, err = e.store.CreateContact(principal.WithActor(ctx, agent), in)
	if refused, ok := errors.AsType[*values.ParseError](err); !ok || refused.Field != fieldConvertLeadIDs {
		t.Fatalf("an agent's convert: %v", err)
	}
	if status, _, archived, _ := e.leadOutcome(ctx, t, ids.UUID(lead.Id)); status != "new" || archived {
		t.Fatalf("an agent converted the lead: %s,%v", status, archived)
	}
}

func TestALinkedInProfileALeadHoldsIsIdentityInAnySpelling(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Sam Profile"), LinkedInURL: new("https://www.linkedin.com/in/sam-profile"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	in := CreateContactInput{FullName: "Sam Profile", Source: "manual", Social: map[string]any{SocialLinkedIn: "http://WWW.LinkedIn.com/in/sam-profile/?trk=x"}}
	_, err = e.store.CreateContact(ctx, in)
	if held := leadHeldRefusal(t, err); held.Field != fieldSocialLinkedIn || len(held.LeadIDs) != 1 || held.LeadIDs[0] != leadID {
		t.Fatalf("a lead's profile in another spelling: %+v", held)
	}
	// A bare handle is stored as typed and names nobody.
	if _, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Sam Handle", Source: "manual", Social: map[string]any{SocialLinkedIn: "sam-profile"}}); err != nil {
		t.Fatalf("a handle that is no profile URL: %v", err)
	}
	in.ConvertLeadIDs = []ids.LeadID{leadID}
	contact, err := e.store.CreateContact(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if status, target, archived, _ := e.leadOutcome(ctx, t, leadID.UUID); status != "promoted" || !archived || target == nil || *target != ids.UUID(contact.Id) {
		t.Fatalf("a lead sharing only a profile was not converted into the contact: %s,%v,%v", status, target, archived)
	}
}

// leadHeldRefusal is err as the lead-held refusal, failing the test otherwise.
func leadHeldRefusal(t *testing.T, err error) *LeadHoldsIdentityError {
	t.Helper()
	held, ok := errors.AsType[*LeadHoldsIdentityError](err)
	if !ok {
		t.Fatalf("want the lead-held refusal, got %v", err)
	}
	return held
}

// assertNoContactHolds fails when any contact holds email: the refused write
// left nothing behind.
func (e *dedupeEnv) assertNoContactHolds(ctx context.Context, t *testing.T, email string) {
	t.Helper()
	if n := e.count(ctx, t, `SELECT count(*) FROM contact_email WHERE email = $1`, email); n != 0 {
		t.Fatalf("a refused write left %d contact addresses for %s", n, email)
	}
}

func (e *dedupeEnv) count(ctx context.Context, t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}
