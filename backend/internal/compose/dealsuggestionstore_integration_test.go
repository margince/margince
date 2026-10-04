// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The deals store's suggestion entry points refuse what they must: a writer
// that is not the system, a draft that breaks the suggestion's shape, a reader
// without the grants, an acceptance with nothing wired to file its evidence.
// And an acceptance settles only the signals its reader could have settled.

import (
	"context"
	"errors"
	"maps"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// record runs RecordSuggestionTx as the principal in ctx.
func (e *scoutEnv) record(ctx context.Context, t *testing.T, draft deals.SuggestionDraft) (bool, error) {
	t.Helper()
	var raised bool
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		raised, err = deals.RecordSuggestionTx(ctx, tx, draft)
		return err
	})
	return raised, err
}

func TestOnlyTheSystemRecordsAWellFormedSuggestion(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	meeting := e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	evidence := []deals.SuggestionEvidence{{Kind: deals.EvidenceMeeting, ActivityID: &meeting, OccurredAt: e.daysAgo(3)}}
	good := deals.SuggestionDraft{CompanyID: acme, NameHint: deals.HintMeetingHeld, Confidence: 0.6, Evidence: evidence}

	if _, err := e.record(e.Admin(), t, good); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an admin recording a suggestion = %v, want permission denied", err)
	}
	amount, noOne := int64(100), ids.Nil
	for name, broken := range map[string]func(d *deals.SuggestionDraft){
		"no company":           func(d *deals.SuggestionDraft) { d.CompanyID = ids.Nil },
		"an unknown hint":      func(d *deals.SuggestionDraft) { d.NameHint = "found in a mail" },
		"no evidence":          func(d *deals.SuggestionDraft) { d.Evidence = nil },
		"an amount alone":      func(d *deals.SuggestionDraft) { d.AmountMinor = &amount },
		"a confidence above 1": func(d *deals.SuggestionDraft) { d.Confidence = 1.5 },
		"evidence naming nothing": func(d *deals.SuggestionDraft) {
			d.Evidence = []deals.SuggestionEvidence{{Kind: deals.EvidenceMeeting, ActivityID: &noOne, OccurredAt: e.now}}
		},
	} {
		draft := good
		broken(&draft)
		if _, err := e.record(e.system(), t, draft); !errors.Is(err, deals.ErrSuggestionDraftInvalid) {
			t.Errorf("a draft with %s = %v, want it refused as malformed", name, err)
		}
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := deals.SupersedeStaleSuggestionsTx(e.Admin(), tx)
		return err
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an admin running the superseding pass = %v, want permission denied", err)
	}
	if raised, err := e.record(e.system(), t, good); err != nil || !raised {
		t.Fatalf("the system recording the good draft = %v, %v; want it raised", raised, err)
	}
}

func TestEvidenceOlderThanADismissalIsRefusedByTheWriter(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	old := e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	e.pass(t)
	if _, err := e.decider().DismissSuggestion(e.Admin(), e.onlySuggestion(e.Admin(), t, acme).ID); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	// Written straight to the writer, past the scout's own filter: the writer
	// holds the floor itself.
	raised, err := e.record(e.system(), t, deals.SuggestionDraft{
		CompanyID: acme, NameHint: deals.HintMeetingHeld, Confidence: 0.6,
		Evidence: []deals.SuggestionEvidence{
			{Kind: deals.EvidenceMeeting, ActivityID: &old, OccurredAt: e.daysAgo(3)},
		},
	})
	if err != nil || raised {
		t.Fatalf("recording evidence older than the dismissal = %v, %v; want nothing written", raised, err)
	}
}

