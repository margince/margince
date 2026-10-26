// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A held meeting proves the buyer attended only when an attendee is outside
// by the rule capture used to keep the meeting. The domains come from the
// workspace tables capture reads.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldMeeting writes a held meeting on the deal as calendar capture stores it:
// the syncing seat as its From participant, then the attendees.
func (f *authorshipFixture) heldMeeting(seat string, attendees ...string) ids.UUID {
	f.t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `INSERT INTO activity (kind, subject, meeting_status, occurred_at, source, captured_by)
			VALUES ('meeting', 'Review', 'held', $1, 'gcal:seed', 'connector:gcal') RETURNING id`,
			clocktest.Now(f.t).Add(-time.Hour)).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, user_id, address, address_domain_folded, role)
			VALUES ($1, $2, $3, $4, 'from')`, id, f.AdminUser, seat, owndomain.StoredOfAddress(seat)); err != nil {
			return err
		}
		for _, a := range attendees {
			if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, address_domain_folded, role)
				VALUES ($1, $2, $3, 'attendee')`, id, a, owndomain.StoredOfAddress(a)); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, id, f.dealID.UUID)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// buyers is how many attendees the evidence writers count as the buyer.
func (f *authorshipFixture) buyers(meeting ids.UUID) int {
	f.t.Helper()
	var n int
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		facts, err := deals.ReadActivityAuthorship(f.Admin(), tx, meeting, ownDomainReader{store: f.own})
		n = facts.BuyerParticipants
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *authorshipFixture) wantBuyers(name string, meeting ids.UUID, want int) {
	f.t.Helper()
	if got := f.buyers(meeting); got != want {
		f.t.Errorf("%s: %d attendees counted as the buyer, want %d", name, got, want)
	}
	proof := deals.MeetingWasHeld("held", false, f.buyers(meeting))
	if proof.Held != (want > 0) {
		f.t.Errorf("%s: the meeting proves it was held = %v, want %v", name, proof.Held, want > 0)
	}
}

func TestAMeetingWithNoConfirmedOwnDomainCountsNoColleagueAsTheBuyer(t *testing.T) {
	f := newAuthorshipFixture(t)
	if domains, err := f.own.Colleagues(f.Admin()); err != nil || !domains.Empty() {
		t.Fatalf("the workspace starts with confirmed own domains %v (err %v); this test needs none", domains.Domains(), err)
	}
	f.wantBuyers("colleagues only", f.heldMeeting("rep@ourco.test", "colleague@ourco.test"), 0)
	f.wantBuyers("a real buyer", f.heldMeeting("rep@ourco.test", "colleague@ourco.test", "ines@customer.test"), 1)
	f.wantBuyers("a consumer seat domain", f.heldMeeting("rep@gmail.com", "friend@gmail.com"), 1)
	f.wantBuyers("an unknown guest", f.heldMeeting("rep@ourco.test", "guest"), 0)
}

func TestAMeetingOnACandidateOwnDomainCountsNoColleagueAsTheBuyer(t *testing.T) {
	f := newAuthorshipFixture(t)
	f.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ('ourco.test', 'admin', true)`)
	f.WsExec(t, `INSERT INTO workspace_email_domain (domain) VALUES ('labs.test')`)
	f.wantBuyers("a candidate seat domain", f.heldMeeting("rep@labs.test", "dev@labs.test"), 0)
	f.wantBuyers("a confirmed domain", f.heldMeeting("rep@labs.test", "colleague@ourco.test"), 0)
	f.wantBuyers("a candidate domain the seat is not on", f.heldMeeting("rep@ourco.test", "dev@labs.test"), 1)
	f.wantBuyers("a real buyer", f.heldMeeting("rep@ourco.test", "ines@customer.test"), 1)
}
