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
	"testing"

	"github.com/jackc/pgx/v5"

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