func TestAReaderWithoutTheGrantsReadsNoSuggestion(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	e.pass(t)
	suggestion := e.onlySuggestion(e.Admin(), t, acme)

	noCompany := e.withoutGrant(e.Rep1, e.Team1, "company")
	if _, _, err := e.Deals.ListSuggestions(noCompany, deals.SuggestionQuery{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("listing without company read = %v, want permission denied", err)
	}
	noDeal := e.withoutGrant(e.Rep1, e.Team1, "deal")
	if _, err := e.Deals.CountOpenSuggestions(noDeal); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("counting without deal read = %v, want permission denied", err)
	}
	if _, err := e.Deals.GetSuggestion(noDeal, suggestion.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("reading one without deal read = %v, want permission denied", err)
	}
	if _, err := e.decider().AcceptSuggestion(noDeal, suggestion.ID, deals.AcceptSuggestionInput{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("accepting without deal create = %v, want permission denied", err)
	}
	if _, err := e.decider().DismissSuggestion(noDeal, suggestion.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("dismissing without deal create = %v, want permission denied", err)
	}
	if got, err := e.Deals.GetSuggestion(e.rep(e.Rep1, e.Team1), suggestion.ID); err != nil || got.ID != suggestion.ID {
		t.Errorf("a rep reading it = %v, %v; want the suggestion", got.ID, err)
	}
	if _, err := e.Deals.AcceptSuggestion(e.Admin(), suggestion.ID, deals.AcceptSuggestionInput{}); err == nil {
		t.Error("a store with no acceptance effects wired opened a deal")
	}
}

func TestAnAcceptanceSettlesOnlyTheSignalsItsReaderCouldSettle(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	mail := e.email(t, "Next steps", "inbound", acme, e.daysAgo(6))
	opportunity := e.signal(t, "new_opportunity", acme, mail, e.daysAgo(5), ids.Nil)
	e.signal(t, "commitment_made", acme, mail, e.daysAgo(2), ids.Nil)
	e.pass(t)
	readsSignals := scoutRepPerms()
	readsSignals.Objects = maps.Clone(readsSignals.Objects)
	readsSignals.Objects["signal"] = principal.ObjectGrant{Read: true}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, readsSignals)

	owner := ids.From[ids.UserKind](e.Rep2).UUID
	out, err := e.decider().AcceptSuggestion(rep, e.onlySuggestion(rep, t, acme).ID, deals.AcceptSuggestionInput{OwnerID: &owner})
	if err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if out.Acknowledged != 0 {
		t.Fatalf("a rep who may not update signals acknowledged %d", out.Acknowledged)
	}
	if status := e.WsScalar(t, `SELECT status FROM signal WHERE id = $1`, opportunity); status != "open" {
		t.Fatalf("the signal is %q, want it left open for somebody who may settle it", status)
	}
	deal, err := e.Deals.GetDeal(e.Admin(), ids.From[ids.DealKind](out.DealID), storekit.LiveOnly)
	if err != nil || deal.OwnerId == nil || ids.UUID(*deal.OwnerId) != e.Rep2 {
		t.Fatalf("deal owner = %v (err %v), want the teammate the rep named", deal.OwnerId, err)
	}
}

// twinsOf reads the companies an open duplicate pair joins to company, as the
// scout does before it records.
func (e *scoutEnv) twinsOf(t *testing.T, company ids.UUID) []ids.UUID {
	t.Helper()
	ctx := e.system()
	var twins []ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		twins, err = contacts.OpenDuplicateCompaniesTx(ctx, tx, company)
		return err
	}); err != nil {
		t.Fatalf("reading the duplicate pair: %v", err)
	}
	return twins
}

// meetingDraft is one meeting held company, as the scout drafts it.
func (e *scoutEnv) meetingDraft(t *testing.T, company, meeting ids.UUID) deals.SuggestionDraft {
	return deals.SuggestionDraft{
		CompanyID: company, NameHint: deals.HintMeetingHeld, Confidence: 0.6,
		Evidence:    []deals.SuggestionEvidence{{Kind: deals.EvidenceMeeting, ActivityID: &meeting, OccurredAt: e.daysAgo(3)}},
		DuplicateOf: e.twinsOf(t, company),
	}
}

