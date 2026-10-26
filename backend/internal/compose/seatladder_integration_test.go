// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	colleaguePrivate = "rep2.private@home.example"
	partnerBuyer     = "buyer@partner.example"
)

// captureColleagueThread lands, in Rep1's mailbox, a mail a colleague wrote
// from a private address to a partner's buyer, with Rep1 in Cc.
func captureColleagueThread(t *testing.T, e *integration.Env, sourceID string) ids.UUID {
	t.Helper()
	ref, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(mailboxOwnerCtx(e, e.Rep1), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: sourceID},
		Counterparty: connector.Counterparty{
			Email: colleaguePrivate, DisplayName: "Rep Two", Domain: "home.example", Direction: connector.DirectionInbound,
		},
		Fields:    capture.ActivityFields{Kind: "email", Subject: "Angebot", Body: "Anbei.", Direction: connector.DirectionInbound},
		ThreadKey: "thread-" + sourceID,
		Addresses: []string{colleaguePrivate, partnerBuyer, "a@authz.test"},
		Source:    "gmail:" + sourceID, CapturedBy: "connector:gmail",
	})
	if err != nil {
		t.Fatalf("capturing %s: %v", sourceID, err)
	}
	return ref.ID
}

// The colleague's private address is a seat's. Capture never makes it the
// subject of the mail, an outside party or a contact. The buyer stands in as
// the subject.
func TestACapturedColleaguesPrivateAddressIsNeverTheOutsideParty(t *testing.T) {
	e := setupWithOwnMailbox(t)
	connectSeatMailbox(t, e, e.Rep2, colleaguePrivate, "shared")

	mail := captureColleagueThread(t, e, "seat-ladder-1")

	if n := contactsAt(t, e, colleaguePrivate); n != 0 {
		t.Errorf("%d contacts for a colleague's private address, want 0", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM capture_pending_counterparty WHERE email = $1`, colleaguePrivate); n != 0 {
		t.Errorf("the ladder asked %d question(s) about a colleague's address, want none", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = $2`, mail, colleaguePrivate); n != 0 {
		t.Errorf("the colleague's address was stamped %d time(s) as an outside party, want 0", n)
	}
	subject := e.WsCount(t, `SELECT count(*) FROM capture_pending_counterparty WHERE email = $1`, partnerBuyer) +
		contactsAt(t, e, partnerBuyer)
	if subject == 0 {
		t.Error("the buyer did not stand in as the party the mail is about")
	}
}
