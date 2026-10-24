// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
)

// A scheduled message waits as its sender typed it, because that text is what
// goes out when it fires. Only the timeline row it then writes is masked.
func TestAScheduledSendDeliversWhatWasTypedAndStoresItMasked(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	p.stakeADeal(t)

	const secret = "Hunter2-Example!"
	var scheduled struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Zugang Passwort: " + secret, "body": "Passwort: " + secret,
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"scheduled_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339), "scheduled_tz": "Europe/Berlin",
	}, nil, &scheduled); status != http.StatusCreated {
		t.Fatalf("scheduling a send → %d, want 201", status)
	}
	id := uuidOf(t, scheduled.ID)
	p.makeDue(t, id)
	p.fire(t, id)
	if !p.sent(t, id) {
		t.Fatal("firing a due message did not send it")
	}

	var deliveredSubject, deliveredBody, storedSubject, storedBody string
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT o.subject, o.body, a.subject, a.body
			  FROM comms_outbound o JOIN activity a ON a.id = o.activity_id
			 WHERE a.id = $1`, p.releasedActivity(t, id)).Scan(&deliveredSubject, &deliveredBody, &storedSubject, &storedBody)
	}); err != nil {
		t.Fatalf("reading the fired message: %v", err)
	}
	if !strings.Contains(deliveredSubject, secret) || !strings.Contains(deliveredBody, secret) {
		t.Fatalf("the delivery carries %q / %q, want the text as typed", deliveredSubject, deliveredBody)
	}
	if storedSubject != credentialmask.MaskText(deliveredSubject) || storedBody != credentialmask.MaskText(deliveredBody) {
		t.Fatalf("the sent row stores %q / %q, want the delivered text masked", storedSubject, storedBody)
	}
}
