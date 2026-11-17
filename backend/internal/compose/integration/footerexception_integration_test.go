// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"net/http"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

var footerExceptionCountry = sync.OnceValue(func() string {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zj", Version: 1, Disclosures: []messagingrules.Disclosure{{Kind: messagingrules.ObjectionRoute, MarketingOnly: true}}})
	return "zj"
})

func TestARepSendsWithMissingEvidenceAndFooterDetails(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	c := &consentEnv{AppEnv: p.AppEnv, contactID: p.contactID, activityID: p.activityID}
	if _, err := c.Owner.Exec(t.Context(), `UPDATE contact SET address_country=@country WHERE id=@contact`, pgx.StrictNamedArgs{"contact": c.contactID, "country": footerExceptionCountry()}); err != nil {
		t.Fatal(err)
	}
	demoteToRep(t, c.AppEnv)
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{"subject": "Requested information", "body": "The requested information.", "to": []string{"buyer@preflight.test"}, "consent_purpose": "marketing_email"}, nil, nil); status != http.StatusConflict {
		t.Fatalf("combined warning send = %d, want a review instead of a footer error", status)
	}
	reviewID := liveReviewID(t, c)
	var review struct {
		AssessmentRevision string `json:"assessment_revision"`
		Refusals           []struct {
			FooterFindings []commsauthz.FooterFinding `json:"footer_findings"`
		} `json:"refusals"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+reviewID, nil, nil, &review); status != http.StatusOK {
		t.Fatalf("read review = %d", status)
	}
	if len(review.Refusals) == 0 || len(review.Refusals[0].FooterFindings) == 0 {
		t.Fatalf("missing footer warnings were not shown: %+v", review)
	}
	confirmation := directed()
	confirmation["assessment_revision"] = review.AssessmentRevision
	var sent struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+reviewID+"/direct-send", confirmation, nil, &sent); status != http.StatusCreated {
		t.Fatalf("SDR confirmation = %d, want 201", status)
	}
	activity, err := ids.Parse(sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	delivery, _ := p.deliveryFor(t, activity)
	p.transmit(t, delivery, "")
	var deliveries int
	if err := c.Owner.QueryRow(t.Context(), `SELECT count(*) FROM comms_outbound WHERE execution_authority='instruction'`).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if deliveries != 1 {
		t.Fatalf("deliveries = %d, want one instructed send", deliveries)
	}
}

var footerOnlyCountry = sync.OnceValue(func() string {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zk", Version: 1, Disclosures: []messagingrules.Disclosure{{Kind: messagingrules.ObjectionRoute}}})
	return "zk"
})

func TestAFooterOnlyExceptionReachesTheProvider(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	if _, err := p.Owner.Exec(t.Context(), `UPDATE contact SET address_country=@country WHERE id=@contact`, pgx.StrictNamedArgs{"contact": p.contactID, "country": footerOnlyCountry()}); err != nil {
		t.Fatal(err)
	}
	demoteToRep(t, p.AppEnv)
	if status, _, _ := p.send(t); status != http.StatusConflict {
		t.Fatalf("footer-only send = %d, want review", status)
	}
	c := &consentEnv{AppEnv: p.AppEnv, contactID: p.contactID, activityID: p.activityID}
	reviewID := liveReviewID(t, c)
	var review struct {
		AssessmentRevision string `json:"assessment_revision"`
		ReasonCode         string `json:"reason_code"`
	}
	if status := p.Call(t, "GET", "/v1/communication-reviews/"+reviewID, nil, nil, &review); status != http.StatusOK {
		t.Fatalf("review = %d", status)
	}
	if review.ReasonCode != commsauthz.ReasonFooterIncomplete {
		t.Fatalf("reason = %q, want footer-only review", review.ReasonCode)
	}
	confirmation := directedShowing(review.ReasonCode)
	confirmation["assessment_revision"] = review.AssessmentRevision
	var sent struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", "/v1/communication-reviews/"+reviewID+"/direct-send", confirmation, nil, &sent); status != http.StatusCreated {
		t.Fatalf("confirm = %d", status)
	}
	activity, err := ids.Parse(sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	delivery, _ := p.deliveryFor(t, activity)
	p.transmit(t, delivery, "")
}

var changingFooterCountry = sync.OnceValue(func() string {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zm", Version: 1, Disclosures: []messagingrules.Disclosure{{Kind: messagingrules.ObjectionRoute, MarketingOnly: true}, {Kind: messagingrules.AdvertiserContact, MarketingOnly: true}}})
	return "zm"
})

func TestAChangedFooterRenewsTheReviewBeforeRecordingTheDecision(t *testing.T) {
	c := setupConsent(t)
	if _, err := c.Owner.Exec(t.Context(), `UPDATE contact SET address_country=@country WHERE id=@contact`, pgx.StrictNamedArgs{"contact": c.contactID, "country": changingFooterCountry()}); err != nil {
		t.Fatal(err)
	}
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("send = %d", status)
	}
	oldID := liveReviewID(t, c)
	var original struct {
		AssessmentRevision string `json:"assessment_revision"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+oldID, nil, nil, &original); status != http.StatusOK {
		t.Fatalf("review = %d", status)
	}
	if status := c.Call(t, "PUT", "/v1/privacy/controller-particulars", AnyMap{"objection_route": "Reply STOP."}, nil, nil); status != http.StatusOK {
		t.Fatalf("update footer = %d", status)
	}
	confirmation := directed()
	confirmation["assessment_revision"] = original.AssessmentRevision
	var changed struct {
		Details struct {
			ReviewID string `json:"review_id"`
		} `json:"details"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+oldID+"/direct-send", confirmation, nil, &changed); status != http.StatusConflict {
		t.Fatalf("changed confirmation = %d, want new review", status)
	}
	if changed.Details.ReviewID == "" || changed.Details.ReviewID == oldID {
		t.Fatalf("no fresh assessment: %+v", changed)
	}
	var count int
	if err := c.Owner.QueryRow(t.Context(), `SELECT count(*) FROM communication_instruction`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("a decision was recorded against warnings the sender did not read")
	}
	var current struct {
		AssessmentRevision string `json:"assessment_revision"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+changed.Details.ReviewID, nil, nil, &current); status != http.StatusOK {
		t.Fatalf("fresh review = %d", status)
	}
	confirmation["assessment_revision"] = current.AssessmentRevision
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+changed.Details.ReviewID+"/direct-send", confirmation, nil, nil); status != http.StatusCreated {
		t.Fatalf("renewed confirmation = %d", status)
	}
}

