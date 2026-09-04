// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The verdict column, its backlog and its write door, against a real database.
//
// Nothing in the unit lane reaches any of it: the backlog is the waiting query
// with one more clause, the CAS is a WHERE the database evaluates, and the audit
// row is written by a trigger-adjacent helper. Each of those can be wrong in a
// way that compiles and simply judges the wrong messages.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asClassifier is the context the verdict pass actually runs under: a system
// principal, the way every background job in compose builds one.
//
// The read env's principal is deliberately read-only, and writing the verdict
// needs activity:update — so a test that used it would either fail or push
// somebody to widen the write gate to suit a fixture. The gate is right; the
// fixture has to say who is writing.
func asClassifier(e *loadEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:owed_verdict",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// The backlog is the WAITING QUEUE minus what is judged, not every unjudged mail.
//
// This is the decision that cost the most to get right. A partial index over
// "unjudged inbound" is cheap and wrong: survivorship in the queue turns on
// links to live records, on the colleague rule and on anti-joins for a later
// reply, none of which an index can encode. A backlog built that way spends
// model calls on messages nobody will ever see.
func TestTheBacklogIsTheWaitingQueueAndNotEveryUnjudgedMail(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	waiting := e.waitingFrom(t, "Can you confirm the price?", "buyer@customer.test", person)
	// Unjudged and inbound, and NOT waiting on anybody: nothing links it to a
	// record the workspace sells to.
	loose := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Dentist appointment', now() - interval '2 days', $2, 'seed', 'system')`,
		loose, "thread-"+loose.String())
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', 'reception@dentist.test')`, ids.NewV7(), loose)

	got := unjudged(t, e)

	if !got[waiting] {
		t.Error("a waiting customer's unjudged message is not in the backlog")
	}
	if got[loose] {
		t.Error("mail nobody is waiting on entered the backlog — the candidates " +
			"are the queue's, not every unjudged inbound")
	}
}

// An older unjudged message is reachable however many newer ones are judged.
//
// THE FAILURE THIS PREVENTS is total and silent. The waiting query takes the
// NEWEST WaitingScanCap threads and no more, so a filter applied outside it
// selects from those 200 rather than from the backlog: once the newest are
// judged, an older unjudged message can never be read again, and the classifier
// spends every pass re-reading rows it has already answered. Nothing fails —
// the backlog simply reports itself empty while work sits in it.
//
// It is the same defect waitingsql.go warns about for its own rules, one level
// up: a filter after LIMIT lets rows nobody wants fill the scan.
func TestAnOlderUnjudgedMessageSurvivesNewerJudgedOnes(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	old := e.waitingAgedFrom(t, "The oldest question", "buyer@customer.test", person, 80)
	// Judged, and newer — enough of them to fill the waiting query's own scan
	// cap. That is what makes this a test rather than a hope: with the filter
	// applied outside the statement, these WaitingScanCap newer rows are the
	// whole candidate set and the older one is unreachable.
	for i := range WaitingScanCap {
		newer := e.waitingAgedFrom(t, "Newer thread", "buyer@customer.test", person, (i%70)+1)
		if _, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), newer, OwedVerdictAsksUs); err != nil {
			t.Fatalf("judging newer message %d: %v", i, err)
		}
	}

	if !unjudged(t, e)[old] {
		t.Error("the oldest unjudged message is not in the backlog — judged rows " +
			"are spending the scan the unjudged ones need")
	}
}

// The recipient context travels, because the question needs it.
//
// "Is something owed by me" is not answerable from a subject and a body: a
// report sent to a desk address with the reader on cc reads exactly like a
// direct question. Two of the three messages that opened this work differ from
// real work mostly in who was on the envelope.
func TestTheCandidateCarriesWhoTheMessageWasAddressedTo(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	activity := e.waitingFrom(t, "Monthly reporting", "paul@customer.test", person)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'to', 'reporting@customer.test')`, ids.NewV7(), activity)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'cc', 'lars@ourco.test')`, ids.NewV7(), activity)

	got := candidate(t, e, activity)

	if len(got.To) != 1 || got.To[0] != "reporting@customer.test" {
		t.Errorf("to = %v, wanted the desk address the report was sent to", got.To)
	}
	if len(got.Cc) != 1 || got.Cc[0] != "lars@ourco.test" {
		t.Errorf("cc = %v, wanted the reader who was merely copied", got.Cc)
	}
}

// A judged message leaves the backlog and carries its verdict into the queue.
func TestAJudgedMessageLeavesTheBacklogAndKeepsItsVerdict(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	activity := e.waitingFrom(t, "Monthly reporting", "paul@customer.test", person)

	applied, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs)
	if err != nil {
		t.Fatalf("setting the verdict: %v", err)
	}
	if !applied {
		t.Fatal("the verdict did not apply to an unjudged message")
	}
	if unjudged(t, e)[activity] {
		t.Error("a judged message is still in the backlog")
	}
	row, ok := present(t, storeKnowing(e), e, activity)
	if !ok {
		t.Fatal("the message left the QUEUE — a verdict may only demote")
	}
	if row.OwedVerdict != OwedVerdictInformsUs {
		t.Errorf("the queue read verdict %q, wanted %q", row.OwedVerdict, OwedVerdictInformsUs)
	}
}

