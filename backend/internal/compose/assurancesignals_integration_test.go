// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestUnlinkedSignalFindingSurvivesReplayAndHonorsAResolution(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Maple Studio", nil)
	source := e.email(t, "A new project", "inbound", company, e.daysAgo(2))
	signal := e.signal(t, "new_opportunity", company, source, e.now, ids.Nil)
	store := assurance.NewStore(InstallationDB(e.Pool))
	scanner := assurance.NewScanner(store, AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig()).WithSignalSubjects(AssuranceSignalSubjects)
	for range 2 {
		result, err := scanner.Scan(e.system(), e.now, nil)
		if err != nil {
			t.Fatal(err)
		}
		if result.EligibleSignals != 1 {
			t.Fatalf("eligible signals = %d, want 1", result.EligibleSignals)
		}
	}
	found := readSignalFindings(e.signalReader(), t, e)
	if len(found) != 1 || found[0].SubjectID != signal {
		t.Fatalf("findings = %+v, want one signal finding", found)
	}
	if err := store.Resolve(e.signalReader(), found[0].ID, assurance.Resolution{Outcome: assurance.OutcomeAddedEvidence, Reason: "This request was already handled"}, e.now); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.Scan(e.system(), e.now, nil); err != nil {
		t.Fatal(err)
	}
	if got := readSignalFindings(e.signalReader(), t, e); len(got) != 0 {
		t.Fatalf("resolved evidence returned: %+v", got)
	}
}

