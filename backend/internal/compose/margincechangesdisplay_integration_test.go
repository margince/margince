// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the report of Margince's changes shows beyond the changes. That is the
// deals it leaves alone for want of an owner, the check that changed nothing,
// and how much value each change moved.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func (v *dealVerdictEnv) teamChanges(ctx context.Context, t *testing.T) crmcontracts.TeamMarginceChanges {
	t.Helper()
	w := httptest.NewRecorder()
	newMarginceChangeHandlers(v.Pool).GetTeamMarginceChanges(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx),
		crmcontracts.GetTeamMarginceChangesParams{})
	if w.Code != http.StatusOK {
		t.Fatalf("team changes = %d %s", w.Code, w.Body.String())
	}
	var out crmcontracts.TeamMarginceChanges
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (v *dealVerdictEnv) createDeal(t *testing.T, name string, owner *ids.UserID) ids.UUID {
	t.Helper()
	amount, currency := int64(1_000_000), "EUR"
	created, err := v.Deals.CreateDeal(v.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: v.pipeline, StageID: v.stage, OwnerID: owner, AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(created.Id)
}

// ownedBy is a deal owned by a new seat in the given state, the same night's
// candidate as the deal the env set up.
func (v *dealVerdictEnv) ownedBy(t *testing.T, name, status string, archived bool) ids.UUID {
	t.Helper()
	seat := ids.NewV7()
	address := seat.String() + "@seat.test"
	v.WsExec(t, `INSERT INTO app_user (id, email, display_name, email_domain_folded, status, archived_at)
		VALUES ($1, $2, $3, 'seat.test', $4, CASE WHEN $5 THEN now() END)`, seat, address, name+" owner", status, archived)
	live := ids.From[ids.UserKind](v.Rep3)
	deal := v.createDeal(t, name, &live)
	v.WsExec(t, `UPDATE deal SET owner_id = $2 WHERE id = $1`, deal, seat)
	return deal
}

func TestTheTeamListShowsEveryDealTheNightlyCheckSkipsForWantOfALiveOwner(t *testing.T) {
	v := setupDealVerdict(t)
	unowned := v.createDeal(t, "Nobody's deal", nil)
	v.WsExec(t, `UPDATE deal SET owner_id = NULL WHERE id = $1`, unowned)
	theirs := ids.From[ids.UserKind](v.Rep3)
	v.createDeal(t, "Another team's deal", &theirs)
	inactive := map[ids.UUID]string{
		v.ownedBy(t, "Invited", "invited", false):     "invited",
		v.ownedBy(t, "Suspended", "suspended", false): "suspended",
		v.ownedBy(t, "Archived", "active", true):      "archived",
	}
	v.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, v.Rep1)

	v.run(t)
	if v.lane.calls != 0 {
		t.Fatalf("model calls = %d, want none: no deal has a live owner the check could act for", v.lane.calls)
	}
	report := v.teamChanges(v.colleague(), t)
	got := map[ids.UUID]crmcontracts.TeamMarginceOwnerlessDeal{}
	for _, d := range report.Ownerless {
		got[ids.UUID(d.DealId)] = d
	}
	if len(got) != 5 {
		t.Fatalf("ownerless = %+v, want the unowned, departed, invited, suspended and archived owners' deals", report.Ownerless)
	}
	if d := got[unowned]; d.Reason != crmcontracts.TeamMarginceOwnerlessDealReasonNoOwner || d.OwnerId != nil {
		t.Errorf("unowned deal = %+v, want no_owner and no owner", d)
	}
	departed := got[v.deal]
	if departed.Reason != crmcontracts.TeamMarginceOwnerlessDealReasonDepartedOwner || departed.OwnerName == nil ||
		departed.ValueBaseMinor == nil || *departed.ValueBaseMinor != 4_000_000 {
		t.Errorf("departed owner's deal = %+v, want departed_owner, the owner's name and 40,000.00 EUR", departed)
	}
	for deal, state := range inactive {
		if d := got[deal]; d.Reason != crmcontracts.TeamMarginceOwnerlessDealReasonInactiveOwner || d.OwnerId == nil {
			t.Errorf("%s owner's deal = %+v, want inactive_owner naming the owner", state, d)
		}
	}

	ungranted := v.As(v.Rep2, []ids.UUID{v.Team1}, principal.Permissions{RowScope: principal.RowScopeAll})
	if found, _, err := v.Deals.OwnerlessDeals(ungranted); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("ownerless for a reader with no deal grant = %+v, %v; want refused", found, err)
	}
}

// lowConfidenceLane proposes an amount it is not sure of.
type lowConfidenceLane struct{ cite *string }

func (l lowConfidenceLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: fmt.Sprintf(`{"actions":[
		{"kind":"set_amount","date":"","amount":45000,"currency":"EUR","quoted_words":"45,000 EUR a year",
		 "probability":0,"cited_activity_ids":[%q],"reason":"The board agreed on the price","confidence":0.3}]}`, *l.cite)}, nil
}

