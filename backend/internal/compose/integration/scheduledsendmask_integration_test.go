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
	"github.com/margince/margince/backend/internal/platform/clocktest"
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
		"scheduled_at": clocktest.Now(t).Add(2 * time.Hour).UTC().Format(time.RFC3339), "scheduled_tz": "Europe/Berlin",
	}, nil, &scheduled); status != http.StatusCreated {
		t.Fatalf("scheduling a send → %d, want 201", status)
	}
	id := uuidOf(t, scheduled.ID)
	p.makeDue(t, id)
	p.fire(t, id)
	if !p.sent(t, id) {
		t.Fatal("firing a due message did not send it")
	}

	sentRow := p.releasedActivity(t, id)
	var deliveredSubject, deliveredBody, storedSubject, storedBody string
	var audits []string
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `
			SELECT o.subject, o.body, a.subject, a.body
			  FROM comms_outbound o JOIN activity a ON a.id = o.activity_id
			 WHERE a.id = $1`, sentRow).Scan(&deliveredSubject, &deliveredBody, &storedSubject, &storedBody); err != nil {
			return err
		}
		rows, err := tx.Query(context.Background(), `
			SELECT action || ' ' || coalesce(before::text, '') || coalesce(after::text, '') || coalesce(evidence::text, '')
			  FROM audit_log WHERE entity_id IN ($1, $2)`, id, sentRow)
		if err != nil {
			return err
		}
		audits, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatalf("reading the fired message: %v", err)
	}
	assertAuditsMasked(t, audits, secret)
	if !strings.Contains(deliveredSubject, secret) || !strings.Contains(deliveredBody, secret) {
		t.Fatalf("the delivery carries %q / %q, want the text as typed", deliveredSubject, deliveredBody)
	}
	if storedSubject != credentialmask.MaskText(deliveredSubject) || storedBody != credentialmask.MaskText(deliveredBody) {
		t.Fatalf("the sent row stores %q / %q, want the delivered text masked", storedSubject, storedBody)
	}
}

// assertAuditsMasked holds every audit image of the message to masked text,
// and needs the schedule image that names the subject among them.
func assertAuditsMasked(t *testing.T, audits []string, secret string) {
	t.Helper()
	scheduled := false
	for _, audit := range audits {
		if strings.Contains(audit, secret) {
			t.Errorf("an audit image holds the credential: %s", audit)
		}
		scheduled = scheduled || strings.HasPrefix(audit, "schedule ") && strings.Contains(audit, credentialmask.Marker)
	}
	if !scheduled {
		t.Fatalf("no masked schedule image among the audit rows %v", audits)
	}
}