func readSignalFindings(ctx context.Context, t *testing.T, e *scoutEnv) []assurance.Exception {
	t.Helper()
	var found []assurance.Exception
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		found, err = AssuranceExceptions(ctx, tx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	filtered := make([]assurance.Exception, 0, len(found))
	for _, finding := range found {
		if finding.SubjectKind == "signal" {
			filtered = append(filtered, finding)
		}
	}
	return filtered
}

func TestLinkingOneBuyingSignalLeavesAnotherRequestAtTheCompanyOpen(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Maple Studio", nil)
	source, buyer := e.buyingEmail(t, company, e.daysAgo(2))
	first := e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, buyer)
	otherSource, otherBuyer := e.buyingEmail(t, company, e.daysAgo(1))
	second := e.signal(t, "new_opportunity", company, otherSource, e.now, ids.Nil, otherBuyer)
	store := assurance.NewStore(InstallationDB(e.Pool)).WithSignalDecision(DecideAssuranceSignal)
	scanner := assurance.NewScanner(store, AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig()).WithSignalSubjects(AssuranceSignalSubjects)
	if _, err := scanner.Scan(e.system(), e.now, nil); err != nil {
		t.Fatal(err)
	}
	found := readSignalFindings(e.signalReader(), t, e)
	if len(found) != 2 {
		t.Fatalf("distinct requests = %d, want 2", len(found))
	}
	companyID := ids.From[ids.CompanyKind](company)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Maple rollout", CompanyID: &companyID, PipelineID: e.pipeline, StageID: e.open, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	dealID := ids.UUID(deal.Id)
	for _, finding := range found {
		if finding.SubjectID == first {
			if err := store.Resolve(e.signalReader(), finding.ID, assurance.Resolution{Outcome: assurance.OutcomeFixedRecord, DealID: &dealID, Reason: "The rollout covers this request"}, e.now); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := scanner.Scan(e.system(), e.now, nil); err != nil {
		t.Fatal(err)
	}
	remaining := readSignalFindings(e.signalReader(), t, e)
	if len(remaining) != 1 || remaining[0].SubjectID != second {
		t.Fatalf("remaining = %+v, want only the other request", remaining)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, source, dealID); got != 1 {
		t.Fatalf("source links = %d, want 1", got)
	}
}

func TestSignalAnswerDoesNotRequireDecidingAProposalTheReaderCannotSee(t *testing.T) {
	e := setupScout(t)
	company := e.SeedCompany(t, "Maple Studio", nil)
	e.buyerPair(t, company, e.daysAgo(2))
	e.pass(t)
	store := assurance.NewStore(InstallationDB(e.Pool)).WithSignalDecision(DecideAssuranceSignal)
	scanner := assurance.NewScanner(store, AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig()).WithSignalSubjects(AssuranceSignalSubjects)
	if _, err := scanner.Scan(e.system(), e.now, nil); err != nil {
		t.Fatal(err)
	}
	perms := integration.AdminWithSignals
	perms.Objects = maps.Clone(perms.Objects)
	delete(perms.Objects, "deal")
	reader := e.As(e.AdminUser, nil, perms)
	found := readSignalFindings(reader, t, e)
	if len(found) != 1 {
		t.Fatalf("findings=%d, want one", len(found))
	}
	var observed map[string]json.RawMessage
	if err := json.Unmarshal(found[0].Observed, &observed); err != nil {
		t.Fatal(err)
	}
	if _, shown := observed["suggestion_id"]; shown {
		t.Fatal("hidden proposal reference disclosed")
	}
	denied := perms
	denied.Objects = maps.Clone(perms.Objects)
	signalGrant := denied.Objects["signal"]
	signalGrant.Update = false
	denied.Objects["signal"] = signalGrant
	// An admin may also answer a forecast call. Without that grant too, the
	// reader holds no grant to answer any finding and is refused outright.
	callGrant := denied.Objects["forecast"]
	callGrant.Create = false
	denied.Objects["forecast"] = callGrant
	for _, id := range []ids.UUID{found[0].ID, ids.NewV7()} {
		err := store.Resolve(e.As(e.AdminUser, nil, denied), id, assurance.Resolution{Outcome: assurance.OutcomeNotRelevant, Reason: "No current buyer need"}, e.now)
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("ungranted resolution=%v", err)
		}
	}
	if err := store.Resolve(reader, found[0].ID, assurance.Resolution{Outcome: assurance.OutcomeNotRelevant, Reason: "No current buyer need"}, e.now); err != nil {
		t.Fatal(err)
	}
	e.pass(t)
	if count := e.stored(t, company, deals.SuggestionOpen); count != 0 {
		t.Fatalf("unsupported proposals=%d, want none", count)
	}
	if _, err := scanner.Scan(e.system(), e.now, nil); err != nil {
		t.Fatal(err)
	}
	if remaining := readSignalFindings(reader, t, e); len(remaining) != 0 {
		t.Fatalf("dismissed evidence returned: %+v", remaining)
	}
}

func TestFilingASignalRequiresPermissionToLinkAllOfItsEvidence(t *testing.T) {
	e := setupScout(t)
	company := e.SeedCompany(t, "Maple Studio", nil)
	source, buyer := e.buyingEmail(t, company, e.daysAgo(2))
	signal := e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, buyer)
	store := assurance.NewStore(InstallationDB(e.Pool)).WithSignalDecision(DecideAssuranceSignal)
	scanner := assurance.NewScanner(store, AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig()).WithSignalSubjects(AssuranceSignalSubjects)
	if _, err := scanner.Scan(e.system(), e.now, nil); err != nil {
		t.Fatal(err)
	}
	found := readSignalFindings(e.signalReader(), t, e)
	if len(found) != 1 {
		t.Fatalf("findings=%d, want one", len(found))
	}
	companyID := ids.From[ids.CompanyKind](company)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Maple rollout", CompanyID: &companyID, PipelineID: e.pipeline, StageID: e.open, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	perms := integration.AdminWithSignals
	perms.Objects = maps.Clone(perms.Objects)
	activityGrant := perms.Objects["activity"]
	activityGrant.Update = false
	perms.Objects["activity"] = activityGrant
	reader := e.As(e.AdminUser, nil, perms)
	dealID := ids.UUID(deal.Id)
	err = store.Resolve(reader, found[0].ID, assurance.Resolution{Outcome: assurance.OutcomeFixedRecord, DealID: &dealID}, e.now)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("filing without source update permission=%v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, source, dealID); got != 0 {
		t.Fatalf("refused source links=%d", got)
	}
	if remaining := readSignalFindings(reader, t, e); len(remaining) != 1 || remaining[0].SubjectID != signal {
		t.Fatalf("refused finding must remain actionable: %+v", remaining)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE id = $1 AND status = 'open'`, signal); got != 1 {
		t.Fatal("refused filing settled its signal")
	}
}

func TestAnUnreadableSignalCensusCannotClearFindingsOrClaimACompletePreview(t *testing.T) {
	e := setupScout(t)
	company := e.SeedCompany(t, "Maple Studio", nil)
	source, buyer := e.buyingEmail(t, company, e.daysAgo(2))
	e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, buyer)
	store := assurance.NewStore(InstallationDB(e.Pool))
	scanner := assurance.NewScanner(store, AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig()).WithSignalSubjects(AssuranceSignalSubjects)
	if _, err := scanner.Scan(e.system(), e.now, nil); err != nil {
		t.Fatal(err)
	}
	for _, object := range []string{"company", "signal", "activity"} {
		t.Run(object, func(t *testing.T) {
			perms := integration.AdminWithSignals
			perms.Objects = maps.Clone(perms.Objects)
			delete(perms.Objects, object)
			reader := e.As(e.AdminUser, nil, perms)
			preview, err := scanner.Preview(reader, e.now)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Counts) != 0 || preview.Readiness != assurance.ReadinessChecksIncomplete {
				t.Fatalf("unreadable preview=%+v", preview)
			}
			run, err := scanner.Scan(reader, e.now, nil)
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != assurance.StatusIncomplete || run.Cleared != 0 {
				t.Fatalf("unreadable run=%+v", run)
			}
			if found := readSignalFindings(e.signalReader(), t, e); len(found) != 1 {
				t.Fatalf("incomplete census changed findings: %+v", found)
			}
		})
	}
}
