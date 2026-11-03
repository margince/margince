// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestLifecyclePreviewResolvesItsActualSourceForTheReader(t *testing.T) {
	now := clocktest.Now(t)
	sourceAt := now.Add(-time.Minute)
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, nil, integration.AdminWithSignals)
	company, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Harbor account", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.Contacts.UpdateCompany(ctx, id, contacts.UpdateCompanyInput{Lifecycle: new("customer")}); err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "meeting", Subject: new("Contract review"), Body: new("The buyer ended the agreement."), OccurredAt: &sourceAt,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	proof, ok := values.ProveQuote(*activity.Body, "The buyer ended the agreement.")
	if !ok {
		t.Fatal("source quote did not match")
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{Kind: signalKindContractEnded, CompanyID: id.UUID, Summary: "The buyer ended the agreement.", Severity: "warn", Fingerprint: "harbor-ended", Evidence: []signals.DerivedEvidence{{ActivityID: ids.UUID(activity.Id), Snippet: "The buyer ended the agreement.", SourceQuote: &proof, LifecycleScope: "company"}}}, sourceAt)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	proposer := NewSignalProposer(e.Pool, svc, slog.New(slog.DiscardHandler))
	proposer.now = func() time.Time { return now }
	if count, err := proposer.RunWorkspace(ctx); err != nil || count != 1 {
		t.Fatalf("proposals=%d, %v", count, err)
	}
	approvalID := ids.From[ids.ApprovalKind](stagedOffer(t, e, id.UUID))
	proposal, err := svc.GetWire(ctx, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.Evidence == nil || len(*proposal.Preview.Evidence) != 2 {
		t.Fatalf("source preview=%+v", proposal.Preview)
	}
	evidence := (*proposal.Preview.Evidence)[1]
	source := (*proposal.Preview.Evidence)[0]
	if source.SourceType == nil || *source.SourceType != "activity" || source.SourceId == nil || *source.SourceId != activity.Id {
		t.Fatalf("activity citation: %+v", source)
	}
	if evidence.EvidenceSnippet != "The buyer ended the agreement." || evidence.SourceId == nil {
		t.Fatalf("source=%+v", evidence)
	}
	restricted := e.As(e.Rep1, nil, integration.AdminPerms)
	if _, err := svc.GetWire(restricted, approvalID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("source-scoped approval: %v", err)
	}
	withheld, err := newApprovalPreviewReader(e.DB()).read(restricted, proposal)
	if err != nil || withheld == nil || withheld.State != "unavailable" || withheld.Evidence == nil || len(*withheld.Evidence) != 0 {
		t.Fatalf("without source permission: %+v, %v", withheld, err)
	}
	if _, err := e.Activities.ArchiveActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(activity.Id)), nil); err != nil {
		t.Fatal(err)
	}
	missingOriginal, err := newApprovalPreviewReader(e.DB()).read(ctx, proposal)
	if err != nil || missingOriginal == nil || missingOriginal.State != "unavailable" || missingOriginal.Evidence == nil || len(*missingOriginal.Evidence) != 0 {
		t.Fatalf("archived original: %+v, %v", missingOriginal, err)
	}
	if _, err := signals.NewStore(e.DB(), nil).ArchiveSignal(ctx, ids.From[ids.SignalKind](ids.UUID(*evidence.SourceId))); err != nil {
		t.Fatal(err)
	}
	archived, err := newApprovalPreviewReader(e.DB()).read(ctx, proposal)
	if err != nil || archived == nil || archived.State != "unavailable" || archived.Evidence == nil || len(*archived.Evidence) != 0 {
		t.Fatalf("archived source: %+v, %v", archived, err)
	}
}

func TestOwnerPreviewRechecksTheQuotedActivityBeforeApproval(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	company := e.SeedCompany(t, "Harbor", &e.AdminUser)
	contact := e.SeedContact(t, "Dana", &e.AdminUser)
	source, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new("Account handover"), Body: new("Please clear the account owner."), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"company","id":%q,"fields":{"owner_id":null}}`, company)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(ctx, approvals.StageInput{Kind: "update_record", TargetType: "company", TargetID: company, ProposedChange: raw, DiffHash: hash, Evidence: []approvals.Evidence{{Snippet: "Please clear the account owner.", SourceType: "activity", SourceID: ids.UUID(source.Id)}}})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(ctx, id)
	if err != nil || proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.Evidence == nil || len(*proposal.Preview.Evidence) != 1 {
		t.Fatalf("current source: %+v, %v", proposal.Preview, err)
	}
	if _, err := e.Activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(source.Id)), activities.UpdateActivityInput{Body: new("Keep the current account owner.")}); err != nil {
		t.Fatal(err)
	}
	proposal, err = svc.GetWire(ctx, id)
	if err != nil || proposal.Preview == nil || proposal.Preview.State != "stale" || proposal.Preview.Evidence == nil || len(*proposal.Preview.Evidence) != 0 {
		t.Fatalf("changed source: %+v, %v", proposal.Preview, err)
	}
	if _, err := svc.Decide(ctx, id, true, nil); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("changed source decision: %v", err)
	}
}
