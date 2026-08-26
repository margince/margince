// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A promotion's whole job is to name a person. A lead that names nobody must
// therefore be refused rather than promoted into a row with an empty name —
// one no search finds, no merge matches and no rep recognises.
//
// The lead is seeded through CreateLead, which is the reason this test is here
// and not beside the unit spec: nothing between the request body and the row
// refuses a full_name that is present and empty, so the state under test is one
// the shipped writer really produces.

import (
	"context"
	"errors"
	"testing"

	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// newPromoteIdentityEnv is a caller who may work leads AND create people. The
// person grant is the point: without it every promotion answers
// permission denied, and a test asserting a refusal would read green on an
// authz denial it never meant to provoke.
func newPromoteIdentityEnv(t *testing.T) (context.Context, *Store) {
	t.Helper()
	ctx, store, _ := newLeadScoreEnvWithBase(t)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the lead-score environment carries no actor, so this test cannot widen its grants")
	}
	actor.Permissions.Objects["person"] = principal.ObjectGrant{
		Create: true, Read: true, Update: true, Delete: true,
	}
	return principal.WithActor(ctx, actor), store
}

func TestPromotingALeadThatNamesNobodyIsRefused(t *testing.T) {
	ctx, store := newPromoteIdentityEnv(t)

	for _, tc := range []struct {
		name string
		in   CreateLeadInput
	}{
		{
			name: "no full_name and no email",
			in:   CreateLeadInput{Title: ptr("VP Sales"), Source: "webform", Status: "new"},
		},
		{
			// The one `FullName != nil` cannot see: a full_name is present,
			// so a nil check calls this lead identified and the ladder then
			// has nothing to match on.
			name: "a present-but-empty full_name and no email",
			in:   CreateLeadInput{FullName: ptr(""), Title: ptr("VP Sales"), Source: "webform", Status: "new"},
		},
		{
			name: "a full_name that is only padding, and no email",
			in:   CreateLeadInput{FullName: ptr("   "), Title: ptr("VP Sales"), Source: "webform", Status: "new"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lead, _, err := store.CreateLead(ctx, tc.in)
			if err != nil {
				t.Fatalf("seeding a lead the writer accepts: %v", err)
			}
			leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))

			// The preview is what a human reads before agreeing, so it has to
			// answer the same way. A preview promising "create" over an act
			// that refuses is the worse half of the two.
			var previewNeedsIdentity *PromoteNeedsIdentityError
			preview, perr := store.PreviewLeadPromotion(ctx, leadID)
			if !errors.As(perr, &previewNeedsIdentity) {
				t.Errorf("previewing a lead that names nobody: got outcome %q (err=%v), want a "+
					"PromoteNeedsIdentityError — the preview and the promotion answer one "+
					"question and a human acts on the preview's answer",
					preview.Outcome, perr)
			}

			person, merged, err := store.PromoteLead(ctx, leadID, PromoteLeadInput{Trigger: "human_qualify"})
			var needsIdentity *PromoteNeedsIdentityError
			if !errors.As(err, &needsIdentity) {
				t.Fatalf("promoting a lead that names nobody: got person %q (merged=%v, err=%v), "+
					"want a PromoteNeedsIdentityError", person.FullName, merged, err)
			}

			// The refusal has to leave nothing behind: a lead marked promoted
			// with no person is worse than either half.
			after, rerr := store.GetLead(ctx, leadID, storekit.IncludeArchived)
			if rerr != nil {
				t.Fatalf("reading the lead back: %v", rerr)
			}
			if after.Status == "promoted" || after.PromotedPersonId != nil {
				t.Errorf("the refused lead is status=%q with promoted_person_id=%v; a refused "+
					"promotion must not stamp the outcome it declined to produce",
					after.Status, after.PromotedPersonId)
			}
		})
	}
}

// The email is what names a lead with no name of its own, so this one must
// still promote — the guard refuses leads that name nobody, not leads that are
// merely unnamed.
func TestALeadNamedOnlyByItsEmailStillPromotes(t *testing.T) {
	ctx, store := newPromoteIdentityEnv(t)

	lead, _, err := store.CreateLead(ctx, CreateLeadInput{
		FullName: ptr(""),
		Email:    ptr("vera@nordwind.example"),
		Source:   "webform",
		Status:   "new",
	})
	if err != nil {
		t.Fatalf("seeding a lead: %v", err)
	}
	person, _, err := store.PromoteLead(ctx,
		ids.From[ids.LeadKind](ids.UUID(lead.Id)), PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil {
		t.Fatalf("promoting a lead named by its email: %v", err)
	}
	if person.FullName != "vera@nordwind.example" {
		t.Errorf("the promoted person is named %q, want the email that named the lead — the "+
			"ladder matched on it, so the person has to be stored under it",
			person.FullName)
	}
}
