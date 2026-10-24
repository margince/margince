// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func evidenceLead(t *testing.T, e *integration.Env, name string) ids.LeadID {
	t.Helper()
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: &name, Source: "webform", Status: "new",
	})
	if err != nil {
		t.Fatalf("creating lead %s: %v", name, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(lead.Id))
}

func signalWith(factor, band, reason string) contacts.SetLeadManualSignalInput {
	return contacts.SetLeadManualSignalInput{Factor: factor, Band: band, SignalKind: "assumption", Reason: reason}
}

func leadTrail(t *testing.T, e *integration.Env, lead ids.LeadID) (audits, events int) {
	t.Helper()
	audits = e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'lead' AND entity_id = $1`, lead.UUID)
	events = e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'entity_id' = $1 OR envelope::text LIKE '%' || $1 || '%'`, lead.String())
	return audits, events
}

func TestTheStoreRefusesAManualSignalWithoutEvidenceAndWritesNothing(t *testing.T) {
	e := integration.Setup(t)
	lead := evidenceLead(t, e, "No Evidence")
	auditsBefore, eventsBefore := leadTrail(t, e, lead)

	for name, reason := range map[string]string{
		"empty": "", "whitespace": " \n\t ", "the marker": contacts.ManualSignalNoEvidence,
		"the padded marker": "  " + contacts.ManualSignalNoEvidence + " ",
		"stock en":          contacts.StockManualSignalReasons()[0], "stock de": contacts.StockManualSignalReasons()[1],
		"stock vi": contacts.StockManualSignalReasons()[2],
	} {
		_, err := e.Contacts.SetLeadManualSignal(e.Admin(), lead, signalWith("employees", "51-200", reason))
		if _, ok := errors.AsType[*contacts.ReasonRequiredError](err); !ok {
			t.Errorf("%s: err = %v, want ReasonRequiredError", name, err)
		}
	}

	if rows := e.WsCount(t, `SELECT count(*) FROM lead_manual_signal WHERE lead_id = $1`, lead.UUID); rows != 0 {
		t.Errorf("%d signal rows after the refusals, want 0", rows)
	}
	if audits, events := leadTrail(t, e, lead); audits != auditsBefore || events != eventsBefore {
		t.Errorf("refusals left audit %d->%d and events %d->%d, want no change", auditsBefore, audits, eventsBefore, events)
	}
}

