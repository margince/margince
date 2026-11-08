// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The report of what Margince changed, over changes the real nightly pass
// wrote. It covers the owner's open bundle, the Undo of many, and what a
// colleague may read of somebody else's deal.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// standInReason is the reason the stand-in model gives for the close date.
const standInReason = "They will sign then"

func TestOneDealChangedThreeWaysCountsOnceAndItsUndoReportsWhatWasKept(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	h := newMarginceChangeHandlers(v.Pool)

	summary, err := h.deals.PendingChangesOwnedBy(v.owner(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Deals != 1 || summary.Closed != 0 || summary.Redated != 1 || summary.Revalued != 1 || summary.Probabilities != 1 {
		t.Fatalf("summary = %+v, want one deal with a new date, amount and probability", summary)
	}
	if len(summary.Bundles) != 1 || len(summary.Bundles[0].AuditLogIDs) != 3 {
		t.Fatalf("bundles = %+v, want one bundle naming the three changes", summary.Bundles)
	}
	bundle := summary.Bundles[0]
	item := crmcontracts.UndoMarginceChangeItem{DealId: openapi_types.UUID(bundle.DealID.UUID), AuditLogIds: wireUUIDs(bundle.AuditLogIDs)}

	item.Version = bundle.Version - 1
	refused, err := h.undoBundle(v.owner(t), item)
	if err != nil {
		t.Fatal(err)
	}
	if refused.Outcome != crmcontracts.UndoMarginceChangeResultOutcomeRefused || len(refused.Kept) != 3 ||
		refused.Kept[0].Reason != crmcontracts.UndoMarginceChangeKeptReasonVersionSkew {
		t.Fatalf("undo at a version nobody read = %+v, want every change refused for the version", refused)
	}

	amount := int64(3_000_000)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	current, err := strconv.ParseInt(v.field(t, "version"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	item.Version = current
	partly, err := h.undoBundle(v.owner(t), item)
	if err != nil {
		t.Fatal(err)
	}
	if partly.Outcome != crmcontracts.UndoMarginceChangeResultOutcomePartlyUndone || len(partly.Undone) != 2 ||
		len(partly.Kept) != 1 || partly.Kept[0].Kind == nil || *partly.Kept[0].Kind != deals.VerdictAmountCorrection ||
		partly.Kept[0].Reason != crmcontracts.UndoMarginceChangeKeptReasonChangedSince {
		t.Fatalf("undo after a colleague's amount = %+v, want the date and probability undone and the amount kept", partly)
	}
	if got := v.field(t, "amount_minor"); got != "3000000" {
		t.Fatalf("amount = %s, want the colleague's 30,000.00 EUR kept", got)
	}
	after, err := h.deals.PendingChangesOwnedBy(v.owner(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Deals != 1 || after.Revalued != 1 || after.Redated != 0 {
		t.Fatalf("summary after the undo = %+v, want only the kept amount open", after)
	}
}

// colleague reads every deal of the workspace but owns none of them.
func (v *dealVerdictEnv) colleague() context.Context {
	return v.As(v.Rep2, []ids.UUID{v.Team1}, principal.Permissions{
		RoleKeys: []string{"manager"},
		Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
}

func TestAColleagueSeesWhatMarginceChangedButNotWhy(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	h := newMarginceChangeHandlers(v.Pool)
	dealID := ids.From[ids.DealKind](v.deal)

	owned, _, err := h.deals.DealChanges(v.owner(t), dealID, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(owned, func(c deals.CorrectionReceipt) bool { return c.Basis == standInReason }) {
		t.Fatalf("owner's list = %+v, want the reason Margince gave", owned)
	}

	seen, _, err := h.deals.DealChanges(v.colleague(), dealID, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	team, err := h.deals.TeamChanges(v.colleague(), []ids.UUID{v.Rep1}, v.now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for surface, rows := range map[string][]deals.CorrectionReceipt{"deal list": seen, "team list": team.Rows} {
		if len(rows) != 3 {
			t.Fatalf("%s for a colleague = %d changes, want all three", surface, len(rows))
		}
		for _, row := range rows {
			if row.Basis != "" || string(row.Evidence) != "{}" {
				t.Errorf("%s for a colleague carries the reason %q / evidence %s, want neither", surface, row.Basis, row.Evidence)
			}
		}
	}

	agent := principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), v.WS), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:change-report", SeatType: principal.SeatFull,
			OnBehalfOf: v.Rep2, UserID: v.Rep2, TeamIDs: []ids.UUID{v.Team1},
			Scopes:      principal.NewScopeSet(principal.ScopeRead),
			Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"deal": {Read: true}}, RowScope: principal.RowScopeAll},
		})
	w := httptest.NewRecorder()
	h.ListDealMarginceChanges(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(agent),
		openapi_types.UUID(v.deal), crmcontracts.ListDealMarginceChangesParams{})
	var page crmcontracts.DealMarginceChanges
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("agent read = %d %s", w.Code, w.Body.String())
	}
	if len(page.Changes) != 3 || strings.Contains(w.Body.String(), standInReason) || strings.Contains(w.Body.String(), "45,000 EUR a year") {
		t.Fatalf("agent for a colleague = %s, want three changes with no reason or quote", w.Body.String())
	}
}

func TestTheMorningEmailListsTheChangedDealAndAnUndoBeforeTheSendStopsIt(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	worker := &briefGenerateWorker{
		engine: briefs.NewBriefEngine(v.Pool, contacts.NewStore(InstallationDB(v.Pool))),
		pool:   v.Pool, users: identity.NewService(v.Pool), log: slog.New(slog.DiscardHandler),
	}
	now, words := v.now.Add(time.Hour), mailcopy.For("en")

	message, useful, items, err := worker.morningMessageItems(v.owner(t), now, words, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !useful || !strings.Contains(message.Text, "Margince changed 1 of your deals last night") || !strings.Contains(message.Text, "Support rollout") {
		t.Fatalf("morning with only Margince's changes = useful %v:\n%s\nwant it sent naming the deal", useful, message.Text)
	}
	var basis sendBasis
	if err := database.WithWorkspaceTx(v.owner(t), v.Pool, func(tx pgx.Tx) error {
		basis, err = readSendBasis(v.owner(t), tx, v.Rep1, nil, morningRefs(items, digestLines{}), now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	v.undo(t, deals.VerdictAmountCorrection)
	err = database.WithWorkspaceTx(v.owner(t), v.Pool, func(tx pgx.Tx) error {
		return holdSendBasisTx(v.owner(t), tx, basis, func() time.Time { return now })
	})
	if !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("holding a morning after an Undo of what it lists = %v, want the morning read again", err)
	}
}

func TestAnOwnerWhoseAmountIsMaskedSeesTheChangeButNoAmount(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	masked := v.As(v.Rep1, []ids.UUID{v.Team1}, principal.Permissions{
		Objects:    map[string]principal.ObjectGrant{"deal": {Read: true}},
		RowScope:   principal.RowScopeTeam,
		FieldMasks: []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}},
	})
	rows, _, err := newMarginceChangeHandlers(v.Pool).deals.DealChanges(masked, ids.From[ids.DealKind](v.deal), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	var amount *deals.CorrectionReceipt
	for i := range rows {
		if rows[i].Correction == deals.VerdictAmountCorrection {
			amount = &rows[i]
		}
	}
	if amount == nil {
		t.Fatalf("changes = %+v, want the amount change listed", rows)
	}
	shown := attention.WireReceipt(correctionReceipt(*amount))
	wire, err := json.Marshal(shown)
	if err != nil {
		t.Fatal(err)
	}
	if amount.BeforeAmount != nil || amount.AfterAmount != nil ||
		strings.Contains(string(amount.Evidence), "45000") || strings.Contains(string(amount.Evidence), "quoted") || strings.Contains(string(wire), "45,000") || strings.Contains(string(wire), "45000") {
		t.Fatalf("masked owner's amount change = %s (evidence %s), want no amount anywhere", wire, amount.Evidence)
	}
}

func TestAnAgentCannotUndoMarginceChangesInBulk(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	agent := principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), v.WS), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:bulk-undo", SeatType: principal.SeatFull,
			OnBehalfOf: v.Rep1, UserID: v.Rep1, TeamIDs: []ids.UUID{v.Team1},
			Scopes:      principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
			Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}}, RowScope: principal.RowScopeAll},
		})
	body := `{"items":[{"deal_id":"` + v.deal.String() + `","version":1,"audit_log_ids":["` + v.deal.String() + `"]}]}`
	w := httptest.NewRecorder()
	newMarginceChangeHandlers(v.Pool).UndoMarginceChanges(w,
		httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)).WithContext(agent))
	if w.Code != http.StatusForbidden {
		t.Fatalf("an agent's bulk Undo = %d %s, want 403 as the single Undo answers", w.Code, w.Body.String())
	}
	if got := v.field(t, "amount_minor"); got != "4500000" {
		t.Fatalf("amount after the refused Undo = %s, want Margince's 45,000.00 EUR still there", got)
	}
}