// One meeting is evidence for one opportunity. A business filed twice and
// paired as a possible duplicate gets one suggestion from it, not two.
func TestOneMeetingRaisesOneSuggestionAcrossDuplicateCompanies(t *testing.T) {
	e := setupScout(t)
	named := e.SeedCompany(t, "Northwind Traders", nil)
	twin := e.SeedCompany(t, "NORTHWIND TRADERS", nil)
	if twins := e.twinsOf(t, twin); len(twins) != 1 || twins[0] != named {
		t.Fatalf("the second create left duplicate pair %v, want it paired with %v", twins, named)
	}
	dana := e.employee(t, "Dana Buyer", named)
	meeting := e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))

	if first, err := e.record(e.system(), t, e.meetingDraft(t, named, meeting)); err != nil || !first {
		t.Fatalf("the first suggestion = %v, %v; want it raised", first, err)
	}
	if second, err := e.record(e.system(), t, e.meetingDraft(t, twin, meeting)); err != nil || second {
		t.Fatalf("the same meeting raised a second suggestion on the duplicate company = %v, %v", second, err)
	}
}

// Two businesses at one meeting are two opportunities. Shared evidence alone
// does not make them one company.
func TestOneMeetingWithTwoDistinctCompaniesRaisesASuggestionForEach(t *testing.T) {
	e := setupScout(t)
	buyer := e.SeedCompany(t, "Northwind Traders", nil)
	partner := e.SeedCompany(t, "Contoso Logistics", nil)
	dana := e.employee(t, "Dana Buyer", buyer)
	meeting := e.meeting(e.Admin(), t, "Joint scoping workshop", &dana, e.daysAgo(3))

	for _, company := range []ids.UUID{buyer, partner} {
		if raised, err := e.record(e.system(), t, e.meetingDraft(t, company, meeting)); err != nil || !raised {
			t.Fatalf("the suggestion for %v = %v, %v; want each business raised", company, raised, err)
		}
	}
	bought, partnered := e.onlySuggestion(e.Admin(), t, buyer).ID, e.onlySuggestion(e.Admin(), t, partner).ID
	// A duplicate pair elsewhere in the workspace, so the pass has pairs to
	// withdraw against and must tell this one apart from them.
	e.SeedCompany(t, "Fabrikam Inc", nil)
	e.SeedCompany(t, "FABRIKAM INC", nil)

	e.pass(t)
	for _, id := range []ids.UUID{bought, partnered} {
		if state := e.suggestionState(t, id); state != deals.SuggestionOpen {
			t.Errorf("suggestion %v is %q after a scout pass, want each business's left open", id, state)
		}
	}
}

// suggestionState reads one suggestion's lifecycle state, whoever may see it.
func (e *scoutEnv) suggestionState(t *testing.T, id ids.UUID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT state FROM deal_suggestion WHERE id = $1`, id)
}

// recordUnpaired records a suggestion as a scout that had not yet seen the
// duplicate pair would, and answers its id.
func (e *scoutEnv) recordUnpaired(t *testing.T, company, meeting ids.UUID) ids.UUID {
	t.Helper()
	draft := e.meetingDraft(t, company, meeting)
	draft.DuplicateOf = nil
	if raised, err := e.record(e.system(), t, draft); err != nil || !raised {
		t.Fatalf("recording the suggestion on %v = %v, %v; want it raised", company, raised, err)
	}
	return e.onlySuggestion(e.Admin(), t, company).ID
}

// A business filed twice and offered one meeting twice, before the pair was
// known, is left with the first suggestion after one scout pass.
func TestAScoutPassWithdrawsTheNewerSuggestionRaisedTwiceForOneBusiness(t *testing.T) {
	e := setupScout(t)
	named := e.SeedCompany(t, "Northwind Traders", nil)
	twin := e.SeedCompany(t, "NORTHWIND TRADERS", nil)
	dana := e.employee(t, "Dana Buyer", named)
	meeting := e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	older := e.recordUnpaired(t, named, meeting)
	newer := e.recordUnpaired(t, twin, meeting)

	e.pass(t)

	if state := e.suggestionState(t, older); state != deals.SuggestionOpen {
		t.Errorf("the first suggestion is %q, want it left open", state)
	}
	if state := e.suggestionState(t, newer); state != deals.SuggestionSuperseded {
		t.Fatalf("the second suggestion on the duplicate is %q, want it superseded", state)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_suggestion' AND entity_id = $1
		AND action = 'update' AND after->>'state' = 'superseded'`, newer); n != 1 {
		t.Fatalf("%d audit rows record the withdrawal, want 1", n)
	}
}