func TestAManualSignalStoresAndAuditsTheTrimmedEvidence(t *testing.T) {
	e := integration.Setup(t)
	lead := evidenceLead(t, e, "Trimmed Evidence")
	out, err := e.Contacts.SetLeadManualSignal(e.Admin(), lead, signalWith("employees", "51-200", "  seen on the careers page \n"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != "seen on the careers page" {
		t.Errorf("returned reason = %q", out.Reason)
	}
	if got := e.WsScalar(t, `SELECT reason FROM lead_manual_signal WHERE lead_id = $1`, lead.UUID); got != "seen on the careers page" {
		t.Errorf("stored reason = %q", got)
	}
	if got := e.WsScalar(t, `SELECT after->'manual_signal'->>'reason' FROM audit_log
		WHERE entity_type = 'lead' AND entity_id = $1 AND after ? 'manual_signal'`, lead.UUID); got != "seen on the careers page" {
		t.Errorf("audited reason = %q", got)
	}
}

func TestTheNoEvidenceTaskMarksOnlyStockSentencesAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	stock := contacts.StockManualSignalReasons()
	if len(stock) != 3 {
		t.Fatalf("%d stock sentences, want one per locale", len(stock))
	}
	factors := [][2]string{{"employees", "51-200"}, {"web_traffic", "high"}, {"budget_hint", "some"}}
	var forced []ids.LeadID
	for i, sentence := range stock {
		lead := evidenceLead(t, e, "Stock "+sentence[:2])
		for _, f := range factors[:i+1] {
			if _, err := e.Contacts.SetLeadManualSignal(e.Admin(), lead, signalWith(f[0], f[1], "typed first")); err != nil {
				t.Fatal(err)
			}
		}
		forced = append(forced, lead)
		e.WsExec(t, `UPDATE lead_manual_signal SET reason = $2 WHERE lead_id = $1`, lead.UUID, sentence)
	}
	human := evidenceLead(t, e, "Human Text")
	if _, err := e.Contacts.SetLeadManualSignal(e.Admin(), human, signalWith("employees", "11-50", "They told me on the phone.")); err != nil {
		t.Fatal(err)
	}
	// A superseded row still shows in the history, so it is repaired too.
	e.WsExec(t, `UPDATE lead_manual_signal SET superseded_at = now(), superseded_by = 'capture' WHERE lead_id = $1 AND factor = 'employees'`, forced[0].UUID)
	columns := `SELECT string_agg(id::text || band || points || signal_kind || set_by || set_at || coalesce(superseded_by,''), ',' ORDER BY id) FROM lead_manual_signal`
	beforeColumns := e.WsScalar(t, columns)
	scoreBefore := e.WsScalar(t, `SELECT string_agg(score::text, ',' ORDER BY id) FROM lead`)

	stockRows := e.WsCount(t, `SELECT count(*) FROM lead_manual_signal WHERE reason = ANY($1)`, stock)
	if stockRows != 6 {
		t.Fatalf("%d stock rows seeded, want 6", stockRows)
	}
	step := manualSignalNoEvidenceDataTask().Steps[0]
	var estimated int64
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		estimated, _, err = step.Estimate(ctx, tx, 1000)
		return err
	}); err != nil || estimated != 6 {
		t.Fatalf("estimate = %d, %v, want 6", estimated, err)
	}

	task := manualSignalNoEvidenceDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 2, 1
	auditsBefore := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'lead' AND after ? 'manual_signals'`)
	runMachineAddressTask(ctx, t, e, task)

	if left := e.WsCount(t, `SELECT count(*) FROM lead_manual_signal WHERE reason = ANY($1)`, stock); left != 0 {
		t.Fatalf("%d stock rows left", left)
	}
	if marked := e.WsCount(t, `SELECT count(*) FROM lead_manual_signal WHERE reason = $1`, contacts.ManualSignalNoEvidence); marked != 6 {
		t.Fatalf("%d marked rows, want 6", marked)
	}
	if got := e.WsScalar(t, `SELECT reason FROM lead_manual_signal WHERE lead_id = $1`, human.UUID); got != "They told me on the phone." {
		t.Errorf("the human's reason became %q", got)
	}
	if got := e.WsScalar(t, columns); got != beforeColumns {
		t.Error("the repair changed a column other than reason")
	}
	if got := e.WsScalar(t, `SELECT string_agg(score::text, ',' ORDER BY id) FROM lead`); got != scoreBefore {
		t.Error("the repair moved a lead score")
	}
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'lead' AND after ? 'manual_signals'`) - auditsBefore
	for i, lead := range forced {
		if e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after ? 'manual_signals'`, lead.UUID) == 0 {
			t.Errorf("repaired lead %d has no audit row", i)
		}
	}
	if got := e.WsScalar(t, `SELECT before->'manual_signals'->0->>'reason' FROM audit_log WHERE entity_id = $1 AND after ? 'manual_signals'`, forced[0].UUID); got != stock[0] {
		t.Errorf("audit before-image reason = %q, want the stock sentence", got)
	}

	runMachineAddressTask(ctx, t, e, task)
	if again := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'lead' AND after ? 'manual_signals'`) - auditsBefore; again != audits {
		t.Errorf("a second run raised the repair audit rows from %d to %d", audits, again)
	}
}

func TestTheNoEvidenceBatchRollsBackWithItsTransaction(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	lead := evidenceLead(t, e, "Rollback Lead")
	if _, err := e.Contacts.SetLeadManualSignal(e.Admin(), lead, signalWith("employees", "51-200", "typed")); err != nil {
		t.Fatal(err)
	}
	stock := contacts.StockManualSignalReasons()[0]
	e.WsExec(t, `UPDATE lead_manual_signal SET reason = $2 WHERE lead_id = $1`, lead.UUID, stock)

	rollback := errors.New("rolled back")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := manualSignalNoEvidenceDataTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err != nil || out.Changed != 1 {
			t.Errorf("batch changed %d, err %v, want 1", out.Changed, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT reason FROM lead_manual_signal WHERE lead_id = $1`, lead.UUID); got != stock {
		t.Errorf("reason after rollback = %q, want the stock sentence", got)
	}
}
