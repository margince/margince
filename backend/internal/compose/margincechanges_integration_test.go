// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The report of what Margince changed, over changes the real nightly pass
// wrote: the owner's open bundle, the Undo of many, and what a colleague may
// read of somebody else's deal.

import (
	"context"
	"encoding/json"
	"errors"
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
		strings.Contains(string(amount.Evidence), "45") || strings.Contains(string(wire), "45,000") || strings.Contains(string(wire), "45000") {
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