// accept keeps one of the deal's changes, as the owner's Accept does.
func (v *dealVerdictEnv) accept(t *testing.T, dealID ids.UUID, correction string) {
	t.Helper()
	audit := v.WsScalar(t, `SELECT audit_log_id::text FROM deal_correction WHERE deal_id = $1 AND correction = $2`, dealID, correction)
	version, err := strconv.ParseInt(v.WsScalar(t, `SELECT version::text FROM deal WHERE id = $1`, dealID), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Deals.AcceptAppliedChange(v.owner(t), ids.From[ids.DealKind](dealID), ids.MustParse(audit), version); err != nil {
		t.Fatal(err)
	}
}

func TestAReviewedChangeStillUndoesOnEveryPath(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	h := newMarginceChangeHandlers(v.Pool)
	summary, err := h.deals.PendingChangesOwnedBy(v.owner(t), nil)
	if err != nil || len(summary.Bundles) != 1 {
		t.Fatalf("pending = %+v %v", summary, err)
	}
	bundle := summary.Bundles[0]
	v.accept(t, v.deal, deals.VerdictAmountCorrection)

	w := httptest.NewRecorder()
	h.ListDealMarginceChanges(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(v.owner(t)),
		openapi_types.UUID(v.deal), crmcontracts.ListDealMarginceChangesParams{})
	var page crmcontracts.DealMarginceChanges
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	listed := slices.ContainsFunc(page.Changes, func(change crmcontracts.Receipt) bool {
		return change.Kind == deals.VerdictAmountCorrection && change.Undo != nil &&
			change.Review != nil && change.Review.Accepted && change.Review.CanUndo
	})
	if !listed {
		t.Fatalf("the deal's list shows the reviewed amount as %s, want it reviewed and still offering Undo", w.Body.String())
	}

	result, err := h.undoBundle(v.owner(t), crmcontracts.UndoMarginceChangeItem{
		DealId: openapi_types.UUID(v.deal), Version: bundle.Version, AuditLogIds: wireUUIDs(bundle.AuditLogIDs),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Undone, deals.VerdictAmountCorrection) || v.field(t, "amount_minor") == "4500000" {
		t.Fatalf("bulk Undo after review = %+v, amount %s; want the reviewed amount taken back", result, v.field(t, "amount_minor"))
	}
}

// morningBasis reads one morning and the basis its send would hold.
func (v *dealVerdictEnv) morningBasis(t *testing.T, now time.Time) (string, sendBasis) {
	t.Helper()
	worker := &briefGenerateWorker{
		engine: briefs.NewBriefEngine(v.Pool, contacts.NewStore(InstallationDB(v.Pool))),
		pool:   v.Pool, users: identity.NewService(v.Pool), log: slog.New(slog.DiscardHandler),
	}
	message, _, items, err := worker.morningMessageItems(v.owner(t), now, mailcopy.For("en"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var basis sendBasis
	if err := database.WithWorkspaceTx(v.owner(t), v.Pool, func(tx pgx.Tx) error {
		basis, err = readSendBasis(v.owner(t), tx, v.Rep1, nil, morningRefs(items, digestLines{}), now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return message.Text, basis
}

func (v *dealVerdictEnv) holds(t *testing.T, basis sendBasis, now time.Time) error {
	t.Helper()
	return database.WithWorkspaceTx(v.owner(t), v.Pool, func(tx pgx.Tx) error {
		return holdSendBasisTx(v.owner(t), tx, basis, func() time.Time { return now })
	})
}

func TestAMorningIsReadAgainWhenAnyChangeItCountsIsAcceptedOrUndone(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	first := v.deal
	owner := ids.From[ids.UserKind](v.Rep1)
	for i := range 5 {
		amount, currency := int64(4_000_000), "EUR"
		created, err := v.Deals.CreateDeal(v.Admin(), deals.CreateDealInput{
			Name: fmt.Sprintf("Rollout %d", i), PipelineID: v.pipeline, StageID: v.stage, OwnerID: &owner,
			AmountMinor: &amount, Currency: &currency,
		})
		if err != nil {
			t.Fatal(err)
		}
		v.deal = ids.UUID(created.Id)
		v.note = v.logNote(t, "We sign on "+v.lane.date+" for 45,000 EUR a year.")
		v.run(t)
	}
	now := v.now.Add(time.Hour)

	text, basis := v.morningBasis(t, now)
	if !strings.Contains(text, "Margince changed 6 of your deals last night") || !strings.Contains(text, "and 1 more deals") || strings.Contains(text, "Support rollout") {
		t.Fatalf("morning with six changed deals:\n%s\nwant five named, the first counted only", text)
	}
	if err := v.holds(t, basis, now); err != nil {
		t.Fatalf("holding before any change = %v", err)
	}
	v.accept(t, first, deals.VerdictProbabilityCorrection)
	if err := v.holds(t, basis, now); !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("holding after an Accept on a counted, unlisted deal = %v, want the morning read again", err)
	}

	_, basis = v.morningBasis(t, now)
	v.accept(t, v.deal, deals.VerdictAmountCorrection)
	if err := v.holds(t, basis, now); !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("holding after an Accept on a listed deal = %v, want the morning read again", err)
	}
}

// bundleLane moves the deal a stage on, then lowers its probability and sets
// its close date. All three cite the note.
type bundleLane struct {
	move *stageMoveLane
	date string
}

func (l bundleLane) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	moved, err := l.move.Complete(ctx, req)
	if err != nil {
		return moved, err
	}
	var answer struct {
		Actions []map[string]any `json:"actions"`
	}
	if err := json.Unmarshal([]byte(moved.Text), &answer); err != nil {
		return model.Response{}, err
	}
	answer.Actions = append(answer.Actions,
		map[string]any{
			"kind": "set_probability", "date": "", "amount": 0, "currency": "", "quoted_words": "", "probability": 5,
			"cited_activity_ids": []string{l.move.note}, "reason": "They hesitate", "confidence": 0.9,
		},
		map[string]any{
			"kind": "set_close_date", "date": l.date, "amount": 0, "currency": "", "quoted_words": "", "probability": 0,
			"cited_activity_ids": []string{l.move.note}, "reason": standInReason, "confidence": 0.9,
		})
	text, err := json.Marshal(answer)
	return model.Response{Text: string(text)}, err
}

// setupBundle is a deal Margince moved, re-dated and gave a probability in one
// night.
func setupBundle(t *testing.T) *stageMoveEnv {
	t.Helper()
	v := setupStageMove(t)
	v.pass = newDealVerdictPass(v.Pool, bundleLane{move: v.move, date: v.lane.date}, slog.New(slog.DiscardHandler))
	v.pass.now = func() time.Time { return v.now.Add(time.Minute) }
	v.run(t)
	if n := v.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1`, v.deal); n != 3 {
		t.Fatalf("corrections = %d, want the move, the probability and the date (outcomes %s)", n, v.outcomes(t))
	}
	return v
}

// moveOn is a colleague moving the deal past the stage Margince moved it to.
func (v *stageMoveEnv) moveOn(t *testing.T) {
	t.Helper()
	later := nextOpenStage(t, v.dealVerdictEnv, v.next)
	if _, err := v.Deals.AdvanceDeal(v.owner(t), ids.From[ids.DealKind](v.deal), deals.AdvanceDealInput{ToStageID: later}); err != nil {
		t.Fatal(err)
	}
}

func TestAnUndoRefusedForTheStageLeavesTheWholeDealAsItWas(t *testing.T) {
	v := setupBundle(t)
	v.moveOn(t)
	h := newMarginceChangeHandlers(v.Pool)
	summary, err := h.deals.PendingChangesOwnedBy(v.owner(t), nil)
	if err != nil || len(summary.Bundles) != 1 || len(summary.Bundles[0].AuditLogIDs) != 3 {
		t.Fatalf("pending = %+v %v, want one bundle of three", summary, err)
	}
	before := v.WsScalar(t, `SELECT row_to_json(d)::text FROM (SELECT stage_id, expected_close_date, win_probability_override,
		win_probability_set_by, version FROM deal WHERE id = $1) d`, v.deal)

	result, err := h.undoBundle(v.owner(t), crmcontracts.UndoMarginceChangeItem{
		DealId: openapi_types.UUID(v.deal), Version: summary.Bundles[0].Version, AuditLogIds: wireUUIDs(summary.Bundles[0].AuditLogIDs),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != crmcontracts.UndoMarginceChangeResultOutcomeRefused || len(result.Undone) != 0 || len(result.Kept) != 3 {
		t.Fatalf("undo after a colleague moved the deal on = %+v, want every change kept", result)
	}
	for _, kept := range result.Kept {
		if kept.Reason != crmcontracts.UndoMarginceChangeKeptReasonChangedSince {
			t.Errorf("kept %v for %s, want changed_since: the deal moved since", kept.Kind, kept.Reason)
		}
	}
	if after := v.WsScalar(t, `SELECT row_to_json(d)::text FROM (SELECT stage_id, expected_close_date, win_probability_override,
		win_probability_set_by, version FROM deal WHERE id = $1) d`, v.deal); after != before {
		t.Fatalf("deal after the refused Undo = %s, want it as it was %s", after, before)
	}
	if n := v.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1 AND reversed_at IS NOT NULL`, v.deal); n != 0 {
		t.Fatalf("corrections stamped undone = %d, want none", n)
	}
}

func TestAHeldDealKeepsAReviewedChangeForTheDealsReason(t *testing.T) {
	v := setupBundle(t)
	v.accept(t, v.deal, deals.VerdictProbabilityCorrection)
	v.moveOn(t)
	version, err := strconv.ParseInt(v.field(t, "version"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	var audits []openapi_types.UUID
	for id := range strings.SplitSeq(v.WsScalar(t, `SELECT string_agg(audit_log_id::text, ',') FROM deal_correction WHERE deal_id = $1`, v.deal), ",") {
		audits = append(audits, openapi_types.UUID(ids.MustParse(id)))
	}
	result, err := newMarginceChangeHandlers(v.Pool).undoBundle(v.owner(t), crmcontracts.UndoMarginceChangeItem{
		DealId: openapi_types.UUID(v.deal), Version: version, AuditLogIds: audits,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]crmcontracts.UndoMarginceChangeKeptReason{
		deals.VerdictProbabilityCorrection: crmcontracts.UndoMarginceChangeKeptReasonChangedSince,
		deals.VerdictCloseDateCorrection:   crmcontracts.UndoMarginceChangeKeptReasonChangedSince,
		deals.VerdictStageMoveCorrection:   crmcontracts.UndoMarginceChangeKeptReasonChangedSince,
	}
	if result.Outcome != crmcontracts.UndoMarginceChangeResultOutcomeRefused || len(result.Kept) != len(want) {
		t.Fatalf("undo = %+v, want all three kept", result)
	}
	for _, kept := range result.Kept {
		if kept.Kind == nil || kept.Reason != want[*kept.Kind] {
			t.Errorf("kept %v for %s, want %s", kept.Kind, kept.Reason, want[derefString(kept.Kind)])
		}
	}
}

// mailedMorning records a morning edition sent to the owner at at, through the
// brief engine's own claim.
func (v *dealVerdictEnv) mailedMorning(t *testing.T, at time.Time) {
	t.Helper()
	engine := briefs.NewBriefEngine(v.Pool, contacts.NewStore(InstallationDB(v.Pool)))
	run, err := engine.SnapshotRun(v.owner(t), at)
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := engine.ClaimMailAttempt(v.owner(t), run.ID, at); err != nil || !claimed {
		t.Fatalf("claiming the earlier morning = %v %v", claimed, err)
	}
}

func TestLastNightStartsWhenThePreviousMorningWasSent(t *testing.T) {
	const counted = "Margince changed 1 of your deals last night"
	for _, tc := range []struct {
		name    string
		sentAt  time.Duration
		readAt  time.Duration
		counted bool
	}{
		{"a morning sent after the changes", time.Hour, 2 * time.Hour, false},
		{"a morning sent while a change could still commit", time.Minute, 2 * time.Hour, true},
		{"a morning sent before the changes, read 30 hours later", -time.Hour, 30 * time.Hour, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := setupDealVerdict(t)
			v.run(t)
			v.mailedMorning(t, v.now.Add(tc.sentAt))
			text, _ := v.morningBasis(t, v.now.Add(tc.readAt))
			if strings.Contains(text, counted) != tc.counted {
				t.Fatalf("morning:\n%s\nwant the changes counted = %t", text, tc.counted)
			}
		})
	}
}
