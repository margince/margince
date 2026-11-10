// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/installseam"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnAgreementRacingADealCompanyChangeRechecksTheCompany(t *testing.T) {
	contractCompanyRace(t, false)
}

func TestAnAgreementUpdateRacingADealCompanyChangeRechecksTheCompany(t *testing.T) {
	contractCompanyRace(t, true)
}

func contractCompanyRace(t *testing.T, update bool) {
	t.Helper()
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	companyA, companyB := e.SeedCompany(t, "Original company", nil), e.SeedCompany(t, "Other company", nil)
	deal := e.SeedDeal(t, "Renewal", pipeline, open, &e.Rep1)
	anchorOnCompany(t, e, deal, companyA)
	var existing ids.ContractID
	if update {
		existing = seedContract(t, e, contracts.CreateContractInput{CompanyID: companyIDOf(companyA), Title: "Agreement", ValueBasis: contracts.BasisTotal, Source: "manual"})
	}
	ctx, cancel := context.WithTimeout(e.Admin(), 20*time.Second)
	defer cancel()
	proceed, held := make(chan struct{}), make(chan int32, 1)
	release := sync.OnceFunc(func() { close(proceed) })
	defer release()
	installation := installseam.Deals()
	check := installation.EnsureContractsShareCompany
	installation.EnsureContractsShareCompany = func(ctx context.Context, tx pgx.Tx, deal ids.DealID, company ids.CompanyID) error {
		if err := check(ctx, tx, deal, company); err != nil {
			return err
		}
		var pid int32
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return err
		}
		held <- pid
		select {
		case <-proceed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	updated := make(chan error, 1)
	go func() {
		target := companyIDOf(companyB)
		_, err := deals.NewStore(e.DB(), installation).UpdateDeal(ctx, ids.From[ids.DealKind](deal), deals.UpdateDealInput{CompanyID: &target})
		updated <- err
	}()
	var holder int32
	select {
	case holder = <-held:
	case err := <-updated:
		t.Fatalf("deal update failed before the race: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	created, returned := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(returned)
		if update {
			dealID := crmcontracts.Id(deal)
			_, err := e.Contracts.UpdateContract(ctx, existing, crmcontracts.UpdateContractRequest{DealId: &dealID}, nil, nil)
			created <- err
			return
		}
		_, err := e.Contracts.CreateContract(ctx, contracts.CreateContractInput{CompanyID: companyIDOf(companyA), DealID: dealIDPtr(deal), Title: "Agreement", ValueBasis: contracts.BasisTotal, Source: "manual"})
		created <- err
	}()
	testdb.AwaitQueuedBehind(t, OwnerConn(t), holder, 1, returned, "the agreement writer")
	release()
	if err := <-updated; err != nil {
		t.Fatal(err)
	}
	var mismatch *contracts.CrossCompanyLinkError
	if err := <-created; !errors.As(err, &mismatch) {
		t.Fatalf("the agreement did not recheck the new deal company: %v", err)
	}
}

func TestALegacyContractWithADifferentCompanyCanBeRepairedByItsAdministrator(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	companyA, companyB := e.SeedCompany(t, "Original company", nil), e.SeedCompany(t, "Other company", nil)
	deal := e.SeedDeal(t, "Renewal", pipeline, open, &e.Rep1)
	anchorOnCompany(t, e, deal, companyA)
	agreement := seedContract(t, e, contracts.CreateContractInput{CompanyID: companyIDOf(companyA), DealID: dealIDPtr(deal), Title: "Agreement", ValueBasis: contracts.BasisTotal, Source: "manual"})
	e.WsExec(t, `UPDATE deal SET company_id=$1 WHERE id=$2`, companyB, deal)
	if _, err := e.Contracts.GetContract(e.Admin(), agreement); err != nil {
		t.Fatalf("administrator cannot read legacy agreement: %v", err)
	}
	if _, err := e.Contracts.UpdateContract(e.Admin(), agreement, crmcontracts.UpdateContractRequest{}, []string{"deal_id"}, nil); err != nil {
		t.Fatalf("administrator cannot detach inconsistent deal: %v", err)
	}
	repaired, err := e.Contracts.GetContract(e.Admin(), agreement)
	if err != nil || repaired.DealId != nil {
		t.Fatalf("the inconsistent deal link survived repair: %v, %v", repaired.DealId, err)
	}
}

func TestFilingAgainstALegacyAgreementUsesTheAgreementsCompany(t *testing.T) {
	e := Setup(t)
	companyA := e.SeedCompany(t, "Agreement company", &e.Rep2)
	companyB := e.SeedCompany(t, "Deal company", &e.Rep1)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Renewal", pipeline, open, &e.Rep1)
	anchorOnCompany(t, e, deal, companyA)
	agreement := seedContract(t, e, contracts.CreateContractInput{CompanyID: companyIDOf(companyA), DealID: dealIDPtr(deal), Title: "Agreement", ValueBasis: contracts.BasisTotal, Source: "manual"})
	e.WsExec(t, `UPDATE company SET visibility='owner' WHERE id=$1`, companyA)
	e.WsExec(t, `UPDATE deal SET company_id=$1 WHERE id=$2`, companyB, deal)
	store := e.Activities.WithBlobstore(blobstore.NewMemory())
	reader := e.As(e.Rep1, nil, AdminPerms)
	contractID := agreement.UUID
	upload := func() error {
		_, err := store.UploadAttachment(reader, activities.AttachmentInput{EntityType: "deal", EntityID: deal, Filename: "agreement.pdf", ContentType: "application/pdf", Content: bytes.NewReader([]byte("%PDF-1.4 agreement")), ContractID: &contractID})
		return err
	}
	if err := upload(); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("filing revealed a hidden agreement: %v", err)
	}
	e.WsExec(t, `UPDATE company SET owner_id=$1 WHERE id=$2`, e.Rep1, companyA)
	if err := upload(); err != nil {
		t.Fatalf("filing by the agreement's company owner failed: %v", err)
	}
	if err := e.Contracts.ArchiveContract(reader, agreement); err != nil {
		t.Fatal(err)
	}
	if err := upload(); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("new filing against an archived agreement was accepted: %v", err)
	}
}

func TestACompanyOwnerCanRepairALegacyAgreementOnAnotherOwnersDeal(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	companyA := e.SeedCompany(t, "Agreement company", &e.Rep1)
	companyB := e.SeedCompany(t, "Deal company", &e.Rep3)
	deal := e.SeedDeal(t, "Renewal", pipeline, open, &e.Rep3)
	anchorOnCompany(t, e, deal, companyA)
	agreement := seedContract(t, e, contracts.CreateContractInput{CompanyID: companyIDOf(companyA), DealID: dealIDPtr(deal), Title: "Agreement", ValueBasis: contracts.BasisTotal, Source: "manual"})
	e.WsExec(t, `UPDATE deal SET company_id=$1 WHERE id=$2`, companyB, deal)
	perms := AccountRepPerms
	perms.Objects = maps.Clone(perms.Objects)
	grant := perms.Objects["contract"]
	grant.Update = true
	perms.Objects["contract"] = grant
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	if _, err := e.Contracts.UpdateContract(owner, agreement, crmcontracts.UpdateContractRequest{}, []string{"deal_id"}, nil); err != nil {
		t.Fatalf("the agreement company owner cannot detach an inconsistent deal they do not own: %v", err)
	}
	repaired, err := e.Contracts.GetContract(e.Admin(), agreement)
	if err != nil || repaired.DealId != nil {
		t.Fatalf("the company owner's repair left the old deal link: %v, %v", repaired.DealId, err)
	}
}
