// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// Two packs that fix different advertising labels at the start of a subject.
var subjectLabelCountries = sync.OnceValues(func() (string, string) {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zl", Version: 1, SubjectPrefix: "(광고)"})
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zn", Version: 1, SubjectPrefix: "<ADV>"})
	return "zl", "zn"
})

// labelReview is what a refused Send leaves for the sender to read.
type labelReview struct {
	ReasonCode string         `json:"reason_code"`
	Refusals   []labelRefusal `json:"refusals"`
}

type labelRefusal struct {
	ReasonCode string `json:"reason_code"`
}

func (c *consentEnv) placeSubjectIn(t *testing.T, contactID, country string) {
	t.Helper()
	if _, err := c.Owner.Exec(t.Context(), `UPDATE contact SET address_country=@country WHERE id=@contact`,
		pgx.StrictNamedArgs{"contact": contactID, "country": country}); err != nil {
		t.Fatal(err)
	}
}

func (c *consentEnv) sendMarketingTo(t *testing.T, to ...string) int {
	t.Helper()
	return c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"subject": "Our offer", "body": "Half price this week.", "to": to, "consent_purpose": "marketing_email",
	}, nil, nil)
}

// refusedAtSend asserts the Send was refused inline: a 409, no delivery, and a
// review whose every refusal is want.
func (c *consentEnv) refusedAtSend(t *testing.T, status int, want string) {
	t.Helper()
	if status != http.StatusConflict {
		t.Fatalf("send = %d, want 409 so the draft stays in the composer", status)
	}
	var deliveries int
	if err := c.Owner.QueryRow(t.Context(), `SELECT count(*) FROM comms_outbound`).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 {
		t.Fatalf("%d deliveries were staged, want none: a parked delivery cannot be edited", deliveries)
	}
	var review labelReview
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+liveReviewID(t, c), nil, nil, &review); status != http.StatusOK {
		t.Fatalf("review = %d", status)
	}
	if review.ReasonCode != want || len(review.Refusals) == 0 ||
		slices.ContainsFunc(review.Refusals, func(r labelRefusal) bool { return r.ReasonCode != want }) {
		t.Fatalf("review %+v, want every refusal %q", review, want)
	}
}

func TestSendRefusesAnAdvertisingSubjectWithoutItsLabel(t *testing.T) {
	c := setupConsent(t)
	labelled, _ := subjectLabelCountries()
	c.placeSubjectIn(t, c.contactID, labelled)
	c.refusedAtSend(t, c.sendMarketingTo(t, "subject@consent.test"), commsauthz.ReasonSubjectLabelMissing)
}

func TestSendRefusesRecipientsWhoseLabelsConflict(t *testing.T) {
	c := setupConsent(t)
	korean, singapore := subjectLabelCountries()
	c.placeSubjectIn(t, c.contactID, korean)
	var second struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Second Subject", "emails": []AnyMap{{"email": "second@consent.test"}},
	}, nil, &second); status != http.StatusCreated {
		t.Fatalf("create contact = %d", status)
	}
	c.placeSubjectIn(t, second.ID, singapore)
	c.refusedAtSend(t, c.sendMarketingTo(t, "subject@consent.test", "second@consent.test"),
		commsauthz.ReasonSubjectLabelsConflict)
}

func TestThePreviewReportsAMissingSubjectLabel(t *testing.T) {
	c := setupConsent(t)
	labelled, _ := subjectLabelCountries()
	c.placeSubjectIn(t, c.contactID, labelled)
	var preview struct {
		Allowed    bool `json:"allowed"`
		Recipients []struct {
			ReasonCode string `json:"reason_code"`
		} `json:"recipients"`
	}
	ask := func(subject string) {
		t.Helper()
		if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email:preview", AnyMap{
			"to": []string{"subject@consent.test"}, "consent_purpose": "marketing_email",
			"subject": subject, "body": "Half price this week.",
		}, nil, &preview); status != http.StatusOK {
			t.Fatalf("preview = %d", status)
		}
	}
	ask("Our offer")
	if preview.Allowed || len(preview.Recipients) != 1 || preview.Recipients[0].ReasonCode != commsauthz.ReasonSubjectLabelMissing {
		t.Fatalf("preview %+v, want the missing label", preview)
	}
	ask("(광고) Our offer")
	if len(preview.Recipients) != 1 || preview.Recipients[0].ReasonCode == commsauthz.ReasonSubjectLabelMissing {
		t.Fatalf("preview %+v, want no label refusal once the subject carries it", preview)
	}
}
