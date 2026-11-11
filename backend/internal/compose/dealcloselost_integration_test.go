// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Margince closes a deal the customer stopped answering, as its owner's agent,
// and the owner can undo it from Home. These tests drive the real closer over
// deals written through the deals and activities stores.

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type closeLostEnv struct {
	*viabilityEnv
	closer silentDealCloser
}

func setupCloseLost(t *testing.T) *closeLostEnv {
	t.Helper()
	v := setupViability(t)
	// The owner is a rep: Margince acts with the grants their seat holds.
	v.GrantRole(t, v.Rep1, "rep")
	closer := newSilentDealCloser(v.Pool, slog.New(slog.DiscardHandler))
	closer.now = func() time.Time { return v.now }
	return &closeLostEnv{viabilityEnv: v, closer: closer}
}

// run is one nightly check, as the close-date sweep worker runs it.
func (c *closeLostEnv) run(t *testing.T) int {
	t.Helper()
	closed, err := c.closer.CloseWorkspace(principal.SystemActing(c.Admin(), closeDateSweepActor))
	if err != nil {
		t.Fatal(err)
	}
	return closed
}

func (c *closeLostEnv) status(t *testing.T, deal ids.UUID) string {
	t.Helper()
	return c.WsScalar(t, `SELECT status FROM deal WHERE id = $1`, deal)
}

