// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ownerRep is a Team1 rep with write but not delete on contacts and companies.
func ownerRep(e *integration.Env, user ids.UUID) context.Context {
	perms := integration.RepPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["company"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	return e.As(user, []ids.UUID{e.Team1}, perms)
}

func archivedRow(t *testing.T, e *integration.Env, table string, id ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM `+table+` WHERE id = $1 AND archived_at IS NOT NULL`, id) == 1
}

// A bulk archive by an owner without delete takes their own contacts and
// companies and skips a teammate's.
func TestABulkArchiveTakesTheOwnersContactsAndCompanies(t *testing.T) {
	e := integration.Setup(t)
	rep := ownerRep(e, e.Rep1)
	for _, tc := range []struct {
		kind  crmcontracts.BulkRecordType
		table string
		seed  func(owner ids.UUID) ids.UUID
	}{
		{crmcontracts.BulkRecordTypeContact, "contact", func(owner ids.UUID) ids.UUID {
			return e.SeedContact(t, "Contact "+ids.NewV7().String()[:8], &owner)
		}},
		{crmcontracts.BulkRecordTypeCompany, "company", func(owner ids.UUID) ids.UUID {
			return e.SeedCompany(t, "Company "+ids.NewV7().String()[:8], &owner)
		}},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			mine, theirs := tc.seed(e.Rep1), tc.seed(e.Rep2)
			item := func(id ids.UUID) crmcontracts.BulkItem {
				return crmcontracts.BulkItem{Id: openapi_types.UUID(id), Version: 1}
			}
			out, err := bulkEngineFor(e).Execute(rep, bulkChange{
				recordType: tc.kind, verb: crmcontracts.BulkVerbArchive,
				items: []crmcontracts.BulkItem{item(mine), item(theirs)},
			})
			if err != nil {
				t.Fatalf("the owner's bulk archive was refused whole: %v", err)
			}
			if out.Changed != 1 || !archivedRow(t, e, tc.table, mine) || archivedRow(t, e, tc.table, theirs) {
				t.Fatalf("changed %d, skipped %v; want the owner's own archived and the teammate's left", out.Changed, out.Skipped)
			}
		})
	}
}

// The agent's archive goes through the same rule.
func TestTheAgentArchivesTheOwnersContactAndNotATeammates(t *testing.T) {
	e := integration.Setup(t)
	rep := ownerRep(e, e.Rep1)
	provider := contacts.NewProvider(e.DB())
	mine, theirs := e.SeedContact(t, "Mine Example", &e.Rep1), e.SeedContact(t, "Theirs Example", &e.Rep2)
	archive := func(id ids.UUID) error {
		_, err := provider.ArchiveAt(rep, datasource.ArchiveInput{Ref: datasource.EntityRef{Type: datasource.EntityContact, ID: id}})
		return err
	}
	if err := archive(theirs); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the agent archiving a teammate's contact → %v, want ErrPermissionDenied", err)
	}
	if err := archive(mine); err != nil || !archivedRow(t, e, "contact", mine) {
		t.Fatalf("the agent archiving the owner's own contact → %v, want it archived", err)
	}
}

// An owner without delete cannot archive a company another owner's live deal
// still names: it would vanish from that deal. A manager can, as before.
func TestACompanyAnotherOwnersDealNamesIsAManagersToArchive(t *testing.T) {
	e := integration.Setup(t)
	rep := ownerRep(e, e.Rep1)
	company := e.SeedCompany(t, "Shared GmbH", &e.Rep1)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Rep2's deal", pipeline, open, &e.Rep2)
	e.WsExec(t, `UPDATE deal SET company_id = $1 WHERE id = $2`, company, deal)
	companyID := ids.From[ids.CompanyKind](company)

	read, err := e.Contacts.GetCompany(rep, companyID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.Archivable == nil || *read.Archivable {
		t.Errorf("the owner reads archivable=%v on a company another owner's deal names, want false", read.Archivable)
	}
	if _, err := e.Contacts.ArchiveCompany(rep, companyID, nil); !errors.As(err, new(*contacts.StillReferencedError)) {
		t.Fatalf("the owner archiving a company another owner's deal names → %v, want it refused as still referenced", err)
	}
	if _, err := e.Contacts.ArchiveCompany(e.Admin(), companyID, nil); err != nil {
		t.Fatalf("a manager archiving the same company → %v, want allowed", err)
	}
}

// An owner change that commits while the owner's archive waits on the row is
// read by that archive: it runs on the authority the row holds when written.
func TestAnArchiveWaitingOnAnOwnerChangeReadsTheNewOwner(t *testing.T) {
	e := integration.Setup(t)
	rep := ownerRep(e, e.Rep1)
	company := e.SeedCompany(t, "Handed GmbH", &e.Rep1)
	owner := integration.OwnerConn(t)
	transfer, err := owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := transfer.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	if _, err := transfer.Exec(context.Background(), `UPDATE company SET owner_id = $1 WHERE id = $2`, e.Rep2, company); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := e.Contacts.ArchiveCompany(rep, ids.From[ids.CompanyKind](company), nil)
		done <- err
	}()
	waitForLockWaiter(t, nil, `'transactionid', 'tuple'`, "the archive did not wait on the owner change")
	if err := transfer.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the archive after the company was handed on → %v, want ErrPermissionDenied", err)
	}
	if archivedRow(t, e, "company", company) {
		t.Fatal("the archive ran on the authority the row held before the hand-over")
	}
}

// An owner may decide their own agent's staged archive, by the same rule the
// archive asks; a teammate's staged archive is not theirs to decide.
func TestTheOwnerDecidesTheStagedArchiveOfTheirOwnContact(t *testing.T) {
	e := integration.Setup(t)
	rep := ownerRep(e, e.Rep1)
	svc := NewApprovalsService(e.DB())
	stage := func(contact ids.UUID) ids.ApprovalID {
		t.Helper()
		raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"contact","id":%q}`, contact)))
		if err != nil {
			t.Fatal(err)
		}
		approval, err := svc.Stage(e.Admin(), approvals.StageInput{
			Kind: "archive_record", TargetType: "contact", TargetID: contact, ProposedChange: raw, DiffHash: hash,
		})
		if err != nil {
			t.Fatal(err)
		}
		return approval
	}
	mine, theirs := e.SeedContact(t, "Mine Example", &e.Rep1), e.SeedContact(t, "Theirs Example", &e.Rep2)
	if _, err := svc.Decide(rep, stage(theirs), true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("deciding the staged archive of a teammate's contact → %v, want it not offered", err)
	}
	if _, err := svc.Decide(rep, stage(mine), true, nil); err != nil {
		t.Fatalf("the owner deciding the staged archive of their own contact → %v, want allowed", err)
	}
}