// Two meetings are two pieces of evidence: a duplicate pair offered one each
// keeps both until a human merges the records.
func TestDuplicateCompaniesCitingDifferentMeetingsKeepBothSuggestions(t *testing.T) {
	e := setupScout(t)
	named := e.SeedCompany(t, "Northwind Traders", nil)
	twin := e.SeedCompany(t, "NORTHWIND TRADERS", nil)
	dana := e.employee(t, "Dana Buyer", named)
	first := e.recordUnpaired(t, named, e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3)))
	second := e.recordUnpaired(t, twin, e.meeting(e.Admin(), t, "Pricing call", &dana, e.daysAgo(2)))

	e.pass(t)

	for _, id := range []ids.UUID{first, second} {
		if state := e.suggestionState(t, id); state != deals.SuggestionOpen {
			t.Errorf("suggestion %v is %q, want it left open: it cites its own meeting", id, state)
		}
	}
}

// A user who dismissed the suggestion on one record dismissed the business:
// its duplicate is not offered the same meeting again.
func TestADismissedSuggestionIsNotReofferedOnTheDuplicateCompany(t *testing.T) {
	e := setupScout(t)
	named := e.SeedCompany(t, "Northwind Traders", nil)
	twin := e.SeedCompany(t, "NORTHWIND TRADERS", nil)
	dana := e.employee(t, "Dana Buyer", named)
	meeting := e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	if raised, err := e.record(e.system(), t, e.meetingDraft(t, named, meeting)); err != nil || !raised {
		t.Fatalf("the first suggestion = %v, %v; want it raised", raised, err)
	}
	if _, err := e.decider().DismissSuggestion(e.Admin(), e.onlySuggestion(e.Admin(), t, named).ID); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	if again, err := e.record(e.system(), t, e.meetingDraft(t, twin, meeting)); err != nil || again {
		t.Fatalf("the dismissed meeting was offered again on the duplicate = %v, %v", again, err)
	}
}

// A user who turned the meeting into a deal on one record has the opportunity:
// its duplicate is not offered the same meeting as a second one.
func TestAnAcceptedSuggestionIsNotReofferedOnTheDuplicateCompany(t *testing.T) {
	e := setupScout(t)
	named := e.SeedCompany(t, "Northwind Traders", nil)
	twin := e.SeedCompany(t, "NORTHWIND TRADERS", nil)
	dana := e.employee(t, "Dana Buyer", named)
	meeting := e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	if raised, err := e.record(e.system(), t, e.meetingDraft(t, named, meeting)); err != nil || !raised {
		t.Fatalf("the first suggestion = %v, %v; want it raised", raised, err)
	}
	accepted := e.onlySuggestion(e.Admin(), t, named).ID
	if _, err := e.decider().AcceptSuggestion(e.Admin(), accepted, deals.AcceptSuggestionInput{NoAmount: true}); err != nil {
		t.Fatalf("accepting: %v", err)
	}

	if again, err := e.record(e.system(), t, e.meetingDraft(t, twin, meeting)); err != nil || again {
		t.Fatalf("the accepted meeting was offered again on the duplicate = %v, %v", again, err)
	}
}

// Archiving one end closes a duplicate pair, as the dedupe queue reads it, so
// the live company is no longer read as anybody's duplicate.
func TestAnArchivedTwinNoLongerPairsTheLiveCompany(t *testing.T) {
	e := setupScout(t)
	named := e.SeedCompany(t, "Northwind Traders", nil)
	twin := e.SeedCompany(t, "NORTHWIND TRADERS", nil)
	if _, err := e.Contacts.ArchiveCompany(e.Admin(), ids.From[ids.CompanyKind](named), nil); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	if twins := e.twinsOf(t, twin); len(twins) != 0 {
		t.Fatalf("the live company is still paired with %v, want the archived end closed", twins)
	}
}