// The first verdict stands. Two model calls on one message are two opinions,
// and there is no rule here for preferring the later one.
func TestASecondVerdictDoesNotOverwriteTheFirst(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	activity := e.waitingFrom(t, "Can you confirm?", "buyer@customer.test", person)
	s := storeKnowing(e)

	if _, err := s.SetOwedVerdict(asClassifier(e), activity, OwedVerdictAsksUs); err != nil {
		t.Fatalf("first verdict: %v", err)
	}
	applied, err := s.SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs)
	if err != nil {
		t.Fatalf("second verdict: %v", err)
	}
	if applied {
		t.Error("the second verdict reported that it applied")
	}
	if row, _ := present(t, s, e, activity); row.OwedVerdict != OwedVerdictAsksUs {
		t.Errorf("the verdict is now %q — the first one was overwritten", row.OwedVerdict)
	}
}

// A narrowed message cannot be judged, and the clause is re-tested at the WRITE.
//
// The classifier reads a batch, spends a model call per message and writes the
// answers back. A human or a privacy verdict can narrow a row inside that
// window, and a write landing after the narrowing would stamp a judgement on a
// message the queue's readers may no longer open.
func TestAMessageNarrowedDuringTheModelCallIsNotJudged(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	activity := e.waitingFrom(t, "Private matter", "buyer@customer.test", person)
	// The narrowing lands AFTER the candidate was read, before the write.
	e.exec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, activity)

	applied, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs)
	if err != nil {
		t.Fatalf("setting the verdict: %v", err)
	}
	if applied {
		t.Error("a verdict landed on a message narrowed since it was read")
	}
}

// A message archived during the model call is not judged.
//
// Same window as the narrowing case and the same answer: somebody filed the
// message away while the model was reading it, and coming back with an opinion
// about it writes a verdict and an audit row for a message nobody will see.
func TestAMessageArchivedDuringTheModelCallIsNotJudged(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	activity := e.waitingFrom(t, "Filed away", "buyer@customer.test", person)
	e.exec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, activity)

	applied, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs)
	if err != nil {
		t.Fatalf("setting the verdict: %v", err)
	}
	if applied {
		t.Error("a verdict landed on a message archived since it was read")
	}
}

// The write is audited, because a model touched a customer's mail.
//
// capture_label beside it is deliberately neither audited nor evented, under a
// stated hard-floor rule about routing attention. That rule covers that column.
// This one is a model-derived claim, and "which records did the classifier
// touch" has to stay answerable from audit_log.
func TestJudgingAMessageWritesAnAuditRow(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	activity := e.waitingFrom(t, "Monthly reporting", "paul@customer.test", person)

	var before int
	if err := e.pool.QueryRow(e.as(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`,
		activity).Scan(&before); err != nil {
		t.Fatalf("counting audit rows: %v", err)
	}
	if _, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs); err != nil {
		t.Fatalf("setting the verdict: %v", err)
	}
	var after int
	var recorded string
	if err := e.pool.QueryRow(e.as(),
		`SELECT count(*), coalesce(max(a.after->>'owed_verdict'), '')
		   FROM audit_log a WHERE entity_type = 'activity' AND entity_id = $1`,
		activity).Scan(&after, &recorded); err != nil {
		t.Fatalf("reading audit rows: %v", err)
	}
	if after != before+1 {
		t.Errorf("audit rows went from %d to %d, wanted exactly one more", before, after)
	}
	if recorded != OwedVerdictInformsUs {
		t.Errorf("the audit row records %q, wanted the verdict written", recorded)
	}
}

// A verdict this column does not define is refused before it reaches SQL.
func TestAnUnknownVerdictIsRefused(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	activity := e.waitingFrom(t, "Anything", "buyer@customer.test", person)

	if _, err := storeKnowing(e).SetOwedVerdict(asClassifier(e), activity, "maybe"); err == nil {
		t.Error("an undefined verdict was accepted")
	}
}

// waitingAgedFrom seeds a qualifying wait a given number of days back.
func (e *loadEnv) waitingAgedFrom(t *testing.T, subject, address string, person ids.UUID, days int) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'inbound', $2, now() - make_interval(days => $4), $3, 'seed', 'system')`,
		activity, subject, "thread-"+activity.String(), days)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', $3)`, ids.NewV7(), activity, address)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id)
		VALUES ($1, $2, 'person', $3)`, ids.NewV7(), activity, person)
	return activity
}

// unjudged reads the backlog as a set of ids.
func unjudged(t *testing.T, e *loadEnv) map[ids.UUID]bool {
	t.Helper()
	rows, err := storeKnowing(e).UnjudgedInbound(e.as(), time.Now(), 100, 400)
	if err != nil {
		t.Fatalf("reading the unjudged backlog: %v", err)
	}
	out := map[ids.UUID]bool{}
	for _, row := range rows {
		out[row.ID] = true
	}
	return out
}

// candidate reads one message out of the backlog, failing if it is absent.
func candidate(t *testing.T, e *loadEnv, id ids.UUID) UnjudgedMessage {
	t.Helper()
	rows, err := storeKnowing(e).UnjudgedInbound(e.as(), time.Now(), 100, 400)
	if err != nil {
		t.Fatalf("reading the unjudged backlog: %v", err)
	}
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("the seeded message is absent from %d candidates", len(rows))
	return UnjudgedMessage{}
}