func TestTheOwnerReadsWhyMarginceChangedNothingAndAColleagueDoesNot(t *testing.T) {
	v := setupDealVerdict(t)
	v.pass.brain = lowConfidenceLane{cite: &v.note}
	subject, body, at := "Pricing call", "They asked about volume pricing.", v.now
	call, _, err := v.Activities.LogActivity(v.Admin(), activities.LogActivityInput{
		Kind: "call", Subject: &subject, Body: &body, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: v.deal}},
	})
	if err != nil {
		t.Fatal(err)
	}
	v.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, call.Id)
	v.run(t)

	owned, err := latestCheck(v.owner(t), InstallationDB(v.Pool), v.deal)
	if err != nil {
		t.Fatal(err)
	}
	if owned == nil || !owned.Explained || len(owned.Outcomes) != 1 ||
		owned.Outcomes[0].Kind != crmcontracts.DealMarginceCheckOutcomeKindAmount ||
		owned.Outcomes[0].Why != crmcontracts.DealMarginceCheckOutcomeWhyNotSure {
		t.Fatalf("owner's check = %+v, want the amount left alone as not sure", owned)
	}
	states := map[crmcontracts.DealMarginceCheckSourceSource]crmcontracts.DealMarginceCheckSourceState{}
	for _, s := range owned.Sources {
		states[s.Source] = s.State
	}
	want := map[crmcontracts.DealMarginceCheckSourceSource]crmcontracts.DealMarginceCheckSourceState{
		"notes": "read", "calls": "private", "mail": "unavailable", "meetings": "none", "offers": "none",
	}
	for source, state := range want {
		if states[source] != state {
			t.Errorf("source %s = %q, want %q (all: %+v)", source, states[source], state, owned.Sources)
		}
	}

	seen, err := latestCheck(v.colleague(), InstallationDB(v.Pool), v.deal)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(seen)
	if err != nil {
		t.Fatal(err)
	}
	if seen == nil || seen.Explained || len(seen.Outcomes) != 0 || len(seen.Sources) != 0 || seen.Skipped != nil ||
		strings.Contains(string(wire), "not_sure") || strings.Contains(string(wire), "private") {
		t.Fatalf("colleague's check = %s, want when it ran and nothing of why", wire)
	}
}

func TestAnAppliedChangeLeavesNoCheckOnTheCard(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	check, err := latestCheck(v.owner(t), InstallationDB(v.Pool), v.deal)
	if err != nil || check != nil {
		t.Fatalf("check after Margince changed the deal = %+v, %v; want none, the changes say it", check, err)
	}
}

func TestTheTeamListCountsTheValueEachKindOfChangeMoved(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	report, err := v.Deals.TeamChanges(v.colleague(), []ids.UUID{v.Rep1}, v.now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	moved := map[string]int64{}
	for _, g := range report.Groups {
		moved[g.Kind] = g.ValueMovedBaseMinor
	}
	// 40,000 to 45,000 EUR moves 5,000; ten points less of 45,000 moves 4,500.
	want := map[string]int64{
		deals.VerdictAmountCorrection: 500_000, deals.VerdictProbabilityCorrection: 450_000, deals.VerdictCloseDateCorrection: 0,
	}
	for kind, value := range want {
		if moved[kind] != value {
			t.Errorf("value moved by %s = %d, want %d (groups %+v)", kind, moved[kind], value, report.Groups)
		}
	}
	for _, row := range report.Rows {
		if row.Correction == deals.VerdictProbabilityCorrection && report.Moved[row.AuditLogID] != -450_000 {
			t.Errorf("the probability row moved %d, want -450000: the weighted value went down", report.Moved[row.AuditLogID])
		}
	}

	masked := v.As(v.Rep2, []ids.UUID{v.Team1}, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{"deal": {Read: true}}, RowScope: principal.RowScopeTeam,
		FieldMasks: []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}},
	})
	hidden, err := v.Deals.TeamChanges(masked, []ids.UUID{v.Rep1}, v.now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range hidden.Groups {
		if g.ValueMovedBaseMinor != 0 {
			t.Errorf("a reader with the amount masked sees %d moved by %s, want nothing", g.ValueMovedBaseMinor, g.Kind)
		}
	}
	if len(hidden.Moved) != 0 {
		t.Errorf("a reader with the amount masked sees row values %v, want none", hidden.Moved)
	}

	v.undo(t, deals.VerdictAmountCorrection)
	undone, err := v.Deals.TeamChanges(v.colleague(), []ids.UUID{v.Rep1}, v.now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range undone.Groups {
		if g.Kind == deals.VerdictAmountCorrection && g.ValueMovedBaseMinor != 0 {
			t.Errorf("an undone amount still moved %d, want nothing", g.ValueMovedBaseMinor)
		}
	}
	for _, row := range undone.Rows {
		if _, listed := undone.Moved[row.AuditLogID]; listed && row.Correction == deals.VerdictAmountCorrection {
			t.Errorf("the undone amount row still moved %d, want no entry", undone.Moved[row.AuditLogID])
		}
	}
}

// ownersAgentReader is a passport agent reading for the deal's owner.
func (v *dealVerdictEnv) ownersAgentReader() context.Context {
	return principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), v.WS), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:reader", SeatType: principal.SeatFull,
			OnBehalfOf: v.Rep1, UserID: v.Rep1, TeamIDs: []ids.UUID{v.Team1},
			Scopes:      principal.NewScopeSet(principal.ScopeRead),
			Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"deal": {Read: true}}, RowScope: principal.RowScopeAll},
		})
}

