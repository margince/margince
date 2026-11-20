// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// "Add" splits by direction, and this is the pair that says so against a real
// database.
//
// REFERENCE (naming a record you can see, from a row of your own) needs only
// read. ATTACH (hanging a child row onto a record that every reader of it will
// see) needs write authority over the record. The caller owns it, shares a
// team with its owner, or holds a `write` grant.
//
// Each test moves one thing between its arms: the grant level, or who calls.
// A refusal on its own would prove nothing. A probe that refused everything
// would pass, and so would one that refused the reference direction too. No
// CRM does that: you may name an account you can see on your own deal.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/blobstore"
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

	shareCompany(t, e, company, "read")

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

	shareCompany(t, e, company, "write")

	// The same call, the same row, the same caller: only the column moved.
	if err := fileOntoIt(); err != nil {
		t.Fatalf("a write share could not file an activity onto the shared company → %v", err)
	}
}

// attachAuthorityPerms holds every object grant the attach paths below need,
// so a refusal is about the row and never about a missing grant.
var attachAuthorityPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"company":  {Read: true, Update: true},
		"contact":  {Create: true, Read: true, Update: true},
		"deal":     {Create: true, Read: true, Update: true},
		"activity": {Create: true, Read: true, Update: true},
	},
	RowScope: principal.RowScopeTeam,
}

// attachPaths are the doors that hang a child row onto a company, each run as
// the caller in ctx. Relink first files a note on the caller's own contact,
// so the only record the move adds to is the company.
func attachPaths(t *testing.T, e *Env, company ids.UUID) map[string]func(ctx context.Context, user ids.UUID) error {
	t.Helper()
	files := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	logOnto := func(ctx context.Context, kind string, entityType string, id ids.UUID) (ids.UUID, error) {
		subject := "Rollout " + kind
		row, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
			Kind: kind, Subject: &subject, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: entityType, EntityID: id}},
		})
		return ids.UUID(row.Id), err
	}
	return map[string]func(ctx context.Context, user ids.UUID) error{
		"task": func(ctx context.Context, _ ids.UUID) error {
			_, err := logOnto(ctx, "task", "company", company)
			return err
		},
		"note": func(ctx context.Context, _ ids.UUID) error {
			_, err := logOnto(ctx, "note", "company", company)
			return err
		},
		"relink": func(ctx context.Context, user ids.UUID) error {
			own := e.SeedContact(t, "Own contact", &user)
			note, err := logOnto(ctx, "note", "contact", own)
			if err != nil {
				t.Fatalf("filing a note on the caller's own contact: %v", err)
			}
			_, err = e.Activities.RelinkActivity(ctx, ids.From[ids.ActivityKind](note),
				activities.RelinkActivityInput{EntityType: "company", EntityID: company})
			return err
		},
		"attachment": func(ctx context.Context, _ ids.UUID) error {
			_, err := files.UploadAttachment(ctx, activities.AttachmentInput{
				EntityType: "company", EntityID: company, Filename: "terms.txt", ContentType: "text/plain",
				Content: strings.NewReader("The rollout terms."),
			})
			return err
		},
	}
}

// A company every seat reads, owned by another team. Reading it lets the
// caller name it on their own deal and nothing more. Each attach door answers
// 403 until the caller holds write authority, as owner or through a write
// share. An agent acting for the reader is refused like the reader.
func TestAttachingToAReadableRecordNeedsWriteAuthority(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Brenner Logistik", &e.Rep3)
	companyID := ids.From[ids.CompanyKind](company)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	paths := attachPaths(t, e, company)

	reader := onFullSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, attachAuthorityPerms))
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, attachAuthorityPerms)
	owner := onFullSeat(e.As(e.Rep3, []ids.UUID{e.Team2}, attachAuthorityPerms))

	if _, err := e.Deals.CreateDeal(reader, deals.CreateDealInput{
		Name: "Brenner rollout", PipelineID: pipeline, StageID: open, CompanyID: &companyID, Source: "manual",
	}); err != nil {
		t.Fatalf("a reader could not name a readable company on their own deal → %v", err)
	}
	for name, attach := range paths {
		if err := attach(reader, e.Rep1); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s by a reader → %v, want permission-denied", name, err)
		}
		if err := attach(agent, e.Rep1); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s by the reader's agent → %v, want permission-denied", name, err)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE company_id = $1`, company); n != 0 {
		t.Fatalf("refused attaches left %d links on the company", n)
	}

	for name, attach := range paths {
		if err := attach(owner, e.Rep3); err != nil {
			t.Errorf("%s by the owner → %v, want success", name, err)
		}
	}
	shareCompany(t, e, company, "read")
	if err := paths["note"](reader, e.Rep1); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a note under a read share → %v, want permission-denied", err)
	}
	shareCompany(t, e, company, "write")
	for name, attach := range paths {
		if err := attach(reader, e.Rep1); err != nil {
			t.Errorf("%s under a write share → %v, want success", name, err)
		}
	}
}

// An all-scope seat holds write authority over every row. A capture-private
// company it sees only through a read share still takes nothing from it.
func TestAnAllScopeReaderCannotFileThroughAReadShare(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Halden Werke", &e.Rep3)
	e.MakeCapturePrivate(t, "company", company, e.Rep3)
	perms := attachAuthorityPerms
	perms.RowScope = principal.RowScopeAll
	reader := onFullSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, perms))
	note := attachPaths(t, e, company)["note"]

	shareCompany(t, e, company, "read")
	if err := note(reader, e.Rep1); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a note through a read share → %v, want permission-denied", err)
	}
	shareCompany(t, e, company, "write")
	if err := note(reader, e.Rep1); err != nil {
		t.Fatalf("a note through a write share → %v, want success", err)
	}
}

// shareCompany writes Rep3's grant to Rep1 through the REAL writer, at one
// access level. Its upsert is the path an installation takes from `read` to
// `write`, so a hand-inserted row would prove the rule against a state
// production cannot reach.
func shareCompany(t *testing.T, e *Env, company ids.UUID, access string) {
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
