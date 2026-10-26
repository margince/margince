// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Every pending domain question has one decider: the sole evidenced direct
// recipient of the newest current staff mail the reader may read. The vendor
// question used it alone. Every other reason went to each seat with a reader
// row, so who was asked depended on whose mailbox received the mail.
//
// The question is raised natively (capture, then a triage read). The other
// reasons are then set on the same stored row. Their writers are triage paths
// with their own tests. This test owns who is asked once a row carries one. Each seat that received the mail also holds the reader
// row the old routing read.
func TestEveryPendingDomainReasonAsksOnlyItsDecider(t *testing.T) {
	reasons := []string{
		contacts.PendingUnevidenced, contacts.PendingStaleEvidence,
		contacts.PendingNearDuplicate, contacts.PendingForwardedIdentity,
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			e := integration.Setup(t)
			now := pinClock(t, e)
			rep1, rep2 := e.As(e.Rep1, nil, integration.AdminPerms), e.As(e.Rep2, nil, integration.AdminPerms)
			raise := func(domain string, to []ids.UUID, age time.Duration) {
				t.Helper()
				var addressed []string
				for _, seat := range to {
					addressed = append(addressed, seatEmail(t, seat))
				}
				mail := staffMail{sourceID: domain, from: "contact@" + domain, mailbox: e.Rep1,
					to: addressed, cc: []string{seatEmail(t, e.Rep2)}, occurred: now.Add(-age)}
				captureStaffMail(t, e, mail)
				mail.mailbox = e.Rep2
				captureStaffMail(t, e, mail)
				runVendorTriage(t, e, domain)
				e.WsExec(t, `UPDATE company_domain_disposition SET pending_reason = $2 WHERE domain = $1`, domain, reason)
				for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
					e.WsExec(t, `INSERT INTO company_domain_question_reader (domain, reader_id, evidence_at)
						VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, domain, seat, now.Add(-age))
				}
			}

			raise("sole.example", []ids.UUID{e.Rep1}, time.Hour)
			if !asksAbout(rep1, t, e, "sole.example") {
				t.Error("the sole direct recipient was not asked")
			}
			if asksAbout(rep2, t, e, "sole.example") {
				t.Error("a copied seat whose mailbox also received the mail was asked")
			}
			if code := answerDomain(rep2, e, "keep", "sole.example"); code != http.StatusNotFound {
				t.Errorf("the copied seat answering = %d, want 404", code)
			}

			for _, tc := range []struct {
				name, domain string
				to           []ids.UUID
				age          time.Duration
			}{
				{"two addressed seats", "two.example", []ids.UUID{e.Rep1, e.Rep2}, time.Hour},
				{"mail older than the horizon", "old.example", []ids.UUID{e.Rep1}, 31 * 24 * time.Hour},
			} {
				raise(tc.domain, tc.to, tc.age)
				if asksAbout(rep1, t, e, tc.domain) || asksAbout(rep2, t, e, tc.domain) {
					t.Errorf("%s: a seat was asked, want nobody", tc.name)
				}
			}
			for _, domain := range []string{"sole.example", "two.example", "old.example"} {
				if !onRefusedList(t, e, domain) {
					t.Errorf("%s is not reachable on the refused domains list", domain)
				}
			}
		})
	}
}