// owner is the deal owner acting by hand, with the grants their seat holds.
func (c *closeLostEnv) owner(t *testing.T) context.Context {
	t.Helper()
	ctx, err := dealOwnerAuthority{db: InstallationDB(c.Pool), users: identity.NewService(c.Pool)}.asOwner(c.Admin(), c.Rep1)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

// undo takes the close back the way Home's Undo does.
func (c *closeLostEnv) undo(t *testing.T, deal ids.UUID) {
	t.Helper()
	var correction ids.UUID
	err := database.WithWorkspaceTx(c.Admin(), c.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(c.Admin(), `SELECT id FROM deal_correction
			WHERE deal_id = $1 AND correction = 'close_lost' AND reversed_at IS NULL`, deal).Scan(&correction)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Deals.RevertCorrection(c.owner(t), correction, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMarginceClosesADealTheCustomerStoppedAnswering(t *testing.T) {
	c := setupCloseLost(t)
	stageBefore := c.WsScalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, c.silent)

	if got := c.run(t); got != 1 {
		t.Fatalf("closed %d deals, want 1: only the deal silent for 196 days is due", got)
	}
	if got := c.status(t, c.silent); got != string(deals.DealLost) {
		t.Fatalf("the silent deal is %s, want lost", got)
	}
	if got := c.status(t, c.supported); got != string(deals.DealOpen) {
		t.Fatalf("the deal the customer answered last week is %s, want open", got)
	}
	if got := c.WsScalar(t, `SELECT lost_reason_code FROM deal WHERE id = $1`, c.silent); got != deals.LossReasonWentSilent {
		t.Errorf("lost reason code = %q, want %q", got, deals.LossReasonWentSilent)
	}
	if n := c.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'advance_stage'
		AND actor_id = $2 AND on_behalf_of = $3`, c.silent, marginceActorID, c.Rep1); n != 1 {
		t.Errorf("close audit rows by Margince for the owner = %d, want 1", n)
	}
	if n := c.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1
		AND correction = 'close_lost' AND stage_history_id IS NOT NULL`, c.silent); n != 1 {
		t.Errorf("close corrections naming their stage move = %d, want 1", n)
	}
	if n := c.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.kind = 'note' AND a.subject = 'Margince closed this deal as lost'`, c.silent); n != 1 {
		t.Errorf("timeline notes about the close = %d, want 1", n)
	}

	// A day and more later the owner still finds it on Home, with its Undo.
	receipts := attentionReceipts{svc: approvals.NewService(InstallationDB(c.Pool)), deals: c.Deals}
	recent, err := receipts.Recent(c.owner(t), c.now.Add(48*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Kind != deals.DealCloseLostKind || recent[0].Undo == nil ||
		!strings.Contains(recent[0].Summary, "No word from the customer for 196 days") {
		t.Fatalf("Home receipts two days on = %+v, want the close with its reason and Undo", recent)
	}

	c.undo(t, c.silent)
	if got := c.status(t, c.silent); got != string(deals.DealOpen) {
		t.Fatalf("after Undo the deal is %s, want open", got)
	}
	if got := c.WsScalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, c.silent); got != stageBefore {
		t.Errorf("after Undo the deal is in stage %s, want the stage it was closed from %s", got, stageBefore)
	}
	if n := c.WsCount(t, `SELECT count(*) FROM deal_stage_history WHERE deal_id = $1 AND reversal_of IS NOT NULL`, c.silent); n != 1 {
		t.Errorf("reopening moves that name the close they reverse = %d, want 1", n)
	}
}

func TestAnUndoWithoutANoteIsClosedAgainAtTheNextCheck(t *testing.T) {
	c := setupCloseLost(t)
	c.run(t)
	c.undo(t, c.silent)

	if got := c.run(t); got != 1 {
		t.Fatalf("closed %d deals after an Undo with no note, want 1", got)
	}
	if got := c.status(t, c.silent); got != string(deals.DealLost) {
		t.Fatalf("the deal is %s, want lost again", got)
	}
}

func TestANoteAfterTheUndoKeepsTheDealOpen(t *testing.T) {
	c := setupCloseLost(t)
	c.run(t)
	c.undo(t, c.silent)
	body := "Their budget round is in March; they asked us to keep it open."
	if _, _, err := c.Activities.LogActivity(c.owner(t), activities.LogActivityInput{
		Kind: "note", Body: &body, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: c.silent}},
	}); err != nil {
		t.Fatal(err)
	}

	if got := c.run(t); got != 0 {
		t.Fatalf("closed %d deals, want 0: the owner's note after the Undo says the deal is alive", got)
	}
	if got := c.status(t, c.silent); got != string(deals.DealOpen) {
		t.Fatalf("the deal is %s, want open", got)
	}
}

func TestTheOwnersSwitchStopsTheClose(t *testing.T) {
	c := setupCloseLost(t)
	if _, err := approvals.NewService(InstallationDB(c.Pool)).SetAutoApply(c.owner(t), deals.DealCloseLostKind, false); err != nil {
		t.Fatal(err)
	}

	if got := c.run(t); got != 0 {
		t.Fatalf("closed %d deals with the owner's switch off, want 0", got)
	}
}

// The check decides to close a deal, and closes it in a later transaction. An
// owner who leaves, loses the deal grant or turns the switch off in between
// keeps the deal open.
func TestAnOwnerWhoLeavesBeforeTheSilentCloseWritesKeepsTheDeal(t *testing.T) {
	for name, change := range map[string]func(c *closeLostEnv, t *testing.T){
		"nothing changes": func(*closeLostEnv, *testing.T) {},
		"suspended": func(c *closeLostEnv, t *testing.T) {
			c.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, c.Rep1)
		},
		"role removed": func(c *closeLostEnv, t *testing.T) {
			c.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, c.Rep1)
		},
		"switch turned off": func(c *closeLostEnv, t *testing.T) {
			if _, err := approvals.NewService(InstallationDB(c.Pool)).SetAutoApply(c.owner(t), deals.DealCloseLostKind, false); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := setupCloseLost(t)
			ctx := principal.SystemActing(c.Admin(), closeDateSweepActor)
			var due []closeDecision
			if err := database.WithWorkspaceTx(ctx, c.Pool, func(tx pgx.Tx) error {
				var err error
				due, err = closeDecisions(ctx, tx, c.now, nil)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if len(due) != 1 || due[0].deal.UUID != c.silent {
				t.Fatalf("due = %+v, want only the silent deal", due)
			}
			change(c, t)

			closed, err := c.closer.closeOne(ctx, due[0], c.now)
			if err != nil {
				t.Fatal(err)
			}
			want, wantStatus := false, deals.DealOpen
			if name == "nothing changes" {
				want, wantStatus = true, deals.DealLost
			}
			if closed != want || c.status(t, c.silent) != string(wantStatus) {
				t.Fatalf("closed = %v, deal is %s: want closed = %v, deal %s",
					closed, c.status(t, c.silent), want, wantStatus)
			}
		})
	}
}

func TestADealOnLegalHoldIsNotClosed(t *testing.T) {
	c := setupCloseLost(t)
	c.WsExec(t, `UPDATE deal SET legal_hold = true WHERE id = $1`, c.silent)

	if got := c.run(t); got != 0 {
		t.Fatalf("closed %d deals, want 0: the silent deal is on legal hold", got)
	}
}

func TestARecentReplyOnlyItsParticipantsSeeKeepsTheDealOpen(t *testing.T) {
	c := setupCloseLost(t)
	recent := c.now.AddDate(0, 0, -10)
	c.customerWrites(t, c.silent, recent)
	c.WsExec(t, `UPDATE activity SET audience = 'participants'
		WHERE occurred_at = $1 AND id IN (SELECT activity_id FROM activity_link WHERE deal_id = $2)`, recent, c.silent)

	if got := c.run(t); got != 0 {
		t.Fatalf("closed %d deals, want 0: the customer wrote ten days ago in a private thread", got)
	}
}

// refuses marks the buyer's message on the deal as a refusal.
func (c *closeLostEnv) refuses(t *testing.T, deal ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	mail := c.customerMails(t, deal, at)
	if applied, err := activities.NewStore(c.DB()).SetReplyVerdict(c.Admin(), mail,
		activities.ReplyVerdictNegative, "test"); err != nil || !applied {
		t.Fatalf("judging the reply: applied=%v err=%v", applied, err)
	}
	return mail
}

func TestARefusalClosesTheDealAtOnce(t *testing.T) {
	c := setupCloseLost(t)
	c.refuses(t, c.supported, c.now.AddDate(0, 0, -2))

	c.run(t)
	if got := c.status(t, c.supported); got != string(deals.DealLost) {
		t.Fatalf("the deal the customer turned down two days ago is %s, want lost", got)
	}
	if got := c.WsScalar(t, `SELECT lost_reason_code FROM deal WHERE id = $1`, c.supported); got != deals.LossReasonOther {
		t.Errorf("lost reason code = %q, want %q", got, deals.LossReasonOther)
	}
	if got := c.WsScalar(t, `SELECT evidence->>'reason_kind' FROM audit_log WHERE entity_type='deal' AND entity_id=$1 AND evidence->>'margince_change'='close_lost' ORDER BY occurred_at DESC LIMIT 1`, c.supported); got != "inference" {
		t.Fatalf("model-read refusal reason kind = %q, want inference", got)
	}
}

func TestARefusalWithAWaitAheadIsNotClosed(t *testing.T) {
	c := setupCloseLost(t)
	c.refuses(t, c.supported, c.now.AddDate(0, 0, -2))
	until := c.now.AddDate(0, 1, 0)
	if _, err := c.Deals.UpdateDeal(c.Admin(), ids.From[ids.DealKind](c.supported), deals.UpdateDealInput{WaitUntil: &until}); err != nil {
		t.Fatal(err)
	}

	c.run(t)
	if got := c.status(t, c.supported); got != string(deals.DealOpen) {
		t.Fatalf("the deal is %s, want open: the customer asked to wait until next month", got)
	}
}

func TestARefusalWithAnAcceptedMeetingAheadIsNotClosed(t *testing.T) {
	c := setupCloseLost(t)
	c.refuses(t, c.supported, c.now.AddDate(0, 0, -2))
	subject, at := "Contract walk-through", c.now.AddDate(0, 0, 10)
	meeting, _, err := c.Activities.LogActivity(c.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "deal", EntityID: c.supported}, {EntityType: "contact", EntityID: c.buyer},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.WsExec(t, `UPDATE activity_participant SET response = 'accepted' WHERE activity_id = $1 AND contact_id = $2`,
		ids.UUID(meeting.Id), c.buyer)
	if n := c.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND response = 'accepted'`,
		ids.UUID(meeting.Id)); n != 1 {
		t.Fatalf("accepted guests on the meeting = %d, want 1", n)
	}

	c.run(t)
	if got := c.status(t, c.supported); got != string(deals.DealOpen) {
		t.Fatalf("the deal is %s, want open: the buyer accepted a meeting next week", got)
	}
}

func TestACloseDecidedOnPrivateMailNamesOnlyTheThreshold(t *testing.T) {
	c := setupCloseLost(t)
	private := c.now.AddDate(0, 0, -121)
	c.customerWrites(t, c.silent, private)
	c.WsExec(t, `UPDATE activity SET audience = 'participants'
		WHERE occurred_at = $1 AND id IN (SELECT activity_id FROM activity_link WHERE deal_id = $2)`, private, c.silent)

	if got := c.run(t); got != 1 {
		t.Fatalf("closed %d deals, want 1: 121 days of silence is over the threshold", got)
	}
	// The private reply set the count, so the text every reader sees names the
	// threshold and neither 121 nor the 196 the visible mail alone would give.
	reason := c.WsScalar(t, `SELECT lost_reason FROM deal WHERE id = $1`, c.silent)
	if !strings.Contains(reason, "more than 120 days") || strings.Contains(reason, "121") || strings.Contains(reason, "196") {
		t.Fatalf("lost reason = %q, want the threshold and no day the private reply set", reason)
	}
	note := c.WsScalar(t, `SELECT a.body FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.subject = 'Margince closed this deal as lost'`, c.silent)
	if !strings.Contains(note, "more than 120 days") || strings.Contains(note, "121") {
		t.Fatalf("timeline note = %q, want the threshold and no day the private reply set", note)
	}
}

func TestAPrivateThreadThroughABrokenMailboxKeepsTheDealOpen(t *testing.T) {
	c := setupCloseLost(t)
	owner := integration.OwnerConn(t)
	integration.SeedIDRow(t, owner, `
		INSERT INTO capture_connection (id, provider, user_id, status)
		VALUES ($1, 'imap', $2, 'reauth_required')`, c.Rep2)
	thread := c.customerMails(t, c.silent, c.now.AddDate(0, 0, -150))
	c.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, thread)
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, thread, c.Rep2); err != nil {
		t.Fatal(err)
	}

	if got := c.run(t); got != 0 {
		t.Fatalf("closed %d deals, want 0: a colleague's mailbox that carried the deal's private mail is broken", got)
	}
}

