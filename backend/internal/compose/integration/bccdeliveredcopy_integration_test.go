// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

const blindObjector = "objector@preflight.test"

// footedRefusal is one refused recipient as the review shows it, with the
// footer findings on their copy.
type footedRefusal struct {
	Address        string                     `json:"address"`
	ReasonCode     string                     `json:"reason_code"`
	FooterFindings []commsauthz.FooterFinding `json:"footer_findings"`
}

// An objection in Bcc, a To with no evidence and a footer the jurisdiction
// owes but the installation cannot fill: the objection leads, the sender is
// shown every finding, and one confirmation sends. The copy handed to the
// provider names the blind copy only in the Bcc header Gmail strips, and
// carries neither the sender's reason nor anyone's private link.
func TestABlindObjectionLeadsAndTheDeliveredCopyKeepsTheBlindCopyPrivate(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	c := &consentEnv{AppEnv: p.AppEnv, contactID: p.contactID, activityID: p.activityID}
	var objector struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Blind Objector", "emails": []AnyMap{{"email": blindObjector}},
	}, nil, &objector); status != http.StatusCreated {
		t.Fatalf("creating the blind copy's contact → %d", status)
	}
	if status := c.Call(t, "POST", "/v1/contacts/"+objector.ID+"/consent/suppress", AnyMap{
		"kind": "marketing_objection", "reason": "They objected to marketing.",
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("recording the objection → %d", status)
	}
	if _, err := c.Owner.Exec(t.Context(), `UPDATE contact SET address_country = @country WHERE id IN (@to, @bcc)`,
		pgx.StrictNamedArgs{"country": footerExceptionCountry(), "to": c.contactID, "bcc": objector.ID}); err != nil {
		t.Fatal(err)
	}

	var refused refusedSend
	if status := c.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Our offer", "body": "The terms for next year.",
		"to": []string{"buyer@preflight.test"}, "bcc": []string{blindObjector}, "consent_purpose": "marketing_email",
	}, nil, &refused); status != http.StatusConflict {
		t.Fatalf("the mixed send → %d, want 409", status)
	}
	read := c.review(t, refused.Details.ReviewID)
	var shown struct {
		Refusals []footedRefusal `json:"refusals"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+read.ID, nil, nil, &shown); status != http.StatusOK {
		t.Fatalf("reading the review → %d", status)
	}
	if len(shown.Refusals) != 2 || shown.Refusals[0].Address != blindObjector || shown.Refusals[0].ReasonCode != commsauthz.ReasonObjection {
		t.Fatalf("the review shows %+v, want the blind objection first and the To beside it", shown.Refusals)
	}
	if !slices.ContainsFunc(shown.Refusals, func(r footedRefusal) bool { return len(r.FooterFindings) > 0 }) {
		t.Fatalf("the review shows no footer finding: %+v", shown.Refusals)
	}

	const explanation = "They asked for these terms on the phone today."
	confirmation := confirmationOf(read)
	confirmation["explanation"] = explanation
	var sent struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+read.ID+"/direct-send", confirmation, nil, &sent); status != http.StatusCreated {
		t.Fatalf("one confirmation → %d, want 201", status)
	}
	activity, err := ids.Parse(sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	delivery, _ := p.deliveryFor(t, activity)
	mime, _ := p.transmit(t, delivery, "")
	headers, body, found := strings.Cut(string(mime), "\r\n\r\n")
	if !found {
		t.Fatalf("the delivered copy has no header boundary:\n%s", mime)
	}
	if !strings.Contains(headers, "\r\nBcc: "+blindObjector+"\r\n") || strings.Count(string(mime), blindObjector) != 1 {
		t.Fatalf("the blind copy is named outside its own Bcc header:\n%s", mime)
	}
	for _, private := range []string{explanation, read.ID, "/preferences/", "/unsubscribe", "List-Unsubscribe"} {
		if strings.Contains(string(mime), private) {
			t.Errorf("the delivered copy carries %q:\n%s", private, body)
		}
	}
}