func TestAnAgentActingForTheOwnerReadsNoReasonOnEitherRead(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	agent := v.ownersAgentReader()
	applied, _, err := v.Deals.DealChanges(agent, ids.From[ids.DealKind](v.deal), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 3 {
		t.Fatalf("agent's list = %d changes, want all three", len(applied))
	}
	for _, row := range applied {
		if row.Basis != "" || string(row.Evidence) != "{}" {
			t.Errorf("the owner's agent reads reason %q / evidence %s, want neither", row.Basis, row.Evidence)
		}
	}

	nothing := setupDealVerdict(t)
	nothing.pass.brain = lowConfidenceLane{cite: &nothing.note}
	nothing.run(t)
	check, err := latestCheck(nothing.ownersAgentReader(), InstallationDB(nothing.Pool), nothing.deal)
	if err != nil {
		t.Fatal(err)
	}
	if check == nil || check.Explained || len(check.Outcomes) != 0 || len(check.Sources) != 0 {
		t.Fatalf("the owner's agent's check = %+v, want when it ran and nothing of why", check)
	}
}

// moved is what the team report says each kind moved, and the row values.
func (v *dealVerdictEnv) moved(t *testing.T) (map[string]int64, map[ids.UUID]int64) {
	t.Helper()
	report, err := v.Deals.TeamChanges(v.colleague(), []ids.UUID{v.Rep1}, v.now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]int64{}
	for _, g := range report.Groups {
		groups[g.Kind] = g.ValueMovedBaseMinor
		if g.Unvalued != 0 {
			t.Errorf("%s has %d unvalued deals, want every move valued", g.Kind, g.Unvalued)
		}
	}
	return groups, report.Moved
}

func TestALaterEditDoesNotRewriteTheValueAChangeMoved(t *testing.T) {
	v := setupBundle(t)
	groups, rows := v.moved(t)
	if groups[deals.VerdictStageMoveCorrection] == 0 || groups[deals.VerdictProbabilityCorrection] == 0 || len(rows) != 2 {
		t.Fatalf("moved = %v rows %v, want the stage move and the probability valued", groups, rows)
	}
	amount, restated, usd := int64(9_000_000), int64(18_000_000), "USD"
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{
		Currency: &usd, AmountMinor: &restated,
	}); err != nil {
		t.Fatal(err)
	}
	v.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date) VALUES ('USD', 'EUR', '0.5000000000', CURRENT_DATE)`)
	v.moveOn(t)
	afterGroups, afterRows := v.moved(t)
	for kind, value := range groups {
		if afterGroups[kind] != value {
			t.Errorf("%s moved %d after the edits, want the %d it moved when Margince made it", kind, afterGroups[kind], value)
		}
	}
	for audit, value := range rows {
		if afterRows[audit] != value {
			t.Errorf("row %s moved %d after the edits, want %d", audit, afterRows[audit], value)
		}
	}
}

// amountLane proposes one amount, quoted from the note it cites.
type amountLane struct {
	cite  *string
	whole int
}

func (l *amountLane) Complete(context.Context, model.Request) (model.Response, error) {
	quote := fmt.Sprintf("%d,000 EUR a year", l.whole/1000)
	return model.Response{Text: fmt.Sprintf(`{"actions":[
		{"kind":"set_amount","date":"","amount":%d,"currency":"EUR","quoted_words":%q,
		 "probability":0,"cited_activity_ids":[%q],"reason":"They named the price","confidence":0.9}]}`, l.whole, quote, *l.cite)}, nil
}

func TestTwoAmountChangesOnOneDealCountTheirNetMoveOnce(t *testing.T) {
	v := setupDealVerdict(t)
	lane := &amountLane{cite: &v.note, whole: 45000}
	v.pass.brain = lane
	v.run(t)
	v.note = v.logNote(t, "The board raised it: 50,000 EUR a year.")
	lane.whole = 50000
	v.pass.now = func() time.Time { return v.now.Add(2 * time.Minute) }
	v.run(t)
	if n := v.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1 AND correction = $2`,
		v.deal, deals.VerdictAmountCorrection); n != 2 {
		t.Fatalf("amount changes = %d, want two (outcomes %s)", n, v.outcomes(t))
	}
	groups, rows := v.moved(t)
	if groups[deals.VerdictAmountCorrection] != 1_000_000 {
		t.Errorf("two raises of 5,000 moved %d, want 10,000.00 EUR", groups[deals.VerdictAmountCorrection])
	}
	for audit, value := range rows {
		if value != 500_000 {
			t.Errorf("row %s moved %d, want 5,000.00 EUR each", audit, value)
		}
	}
}
