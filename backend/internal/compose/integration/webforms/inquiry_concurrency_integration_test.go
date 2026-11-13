// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestConcurrentInquiryAssignmentsRetryStaleRoutingCounts(t *testing.T) {
	e := setupFormsApp(t)
	var colleague ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT id FROM app_user WHERE email='ada@example.com'`).Scan(&colleague); err != nil {
		t.Fatal(err)
	}
	admit := inquirySeatContext(t, e, colleague)
	policy, err := automation.NewAutomationStore(e.DB()).Create(admit, automation.CreateAutomationInput{Key: "route_lead", Name: "Inquiry concurrency test", Params: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(admit, "system:inquiry-test")
	claims := automation.NewEffectClaims(e.DB())
	router := contacts.NewStore(e.DB())
	cfg := contacts.RoutingConfig{Owners: []ids.UserID{ids.From[ids.UserKind](colleague)}, CapPerOwner: 1}
	ready, release := make(chan struct{}, 2), make(chan struct{}, 2)
	defer close(release)
	results := make(chan error, 2)
	for _, email := range []string{"concurrent-a@visitor.example", "concurrent-b@visitor.example"} {
		form := createForm(t, e, integration.AnyMap{"name": email, "sender": "browser", "allowed_origins": []string{siteOrigin}})
		if answer := browserPost(t, e, form.FormKey, url.Values{"email": {email}, "message": {"Please call"}}, siteOrigin); answer.Status != http.StatusAccepted {
			t.Fatalf("submission: %d", answer.Status)
		}
		event := inquiryEvent(t, e, email, "activity.captured")
		var lead ids.UUID
		var key string
		if err := e.Owner.QueryRow(t.Context(), `SELECT lead.id, activity.source_id FROM lead JOIN activity_link ON activity_link.lead_id=lead.id JOIN activity ON activity.id=activity_link.activity_id WHERE activity.id=@note`, pgx.NamedArgs{"note": event.Entity.ID}).Scan(&lead, &key); err != nil {
			t.Fatal(err)
		}
		go func() {
			first := true
			_, err := claims.FreezeInquiry(system, key, func(bound context.Context) (automation.InquiryPlan, error) {
				if first {
					first = false
					ready <- struct{}{}
					select {
					case <-release:
					case <-bound.Done():
						return automation.InquiryPlan{}, bound.Err()
					}
				}
				decision, err := router.RouteLeadAs(bound, admit, ids.From[ids.LeadKind](lead), cfg)
				if err != nil {
					return automation.InquiryPlan{}, err
				}
				return automation.InquiryPlan{PolicyID: policy.ID.UUID, PolicyVersion: 1, SourceActivityID: event.Entity.ID, Target: datasource.EntityRef{Type: datasource.EntityLead, ID: lead}, Recipient: colleague, DueAt: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), Reason: decision.Reason}, nil
			})
			results <- err
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case err := <-results:
			t.Fatalf("assignment ended before both snapshots were ready: %v", err)
		}
	}
	release <- struct{}{}
	release <- struct{}{}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var assigned int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM lead WHERE owner_id=@owner`, pgx.NamedArgs{"owner": colleague}).Scan(&assigned); err != nil {
		t.Fatal(err)
	}
	if assigned != 1 {
		t.Fatalf("concurrent snapshots assigned %d leads under a one-lead cap, want one", assigned)
	}
}
