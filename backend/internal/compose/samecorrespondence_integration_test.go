// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sharedKeyThreads captures alphaThread and, between our answer and the
// customer's confirmation, one mail from orders@beta.test on the same key.
func sharedKeyThreads(t *testing.T, o *owedEnv, betaCc string) (first, ours, beta, confirms ids.UUID) {
	t.Helper()
	first = o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, alphaAsks, 0, true))
	ours = o.capture(t, pricingMail(o.seat, "buyer@alpha.test", "pricing-ours@ws.example", ourAnswer, 30, false))
	order := pricingMail("orders@beta.test", o.seat, "order-1@beta.test", betaWrites, 45, true)
	order.cc = betaCc
	beta = o.capture(t, order)
	confirms = o.capture(t, pricingMail("buyer@alpha.test", o.seat, "pricing-2@alpha.test", alphaConfirms, 60, true))
	requireOneThreadKey(t, o, confirms, 4)
	return first, ours, beta, confirms
}

func TestMovingAConversationLeavesAnotherCorrespondenceOnItsKeyAlone(t *testing.T) {
	o := setupOwed(t)
	first, ours, beta, confirms := sharedKeyThreads(t, o, "")
	project := ids.NewV7()
	company := o.e.SeedCompany(t, "Alpha AG", nil)
	o.e.WsExec(t, `INSERT INTO project (id, name, key, company_id, phase, source, captured_by)
		VALUES ($1, 'Pricing', 'PRC1', $2, 'delivering', 'manual', 'human:x')`, project, company)

	mover := o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.AdminPerms)
	out, err := o.e.Activities.RelinkThread(mover, ids.From[ids.ActivityKind](confirms),
		activities.RelinkActivityInput{EntityType: "project", EntityID: project})
	if err != nil {
		t.Fatalf("moving the conversation: %v", err)
	}
	filed := func(id ids.UUID) bool {
		return o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND project_id = $2`, id, project) == 1
	}
	for _, id := range []ids.UUID{first, ours, confirms} {
		if !filed(id) {
			t.Errorf("message %s of the conversation was not moved", id)
		}
	}
	if filed(beta) {
		t.Error("another correspondent's mail on the same thread key moved with the conversation")
	}
	if out.Relinked != 3 {
		t.Errorf("relinked = %d, want the conversation's three messages", out.Relinked)
	}
}

func TestTheExtractionWindowReadsOneCorrespondence(t *testing.T) {
	for name, resolved := range map[string][]string{
		"both senders are contacts":   {"buyer@alpha.test", "orders@beta.test"},
		"only the customer is known":  {"buyer@alpha.test"},
		"neither sender is a contact": nil,
	} {
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			for _, address := range resolved {
				if _, err := o.e.Contacts.CreateContact(o.e.Admin(), contacts.CreateContactInput{
					FullName: address, Source: "manual",
					Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
				}); err != nil {
					t.Fatal(err)
				}
			}
			_, _, _, confirms := sharedKeyThreads(t, o, "")
			key := o.e.WsScalar(t, `SELECT thread_key FROM activity WHERE id = $1`, confirms)
			var bodies []string
			read := func(text string) bool {
				return slices.ContainsFunc(bodies, func(body string) bool { return strings.Contains(body, text) })
			}
			as := o.e.Admin()
			if err := database.WithWorkspaceTx(as, o.e.Pool, func(tx pgx.Tx) error {
				window, err := threadMessages(as, tx, &settledThread{Key: key})
				for _, message := range window {
					bodies = append(bodies, message.Body)
				}
				return err
			}); err != nil {
				t.Fatalf("reading the window: %v", err)
			}
			if read(betaWrites) {
				t.Errorf("another correspondent's mail reached the extraction window: %q", bodies)
			}
			for _, want := range []string{alphaAsks, ourAnswer, alphaConfirms} {
				if !read(want) {
					t.Errorf("the window lost %q of the conversation: %q", want, bodies)
				}
			}
		})
	}
}
