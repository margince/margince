// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Margince reads an open deal's evidence at night and sets its close date,
// amount and probability as its owner's agent. These tests drive the real
// pass over deals and notes written through the deals and activities stores;
// only the model is a stand-in.

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// dealVerdictRefs lets a lane script its reply by record id. The reply reaches
// the pass in the refs the request showed each activity, stage and criterion by.
func dealVerdictRefs(lane completer) completer {
	return inRefs{inner: lane, shown: []shownBy{
		{key: "activity_id", query: `SELECT left(body, 40) FROM activity WHERE id = $1`},
		{key: "stage_id", query: `SELECT 'Later stage: ' || name || ' (' FROM stage WHERE id = $1`},
		{key: "criterion_id", query: `SELECT ': ' || label || ' (criterion_id' FROM stage_exit_criterion WHERE id = $1`},
	}}
}

// dealVerdictLane answers every deal with the same three changes, citing the
// note the test logged. It counts its calls, and runs during while the pass
// waits for it: between the read and the write.
type dealVerdictLane struct {
	cite        *string
	date        string
	probability int
	// quote is the price the amount change quotes; 45,000 EUR by default.
	quote  string
	calls  int
	during func()
}

func (l *dealVerdictLane) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	l.calls++
	if l.during != nil {
		l.during()
	}
	quote, amount, currency := "45,000 EUR a year", 45000, "EUR"
	if l.quote != "" {
		quote, amount, currency = l.quote, 40000, "GBP"
	}
	return model.Response{Text: fmt.Sprintf(`{"actions":[
		{"kind":"set_close_date","date":%q,"amount":0,"currency":"","quoted_words":"","probability":0,
		 "cited_activity_ids":[%q],"reason":"They will sign then","confidence":0.9},
		{"kind":"set_amount","date":"","amount":%d,"currency":%q,"quoted_words":%q,
		 "probability":0,"cited_activity_ids":[%q],"reason":"They named the price","confidence":0.9},
		{"kind":"set_probability","date":"","amount":0,"currency":"","quoted_words":"","probability":%d,
		 "cited_activity_ids":[%q],"reason":"They said yes","confidence":0.9}]}`,
		l.date, *l.cite, amount, currency, quote, *l.cite, l.probability, *l.cite)}, nil
}

type dealVerdictEnv struct {
	*integration.Env
	deal     ids.UUID
	pipeline ids.PipelineID
	stage    ids.StageID
	note     string
	lane     *dealVerdictLane
	pass     dealVerdictPass
	now      time.Time
}

func setupDealVerdict(t *testing.T) *dealVerdictEnv {
	t.Helper()
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	pipeline, open, _ := integration.DealFixture(t, e)
	owner, amount, currency := ids.From[ids.UserKind](e.Rep1), int64(4_000_000), "EUR"
	created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Support rollout", PipelineID: pipeline, StageID: open, OwnerID: &owner,
		AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	v := &dealVerdictEnv{Env: e, deal: ids.UUID(created.Id), pipeline: pipeline, stage: open}
	// The note is dated after the deal was created, so no colleague's edit is
	// newer than the evidence until a test makes one.
	v.now = v.dbNow(t)
	signing := v.now.AddDate(0, 2, 0).Format(time.DateOnly)
	v.note = v.logNote(t, "The board agreed: we sign on "+signing+" for 45,000 EUR a year.")
	// A lower probability than the stage's: Margince may lower freely.
	stagePct, err := strconv.Atoi(v.WsScalar(t, `SELECT `+deals.StageWinProbabilitySQL("d")+` FROM deal d WHERE d.id = $1`, v.deal))
	if err != nil {
		t.Fatal(err)
	}
	v.lane = &dealVerdictLane{cite: &v.note, date: signing, probability: stagePct - 10}
	v.pass = newDealVerdictPass(e.Pool, dealVerdictRefs(v.lane), slog.New(slog.DiscardHandler))
	v.pass.now = func() time.Time { return v.now.Add(time.Minute) }
	return v
}

