// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func originLead(t *testing.T, e *integration.Env, name string, in contacts.CreateLeadInput) ids.UUID {
	t.Helper()
	in.FullName, in.Status = &name, "new"
	if in.Source == "" {
		in.Source = "manual"
	}
	lead, _, err := e.Contacts.CreateLead(e.Admin(), in)
	if err != nil {
		t.Fatalf("creating lead %s: %v", name, err)
	}
	return ids.UUID(lead.Id)
}

func leadOriginState(t *testing.T, e *integration.Env) string {
	t.Helper()
	return e.WsScalar(t, `SELECT string_agg(full_name || ':' || coalesce(origin, '-') || ':' || score || ':' || coalesce(score_computed::text, '-') || ':' || version, ',' ORDER BY full_name) FROM lead`)
}

// The task lifts stored scores that an old source weight lowered. It fills an
// origin only where a retained capture names one that resolves. A second run
// finds nothing left to change, and a human's override and origin stay as set.
func TestTheOriginTaskRescoresAndFillsOriginsAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	referral, event := "referral", "event"
	highScore := originScoreOf(contacts.SourceIntentHigh)

	lifted := originLead(t, e, "A lifted", contacts.CreateLeadInput{Source: "import", Origin: &referral})
	e.WsExec(t, `UPDATE lead SET score = 0 WHERE id = $1`, lifted)
	held := originLead(t, e, "B override", contacts.CreateLeadInput{Origin: &referral})
	override, reason := 77, "buyer confirmed budget on the phone"
	if _, err := e.Contacts.UpdateLead(e.Admin(), ids.From[ids.LeadKind](held), contacts.UpdateLeadInput{Score: &override, ScoreOverrideReason: &reason}); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE lead SET score_computed = 1 WHERE id = $1`, held)
	neutral := originLead(t, e, "C no origin", contacts.CreateLeadInput{Source: "import"})
	e.WsExec(t, `UPDATE lead SET score = 3 WHERE id = $1`, neutral)

	captured := map[string]string{
		"D resolves":    `{"LeadSource":"Referral"}`,
		"E unresolved":  `{"lead_source":"Cold list from a fair"}`,
		"G not a shape": `"LeadSource: Referral"`,
	}
	for name, payload := range captured {
		id := originLead(t, e, name, contacts.CreateLeadInput{})
		key := "cap-" + name
		e.WsExec(t, `UPDATE lead SET source_system = 'crm_sync', source_id = $2 WHERE id = $1`, id, key)
		e.WsExec(t, `INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('crm_sync', $1, $2::jsonb)`, key, payload)
	}
	human := originLead(t, e, "F human origin", contacts.CreateLeadInput{Origin: &event})
	e.WsExec(t, `UPDATE lead SET source_system = 'crm_sync', source_id = 'cap-F' WHERE id = $1`, human)
	e.WsExec(t, `INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('crm_sync', 'cap-F', '{"LeadSource":"Referral"}'::jsonb)`)

	now := clocktest.Now(t)
	task := leadScoresFollowOriginDataTask(func() time.Time { return now })
	for i := range task.Steps {
		task.Steps[i].Limit, task.Steps[i].MinLimit = 2, 1
	}
	runMachineAddressTask(ctx, t, e, task)

	if got := e.WsCount(t, `SELECT score FROM lead WHERE id = $1`, lifted); got != highScore {
		t.Errorf("a lead the old weight lowered scores %d, want %d", got, highScore)
	}
	if got := e.WsCount(t, `SELECT score FROM lead WHERE id = $1`, held); got != override {
		t.Errorf("a human's score override became %d", got)
	}
	if got := e.WsCount(t, `SELECT score_computed FROM lead WHERE id = $1`, held); got != highScore {
		t.Errorf("the machine value beside an override is %d, want %d", got, highScore)
	}
	if got := e.WsCount(t, `SELECT score FROM lead WHERE id = $1`, neutral); got != 0 {
		t.Errorf("a lead with no origin scores %d, want the neutral 0", got)
	}
	for name, want := range map[string]string{"D resolves": "referral", "E unresolved": "", "G not a shape": "", "F human origin": "event"} {
		if got := e.WsScalar(t, `SELECT coalesce(origin, '') FROM lead WHERE full_name = $1`, name); got != want {
			t.Errorf("%s ends with origin %q, want %q", name, got, want)
		}
	}
	if got := e.WsCount(t, `SELECT score FROM lead WHERE full_name = 'D resolves'`); got != highScore {
		t.Errorf("a lead given a referral origin scores %d, want %d: the rescore step runs after the fill", got, highScore)
	}

	settled := leadOriginState(t, e)
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'lead'`)
	events := e.WsCount(t, `SELECT count(*) FROM event_outbox`)
	runMachineAddressTask(ctx, t, e, task)
	if again := leadOriginState(t, e); again != settled {
		t.Errorf("a second run changed the leads:\n%s\nbecame\n%s", settled, again)
	}
	if again := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'lead'`); again != audits {
		t.Errorf("a second run wrote audit rows: %d -> %d", audits, again)
	}
	if again := e.WsCount(t, `SELECT count(*) FROM event_outbox`); again != events {
		t.Errorf("a second run queued events: %d -> %d", events, again)
	}
}

// originScoreOf is the formula's own number for a lead with no signals.
func originScoreOf(intent contacts.SourceIntent) int {
	return contacts.ScoreLeadDetail("", intent, nil, time.Time{}).Score
}
