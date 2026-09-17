// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// "Add" splits by direction, and this is the pair that says so against a real
// database.
//
// REFERENCE — naming a record you can see, from a row of your own — needs only
// read. ATTACH — hanging a child row onto somebody else's record, where every
// reader of that record will now see it — needs a share that is not read-only.
//
// One company, one caller, one grant, and the only thing that moves between the
// arms is `record_grant.access`. A refusal on its own would prove nothing here:
// a probe that refused everything would pass it, and so would one that refused
// the reference direction too — which is the failure this split exists to
// prevent, since no CRM lets you see an account and not sell to it.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attachDirectionPerms is a rep who may create deals and activities and read
// companies — everything both directions need, so a refusal below is about the
// share and never about a missing object grant.
var attachDirectionPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"company":  {Read: true},
		"deal":     {Create: true, Read: true, Update: true},
		"activity": {Create: true, Read: true, Update: true},
	},
	RowScope: principal.RowScopeTeam,
}

func TestAReadShareNamesTheCompanyAndCannotFileOntoIt(t *testing.T) {
	e := Setup(t)
	// Capture-private, which is what makes the share rep1's ONLY path to the
	// row: a plain company is readable by every seat, and then every outcome
	// below would be attributable to the scope tier rather than to the grant.
	company := e.SeedCompany(t, "Halden Werke", &e.Rep3)
	e.MakeCapturePrivate(t, "company", company, e.Rep3)
	companyID := ids.From[ids.CompanyKind](company)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)

	holder := e.As(e.Rep1, []ids.UUID{e.Team1}, attachDirectionPerms)
	fileOntoIt := func() error {
		subject := "Renewal terms"
		_, _, err := e.Activities.LogActivity(holder, activities.LogActivityInput{
			Kind: "note", Subject: &subject,
			Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}},
		})
		return err
	}
	nameItOnADeal := func(name string) error {
		_, err := e.Deals.CreateDeal(holder, deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: open,
			CompanyID: &companyID, Source: "manual",
		})
		return err
	}

	// No share at all: the row is invisible, and BOTH directions answer
	// not-found. Existence stays hidden — this is the refusal the share is
	// about to change the shape of.
	if err := fileOntoIt(); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("filing onto an invisible company → %v, want not-found", err)
	}
	if err := nameItOnADeal("Unshared"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("naming an invisible company on a new deal → %v, want not-found", err)
	}

	shareAt(t, e, company, "read")

	// The reference direction is open: the deal is rep1's own row, and the
	// shared company is a value in it. Refusing this would make a read share
	// unusable for ordinary work.
	if err := nameItOnADeal("Halden renewal"); err != nil {
		t.Fatalf("a read share could not name the company on the holder's own deal → %v", err)
	}
	// The attach direction is not, and the refusal CHANGES SHAPE: the holder
	// already knows the row exists — the share told them — so hiding it behind
	// a not-found would be a lie rather than a protection.
	if err := fileOntoIt(); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a read share filed an activity onto the shared company → %v, want permission-denied", err)
	}

	shareAt(t, e, company, "write")

	// The same call, the same row, the same caller: only the column moved.
	if err := fileOntoIt(); err != nil {
		t.Fatalf("a write share could not file an activity onto the shared company → %v", err)
	}
}

// shareAt writes the grant through the REAL writer, at one access level. Its
// upsert is the path an installation actually takes from `read` to `write`, so
// a hand-inserted row would prove the rule against a state production cannot
// reach.
func shareAt(t *testing.T, e *Env, company ids.UUID, access string) {
	t.Helper()
	owner := e.As(e.Rep3, nil, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"company": {Read: true, Update: true}},
		RowScope: principal.RowScopeOwn,
	})
	if _, err := identity.NewService(e.Pool).CreateRecordGrant(owner, identity.CreateGrantInput{
		RecordType: "company", RecordID: company,
		SubjectType: "user", SubjectID: e.Rep1, Access: access,
	}); err != nil {
		t.Fatalf("owner shares at %s → %v", access, err)
	}
}