func (v *dealVerdictEnv) dbNow(t *testing.T) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, v.WsScalar(t, `SELECT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`))
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func (v *dealVerdictEnv) logNote(t *testing.T, body string) string {
	t.Helper()
	subject, at := "Call notes", v.now
	note, _, err := v.Activities.LogActivity(v.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Body: &body, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: v.deal}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return note.Id.String()
}

func (v *dealVerdictEnv) run(t *testing.T) {
	t.Helper()
	if err := v.pass.RunWorkspace(principal.SystemActing(v.Admin(), closeDateSweepActor)); err != nil {
		t.Fatal(err)
	}
}

func (v *dealVerdictEnv) owner(t *testing.T) context.Context {
	t.Helper()
	ctx, err := dealOwnerAuthority{db: InstallationDB(v.Pool), users: identity.NewService(v.Pool)}.asOwner(v.Admin(), v.Rep1)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func (v *dealVerdictEnv) field(t *testing.T, column string) string {
	t.Helper()
	return v.WsScalar(t, `SELECT coalesce(`+column+`::text, '') FROM deal WHERE id = $1`, v.deal)
}

func (v *dealVerdictEnv) outcomes(t *testing.T) string {
	t.Helper()
	return v.WsScalar(t, `SELECT string_agg(a->>'kind' || '=' || (a->>'outcome'), ',' ORDER BY a->>'kind')
		FROM assurance_deal_receipt r, jsonb_array_elements(r.verdict_actions->'actions') a WHERE r.deal_id = $1`, v.deal)
}

func (v *dealVerdictEnv) undo(t *testing.T, correction string) {
	t.Helper()
	var id ids.UUID
	err := database.WithWorkspaceTx(v.Admin(), v.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(v.Admin(), `SELECT id FROM deal_correction
			WHERE deal_id = $1 AND correction = $2 AND reversed_at IS NULL`, v.deal, correction).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Deals.RevertCorrection(v.owner(t), id, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMarginceSetsWhatTheCustomerSaidAndAnUndoHoldsItBack(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)

	if got := v.field(t, "expected_close_date"); got != v.lane.date {
		t.Fatalf("close date = %q, want the date the customer named %s", got, v.lane.date)
	}
	if got := v.field(t, "amount_minor"); got != "4500000" {
		t.Fatalf("amount = %s, want 45,000.00 EUR", got)
	}
	if got := v.field(t, "win_probability_override") + "/" + v.field(t, "win_probability_set_by"); got != strconv.Itoa(v.lane.probability)+"/margince" {
		t.Fatalf("probability = %s, want %d set by Margince", got, v.lane.probability)
	}
	if n := v.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND actor_id = $2 AND on_behalf_of = $3`,
		v.deal, marginceActorID, v.Rep1); n != 3 {
		t.Errorf("audit rows by Margince for the owner = %d, want one per change", n)
	}
	if n := v.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.subject = 'Margince updated this deal'`, v.deal); n != 1 {
		t.Errorf("timeline notes = %d, want one for the night", n)
	}
	receipts := attentionReceipts{svc: approvals.NewService(InstallationDB(v.Pool)), deals: v.Deals}
	recent, err := receipts.Recent(v.owner(t), v.now.Add(72*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if !dealReceiptNamed(recent, "Changed the amount", "45,000 EUR a year") || !dealReceiptNamed(recent, "win probability", strconv.Itoa(v.lane.probability)+"%") {
		t.Fatalf("Home three days on = %+v, want the amount with its quote and the probability, each with Undo", recent)
	}

	v.run(t)
	if v.lane.calls != 1 {
		t.Fatalf("model calls after a second night with nothing new = %d, want 1", v.lane.calls)
	}

	v.undo(t, deals.VerdictProbabilityCorrection)
	if got := v.field(t, "win_probability_override"); got != "" {
		t.Fatalf("after Undo the probability override = %q, want the stage's again", got)
	}
	v.run(t)
	if got := v.field(t, "win_probability_override"); got != "" || !strings.Contains(v.outcomes(t), "set_probability=dropped:undo_hold") {
		t.Fatalf("the night after an Undo: override %q, outcomes %s; want the change held back", got, v.outcomes(t))
	}

	v.now = v.now.Add(time.Hour)
	v.note = v.logNote(t, "They confirmed again: yes.")
	v.run(t)
	if got := v.field(t, "win_probability_override"); got != strconv.Itoa(v.lane.probability) {
		t.Fatalf("after a new note the probability = %q, want Margince to set it again", got)
	}
}

func TestASwitchedOffKindIsLeftAlone(t *testing.T) {
	v := setupDealVerdict(t)
	if _, err := approvals.NewService(InstallationDB(v.Pool)).SetAutoApply(v.owner(t), deals.VerdictAmountCorrection, false); err != nil {
		t.Fatal(err)
	}
	v.run(t)
	if got := v.field(t, "amount_minor"); got != "4000000" {
		t.Fatalf("amount = %s, want it untouched with the switch off", got)
	}
	if got := v.field(t, "win_probability_override"); got != strconv.Itoa(v.lane.probability) {
		t.Fatalf("probability = %q, want the other kinds still applied", got)
	}
	if !strings.Contains(v.outcomes(t), "set_amount=dropped:switch_off") {
		t.Fatalf("outcomes = %s, want the amount recorded as switched off", v.outcomes(t))
	}
}

func TestAColleaguesEditAfterTheEvidenceWins(t *testing.T) {
	v := setupDealVerdict(t)
	amount := int64(3_000_000)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	v.run(t)
	if got := v.field(t, "amount_minor"); got != "3000000" {
		t.Fatalf("amount = %s, want the colleague's 30,000.00 EUR kept", got)
	}
	if !strings.Contains(v.outcomes(t), "set_amount=dropped:human_edit") {
		t.Fatalf("outcomes = %s, want the amount dropped for the colleague's edit", v.outcomes(t))
	}
}

func dealReceiptNamed(receipts []attention.Receipt, parts ...string) bool {
	for _, receipt := range receipts {
		matched := receipt.Undo != nil
		for _, part := range parts {
			matched = matched && strings.Contains(receipt.Summary, part)
		}
		if matched {
			return true
		}
	}
	return false
}

// untouched says the deal holds what it held before the pass, and that its
// stored checksum leaves it due for the next night.
func (v *dealVerdictEnv) untouched(t *testing.T) {
	t.Helper()
	if got := v.field(t, "amount_minor") + "/" + v.field(t, "win_probability_override"); got != "4000000/" {
		t.Fatalf("amount/probability = %s, want nothing written", got)
	}
	if !strings.Contains(v.outcomes(t), "set_amount="+deals.VerdictDroppedMoved) {
		t.Fatalf("outcomes = %s, want every change dropped because the evidence moved", v.outcomes(t))
	}
	if v.WsScalar(t, `SELECT (r.verdict_input_checksum IS DISTINCT FROM `+dealVerdictChecksumSQL()+`)::text
		  FROM deal d JOIN assurance_deal_receipt r ON r.deal_id = d.id WHERE d.id = $1`, v.deal) != "true" {
		t.Fatal("the stored checksum matches the deal, so the next night would not read it again")
	}
}

func TestAnOfferAcceptedWhileTheModelReadsStopsTheWrite(t *testing.T) {
	v := setupDealVerdict(t)
	desk := v.As(v.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"deal_desk"},
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true, Update: true}, "offer": {Create: true, Read: true, Update: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	description, price, tax := "Rollout", int64(4_000_000), "0.00"
	created, err := v.Deals.CreateOffer(desk, ids.From[ids.DealKind](v.deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax}},
	})
	if err != nil {
		t.Fatal(err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, err := v.Deals.SendOffer(desk, offer, nil); err != nil {
		t.Fatal(err)
	}
	v.lane.during = func() {
		if _, err := v.Deals.AcceptOffer(desk, offer, nil); err != nil {
			t.Errorf("accepting the offer: %v", err)
		}
	}
	v.run(t)
	v.untouched(t)
}

func TestACitedNoteMovedAwayWhileTheModelReadsStopsTheWrite(t *testing.T) {
	v := setupDealVerdict(t)
	owner := ids.From[ids.UserKind](v.Rep1)
	other, err := v.Deals.CreateDeal(v.Admin(), deals.CreateDealInput{
		Name: "Another deal", PipelineID: v.pipeline, StageID: v.stage, OwnerID: &owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	v.lane.during = func() {
		if _, err := v.Activities.RelinkActivity(v.Admin(), ids.From[ids.ActivityKind](ids.MustParse(v.note)),
			activities.RelinkActivityInput{EntityType: "deal", EntityID: ids.UUID(other.Id), ReplaceExistingOfType: true}); err != nil {
			t.Errorf("moving the note: %v", err)
		}
	}
	v.run(t)
	v.untouched(t)
}

// The owner leaves, or loses the deal grant, after Margince bound itself as
// their agent. The write binds them again and refuses.
func TestAnOwnerWhoLeavesBeforeTheWriteGetsNoChange(t *testing.T) {
	for name, c := range map[string]struct{ revoke, outcome string }{
		"deactivated":  {`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, droppedNoOwner},
		"role removed": {`DELETE FROM role_assignment WHERE user_id = $1`, deals.VerdictDroppedOwnerCannotEdit},
	} {
		t.Run(name, func(t *testing.T) { verdictAfterTheOwnerLeft(t, c.revoke, c.outcome) })
	}
}

func verdictAfterTheOwnerLeft(t *testing.T, revoke, wantOutcome string) {
	t.Helper()
	v := setupDealVerdict(t)
	ctx := principal.SystemActing(v.Admin(), closeDateSweepActor)
	c := verdictCandidate{deal: v.deal, owner: v.Rep1}
	var read dealVerdictRead
	if err := database.WithWorkspaceTx(ctx, v.Pool, func(tx pgx.Tx) error {
		var err error
		read, err = readDealVerdict(ctx, tx, v.deal, nil, v.now, v.now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	v.WsExec(t, revoke, v.Rep1)
	amount := deals.VerdictChange{
		Correction: deals.VerdictAmountCorrection, AmountMinor: 4_500_000,
		Evidence: map[string]any{"cited_activity_ids": []string{v.note}},
	}
	outcomes, _, err := v.pass.applyAsOwner(ctx, c, read, []deals.VerdictChange{amount})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0] != wantOutcome {
		t.Errorf("outcomes = %v, want [%s]", outcomes, wantOutcome)
	}
	if got := v.field(t, "amount_minor") + "/" + v.field(t, "win_probability_override"); got != "4000000/" {
		t.Fatalf("amount/probability = %s, want nothing written for an owner who left or lost the grant", got)
	}
}

// Home names an amount in the currency it was written in, even after the
// deal's currency changed.
func TestAReceiptKeepsTheCurrencyTheAmountWasSetIn(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	usd := "USD"
	restated := int64(5_000_000)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{
		Currency: &usd, AmountMinor: &restated,
	}); err != nil {
		t.Fatal(err)
	}
	receipts := attentionReceipts{svc: approvals.NewService(InstallationDB(v.Pool)), deals: v.Deals}
	recent, err := receipts.Recent(v.owner(t), v.now.Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if !dealReceiptNamed(recent, "Changed the amount", "→ 45000.00 EUR") {
		t.Fatalf("Home = %+v, want the amount in EUR, the currency it was set in", recent)
	}
}

// A price in a currency with no stored rate is read again once a rate is.
func TestANewRateMakesANoRateDropDueAgain(t *testing.T) {
	v := setupDealVerdict(t)
	v.note = v.logNote(t, "Our budget is 40,000 GBP a year.")
	v.lane.quote = "40,000 GBP"
	v.run(t)
	if !strings.Contains(v.outcomes(t), "set_amount="+dealverdict.DroppedNoRate) {
		t.Fatalf("outcomes = %s, want the amount dropped for the missing rate", v.outcomes(t))
	}
	calls := v.lane.calls
	v.run(t)
	if v.lane.calls != calls {
		t.Fatal("a deal with nothing new was read again")
	}
	v.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date, created_at)
		VALUES ('GBP', 'EUR', '1.1500000000', CURRENT_DATE, $1)`, v.now.Add(time.Hour))
	v.run(t)
	if v.lane.calls != calls+1 {
		t.Fatal("a stored rate did not make the no-rate deal due again")
	}
}