// Two passes recording a duplicate pair at once: the second waits on the
// first's evidence lock, then finds its suggestion and writes nothing.
func TestTwinsRecordedAtOnceRaiseOneSuggestion(t *testing.T) {
	e := setupScout(t)
	named := e.SeedCompany(t, "Northwind Traders", nil)
	twin := e.SeedCompany(t, "NORTHWIND TRADERS", nil)
	dana := e.employee(t, "Dana Buyer", named)
	meeting := e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	namedDraft, twinDraft := e.meetingDraft(t, named, meeting), e.meetingDraft(t, twin, meeting)
	ctx := e.system()

	firstPID, release, firstDone := make(chan int, 1), make(chan struct{}), make(chan error, 1)
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFirst)
	go func() {
		firstDone <- database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			if _, err := deals.RecordSuggestionTx(ctx, tx, namedDraft); err != nil {
				return err
			}
			var pid int
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			firstPID <- pid
			<-release
			return nil
		})
	}()
	var pid int
	select {
	case pid = <-firstPID:
	case err := <-firstDone:
		t.Fatalf("the first pass ended before it held its lock: %v", err)
	}

	var secondRaised bool
	secondDone := make(chan error, 1)
	go func() {
		var err error
		secondRaised, err = e.record(ctx, t, twinDraft)
		secondDone <- err
	}()
	probe, err := e.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := probe.Rollback(context.Background()); err != nil {
			t.Errorf("releasing the probe: %v", err)
		}
	}()
	waitForBackendBlockedBy(t, probe, pid, secondDone)
	releaseFirst()

	if err := <-firstDone; err != nil {
		t.Fatalf("the first pass: %v", err)
	}
	if err := <-secondDone; err != nil || secondRaised {
		t.Fatalf("the twin recorded at the same time raised a second suggestion = %v, %v", secondRaised, err)
	}
}

// The scout reads the open pairs and acts on them later in its transaction. A
// reviewer answering "not a duplicate" meanwhile waits for it, so no suggestion
// is withdrawn on a pair a human has just said is two businesses.
func TestAReviewerAnsweringAPairWaitsForTheScoutActingOnIt(t *testing.T) {
	e := setupScout(t)
	named := e.SeedCompany(t, "Northwind Traders", nil)
	e.SeedCompany(t, "NORTHWIND TRADERS", nil)
	pair, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM dedupe_candidate
		WHERE left_company_id = $1 OR right_company_id = $1`, named))
	if err != nil {
		t.Fatalf("reading the pair: %v", err)
	}
	ctx := e.system()

	scoutPID, release, scoutDone := make(chan int, 1), make(chan struct{}), make(chan error, 1)
	var releaseOnce sync.Once
	releaseScout := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseScout)
	go func() {
		scoutDone <- database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			if _, err := contacts.OpenDuplicateCompanyPairsTx(ctx, tx); err != nil {
				return err
			}
			var pid int
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			scoutPID <- pid
			<-release
			return nil
		})
	}()
	var pid int
	select {
	case pid = <-scoutPID:
	case err := <-scoutDone:
		t.Fatalf("the scout ended before it held the pairs: %v", err)
	}

	reviewDone := make(chan error, 1)
	go func() {
		_, err := e.Contacts.DisposeDedupeCandidate(e.Admin(), pair, "not_a_duplicate", nil)
		reviewDone <- err
	}()
	probe, err := e.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := probe.Rollback(context.Background()); err != nil {
			t.Errorf("releasing the probe: %v", err)
		}
	}()
	waitForBackendBlockedBy(t, probe, pid, reviewDone)
	releaseScout()

	if err := <-scoutDone; err != nil {
		t.Fatalf("the scout: %v", err)
	}
	if err := <-reviewDone; err != nil {
		t.Fatalf("the review, once the scout let go: %v", err)
	}
}