func TestAnUndoAfterSomebodyMovedTheDealIsRefused(t *testing.T) {
	c := setupCloseLost(t)
	before := c.WsScalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, c.silent)
	c.run(t)
	stage, err := ids.Parse(before)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Deals.AdvanceDeal(c.owner(t), ids.From[ids.DealKind](c.silent), deals.AdvanceDealInput{
		ToStageID: ids.StageID{UUID: stage}, WonWithoutContractReason: nil,
	}); err != nil {
		t.Fatal(err)
	}
	var correction ids.UUID
	if err := database.WithWorkspaceTx(c.Admin(), c.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(c.Admin(), `SELECT id FROM deal_correction WHERE deal_id = $1 AND correction = 'close_lost'`,
			c.silent).Scan(&correction)
	}); err != nil {
		t.Fatal(err)
	}

	_, err = c.Deals.RevertCorrection(c.owner(t), correction, nil, nil)
	refusal, ok := errors.AsType[*deals.CorrectionReversalError](err)
	if !ok || !strings.Contains(refusal.Reason, "moved since Margince closed it") {
		t.Fatalf("Undo after the owner reopened the deal by hand = %v, want the moved-since refusal", err)
	}
}

// A close older than the Home window still reaches Home as a change Margince
// made. It never reads as a decided approval, whose line offers no Undo.
func TestHomeShowsAnOlderCloseAsAChangeWithItsWayBack(t *testing.T) {
	c := setupCloseLost(t)
	c.run(t)
	audit := c.WsScalar(t, `SELECT audit_log_id::text FROM deal_correction WHERE deal_id = $1`, c.silent)
	readAt := c.now.Add(50 * time.Hour)
	brief := fixedBrief{at: c.now.Add(48 * time.Hour)}
	staged := approvals.NewService(InstallationDB(c.Pool))
	lane, err := magic.NewService(c.Pool, brief, func() time.Time { return readAt }).
		WithDecidedReceipts(magicDecidedReceipts{receipts: attentionReceipts{svc: staged, deals: c.Deals}}).
		Read(c.owner(t), nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, line := range lane.Done {
		if line.ReceiptIds == nil || !slices.ContainsFunc(*line.ReceiptIds, func(id openapi_types.UUID) bool {
			return ids.UUID(id).String() == audit
		}) {
			continue
		}
		found++
		if line.ApprovalKind != nil {
			t.Errorf("the close reads as a decided %q approval on Home, want a change with its Undo", *line.ApprovalKind)
		}
	}
	if found != 1 {
		t.Fatalf("Home lines for the close two days on = %d, want 1", found)
	}
}

// fixedBrief is a last brief at one instant: the reader's own state, which
// is what the window depends on.
type fixedBrief struct{ at time.Time }

func (b fixedBrief) CutoffFor(context.Context) (time.Time, bool, error) { return b.at, true, nil }