func TestARepConfirmsAMissingPublicLinkAndTheProviderReceivesTheEmail(t *testing.T) {
	p := setupPreflight(t, compose.WithPublicBaseURL(""))
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	demoteToRep(t, p.AppEnv)
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Product news", "body": "The latest news.", "to": []string{"buyer@preflight.test"}, "consent_purpose": "marketing_email",
	}, nil, nil); status != http.StatusConflict {
		t.Fatalf("send = %d, want review", status)
	}
	c := &consentEnv{AppEnv: p.AppEnv, contactID: p.contactID, activityID: p.activityID}
	reviewID := liveReviewID(t, c)
	var review struct {
		AssessmentRevision string `json:"assessment_revision"`
		ReasonCode         string `json:"reason_code"`
		Refusals           []struct {
			FooterFindings []commsauthz.FooterFinding `json:"footer_findings"`
		} `json:"refusals"`
	}
	if status := p.Call(t, "GET", "/v1/communication-reviews/"+reviewID, nil, nil, &review); status != http.StatusOK {
		t.Fatalf("review = %d", status)
	}
	found := false
	for _, refusal := range review.Refusals {
		for _, finding := range refusal.FooterFindings {
			found = found || finding.Kind == "public_opt_out_link"
		}
	}
	if !found {
		t.Fatal("the omitted public link was not shown")
	}
	confirmation := directedShowing(review.ReasonCode)
	confirmation["assessment_revision"] = review.AssessmentRevision
	var sent struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", "/v1/communication-reviews/"+reviewID+"/direct-send", confirmation, nil, &sent); status != http.StatusCreated {
		t.Fatalf("confirmation = %d", status)
	}
	activity, err := ids.Parse(sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	delivery, _ := p.deliveryFor(t, activity)
	p.transmit(t, delivery, "")
}
