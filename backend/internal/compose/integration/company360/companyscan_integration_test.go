// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The account scan end to end, over a real database: an open of the page
// queues a read, the worker settles what the model grounded, the reader can
// put a finding off exactly as a rule's row, the rail hears every transition
// — and a message outside the reader's audience is never quoted, however
// the model tries.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	company360svc "github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const theirAsk = "Before we go further my team wants to see a sample of the driver reports the units produce."

// seedInboundAsk is one message they sent, linked to the account, whose
// words the model may quote — or may not, when its audience excludes the
// reader.
func seedInboundAsk(t *testing.T, e *integration.Env, company ids.UUID, audience string) ids.UUID {
	t.Helper()
	owner := integration.OwnerConn(t)
	sent := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, direction, subject, body, occurred_at, created_at, source, captured_by, audience)
		VALUES ($1, 'email', 'inbound', 'Telematics — next steps', $2,
		        '2026-05-20T10:00:00Z', '2026-05-20T10:00:00Z', 'manual', 'human:x', $3)`, theirAsk, audience)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id)
		VALUES ($1, 'company', $2)`, sent, company)
	return sent
}

// quotingLane answers every request with one finding resting on the given
// message and quoting the given words — the reply a model would give, with
// the id it was handed or one it made up.
type quotingLane struct {
	messageID string
	quote     string
	calls     int
}

func (l *quotingLane) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	l.calls++
	reply, err := json.Marshal(map[string]any{"findings": []map[string]any{{
		"kind": "question_unanswered", "title": "Send the sample reports",
		"reason":     "Jonas asked for sample driver reports and nothing has gone out.",
		"message_id": l.messageID, "quote": l.quote, "action": "draft_reply",
	}}})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(reply)}, nil
}

// scanFor builds the scan over the composite read, recording what it
// queues so the test can play the worker's part. The 360 it is built on is
// returned too: the dismissal endpoint lives there, and it recognises the
// scan's findings only through the seam this wires.
func scanFor(e *integration.Env, lane companyscan.Completer, queued *[]companyscan.Queued) (*companyscan.Service, *company360svc.Service) {
	view := company360Service(e)
	svc := companyscan.NewService(e.Pool, view, view, lane,
		func(_ context.Context, _ pgx.Tx, scan companyscan.Queued) error {
			*queued = append(*queued, scan)
			return nil
		},
		func() string { return "routing-test" }, func() time.Time { return company360Clock }, nil)
	view.RecogniseScanFindings(svc)
	return svc, view
}

func TestTheScanReadsTheAccountForTheReaderAndTheReaderCanPutAFindingOff(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, company.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []companyscan.Queued
	svc, view := scanFor(e, lane, &queued)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	// The open: nothing stored, so a read is queued and the page is told so.
	first, err := svc.Ensure(rep, company, false)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if first.State != crmcontracts.CompanyScanStateQueued || len(queued) != 1 {
		t.Fatalf("state %q with %d queued, want a queued read", first.State, len(queued))
	}
	if lane.calls != 0 {
		t.Fatal("the ensure asked the model in-request; the read belongs to the worker")
	}
	// A second open while the read is in flight starts nothing twice.
	if again, _ := svc.Ensure(rep, company, true); again.State != crmcontracts.CompanyScanStateQueued || len(queued) != 1 {
		t.Fatalf("a read in flight was queued again: %q, %d", again.State, len(queued))
	}

	// The worker, under the reader's own principal with the row as the trace.
	worker := principal.WithCorrelationID(rep, queued[0].ScanID)
	if err := svc.Run(worker, queued[0].ScanID, company); err != nil {
		t.Fatalf("run: %v", err)
	}
	done, err := svc.Get(rep, company)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if done.State != crmcontracts.CompanyScanStateDone || done.GeneratedBy == nil || *done.GeneratedBy != crmcontracts.Model {
		t.Fatalf("state %q by %v, want done by the model", done.State, done.GeneratedBy)
	}
	if done.Stale != nil && *done.Stale {
		t.Error("a scan just read is reported stale")
	}
	if done.Read == nil || done.Read.Exchanges != 1 {
		t.Errorf("read = %+v, want the one exchange counted", done.Read)
	}
	if len(done.Findings) != 1 {
		t.Fatalf("findings = %v, want the one grounded finding", kindsOf(done.Findings))
	}
	found := done.Findings[0]
	cited := found.Evidence[0]
	if cited.Quote == nil || !strings.Contains(theirAsk, *cited.Quote) || cited.Origin == nil || *cited.Origin != "Email they sent" {
		t.Errorf("receipt = quote %v, origin %v; want the message's own words and where they came from", cited.Quote, cited.Origin)
	}
	if found.Action == nil || found.Action.ActivityId == nil || found.Action.ActivityId.String() != message.String() {
		t.Errorf("action = %+v, want a draft anchored on the message", found.Action)
	}

	// Putting it off: the dismissal endpoint recognises the scan's finding
	// through the same seam it recognises a rule's row, and the next read
	// carries it no more.
	if err := view.DismissSuggestion(rep, company, found.Fingerprint, nil); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	after, err := svc.Get(rep, company)
	if err != nil {
		t.Fatalf("get after dismissal: %v", err)
	}
	if len(after.Findings) != 0 {
		t.Errorf("findings after dismissal = %v, want none", kindsOf(after.Findings))
	}

	// The rail heard every transition, keyed on the row: queued, running, done.
	var announced int
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'ai_task.state_changed'
		   AND envelope->'payload'->>'source' = $1
		   AND envelope->'payload'->>'occurrence_key' = $2`,
		companyscan.ActivitySource, queued[0].ScanID.String()).Scan(&announced); err != nil {
		t.Fatalf("count the rail events: %v", err)
	}
	if announced != 3 {
		t.Errorf("the rail heard %d transitions, want queued, running and done", announced)
	}
}

// The content gate. A message whose audience excludes the reader is not in
// the input, so the model — handed no such id — cannot cite it; and a reply
// that cites it anyway is refused whole. The account settles degraded with
// no finding rather than with a quote the reader may not read.
func TestAMessageOutsideTheReadersAudienceIsNeverQuoted(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	hidden := seedInboundAsk(t, e, company.UUID, "selected")
	lane := &quotingLane{messageID: hidden.String(), quote: "wants to see a sample of the driver reports"}
	var queued []companyscan.Queued
	svc, _ := scanFor(e, lane, &queued)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	if _, err := svc.Ensure(rep, company, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	worker := principal.WithCorrelationID(rep, queued[0].ScanID)
	if err := svc.Run(worker, queued[0].ScanID, company); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := svc.Get(rep, company)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != crmcontracts.CompanyScanStateDegraded || lane.calls != 0 {
		t.Fatalf("state %q after %d model calls; want degraded with no call — the reader may read no exchange, so there was nothing to ask about", got.State, lane.calls)
	}
	if len(got.Findings) != 0 {
		t.Errorf("findings = %v, want none", kindsOf(got.Findings))
	}
}
