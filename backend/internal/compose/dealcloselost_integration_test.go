// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Margince closes a deal the customer stopped answering, as its owner's agent,
// and the owner can undo it from Home. These tests drive the real closer over
// deals written through the deals and activities stores.

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

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
