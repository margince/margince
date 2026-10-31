// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// answeringRepCtx is a rep who may answer findings on the deals they own.
func (e *forecastEnv) answeringRepCtx(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal":                  {Read: true, Update: true},
				"forecast":              {Read: true, Update: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

func (e *forecastEnv) seedFinding(t *testing.T, kind string, deal ids.UUID, amount int64) ids.UUID {
	t.Helper()
	return e.seedID(t, `INSERT INTO assurance_exception (id, logical_key, type, subject_kind, subject_id, severity,
		affected_minor, currency, captured_by) VALUES ($1, $2, $3, 'deal', $4, 'high', $5, 'EUR', 'test')`,
		kind+":"+deal.String(), kind, deal, amount)
}

func (e *forecastEnv) listInputChecks(ctx context.Context, t *testing.T) assurance.InputCheckList {
	t.Helper()
	h := assurance.NewHandlers(assurance.NewStore(InstallationDB(e.Pool)), AssuranceExceptionsIn,
		func() time.Time { return clocktest.Now(t) })
	rec := httptest.NewRecorder()
	h.ListInputChecks(rec, httptest.NewRequest(http.MethodGet, "/forecast/assurance/exceptions", nil).WithContext(ctx),
		crmcontracts.ListInputChecksParams{})
	if rec.Code != http.StatusOK {
		t.Fatalf("the list answered %d: %s", rec.Code, rec.Body)
	}
	var out assurance.InputCheckList
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func modesByFinding(list assurance.InputCheckList) map[string]crmcontracts.InputCheckDestinationMode {
	out := map[string]crmcontracts.InputCheckDestinationMode{}
	for _, check := range list.Data {
		if check.Destination != nil {
			out[check.Id.String()] = check.Destination.Mode
		}
	}
	return out
}

// A finding opens for answering only for a reader who may answer it, so a
// read-only reviewer is never offered an answer the server refuses.
func TestEachFindingSaysWhetherItsReaderMayAnswerIt(t *testing.T) {
	e := setupForecast(t)
	amount := int64(4_800_000)
	mine := e.seedOpenDeal(t, "Harbourline depot retrofit", 20, &e.Rep1, &amount, nil)
	theirs := e.seedOpenDeal(t, "Marlow workshop suite", 20, &e.Rep3, &amount, nil)
	minePast := e.seedFinding(t, "close_past", mine, amount)
	theirsPast := e.seedFinding(t, "close_past", theirs, amount)

	rep := modesByFinding(e.listInputChecks(e.answeringRepCtx(e.Rep1), t))
	if rep[minePast.String()] != crmcontracts.InputCheckDestinationModeAnswer {
		t.Errorf("the owner's own finding opens as %q, want answer", rep[minePast.String()])
	}
	if rep[theirsPast.String()] != crmcontracts.InputCheckDestinationModeRead {
		t.Errorf("a colleague's finding opens as %q for the rep, want read", rep[theirsPast.String()])
	}
	reviewer := modesByFinding(e.listInputChecks(e.forecastReader(e.Admin()), t))
	for _, id := range []ids.UUID{minePast, theirsPast} {
		if reviewer[id.String()] != crmcontracts.InputCheckDestinationModeRead {
			t.Errorf("a read-only reviewer is offered %q on %s, want read", reviewer[id.String()], id)
		}
	}
}

// Groups count the whole list the caller may see: a deal with two findings is
// one deal, and its money is counted once per group.
func TestGroupsCountFindingsAndDistinctDealsOverTheWholeList(t *testing.T) {
	e := setupForecast(t)
	harbour, marlow := int64(4_800_000), int64(2_900_000)
	first := e.seedOpenDeal(t, "Harbourline depot retrofit", 20, &e.Rep1, &harbour, nil)
	second := e.seedOpenDeal(t, "Marlow workshop suite", 20, &e.Rep3, &marlow, nil)
	e.seedFinding(t, "close_past", first, harbour)
	e.seedFinding(t, "no_next_step", first, harbour)
	e.seedFinding(t, "close_past", second, marlow)

	list := e.listInputChecks(e.answeringRepCtx(e.Rep1), t)

	if list.Totals != (crmcontracts.InputCheckTotals{Findings: 3, Deals: 2}) {
		t.Errorf("totals are %+v, want 3 findings on 2 deals", list.Totals)
	}
	byType := map[string]crmcontracts.InputCheckGroup{}
	for _, group := range list.Groups {
		byType[group.Type] = group
	}
	past := byType["close_past"]
	if past.Findings != 2 || past.Deals != 2 || len(past.Exposure) != 1 || past.Exposure[0].AmountMinor != harbour+marlow {
		t.Errorf("close_past groups as %+v, want 2 findings on 2 deals at %d EUR", past, harbour+marlow)
	}
	if past.Destination.Mode != crmcontracts.InputCheckDestinationModeAnswer {
		t.Errorf("a group holding one answerable finding opens as %q, want answer", past.Destination.Mode)
	}
	if step := byType["no_next_step"]; step.Findings != 1 || step.Deals != 1 {
		t.Errorf("no_next_step groups as %+v, want 1 finding on 1 deal", step)
	}
}
