// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// The update shape's own behaviour, which the integration suite cannot show
// because most of it needs the database to refuse a statement that normally
// succeeds.
//
// An update is a domain row, an audit row and an event that land in ONE
// transaction, so a step that fails without saying so leaves the transaction to
// commit the parts that worked: the row moves and the trail does not, or both
// move and no consumer hears. Each failure has to reach the caller, and each has
// to say WHICH step it was — "something went wrong writing a contract" does not
// tell an operator whether to look at the row, the audit table or the outbox.
//
// The fake records what each statement was HANDED, not only that one was sent.
// Recording the SQL alone leaves the payloads untested, and the event's is not
// guarded anywhere else: events.Envelope.Validate checks the id, type, actor,
// entity and trace and never inspects the payload, so an emit that shipped
// patch.Before() as ChangedFields satisfies every other gate in the tree.

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// updatingContext carries what every real write path binds before reaching the
// store: the actor the audit row is filed under, and the correlation id that
// links that row to the event. Emit refuses without the second, so a context
// missing it fails the shape one step earlier than the step under test.
func updatingContext() context.Context {
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String()})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// aContractPatch carries one column, so the patch is not empty and the shape
// runs all the way through rather than returning early.
func aContractPatch() *storekit.Patch {
	patch := storekit.NewPatch()
	patch.Set("title", nil, "renamed")
	return patch
}

// The three statements an update sends, matched by what they DO rather than by
// the table they mention. `contract` appears in the row lock's own SELECT, so a
// substring test is satisfied by a shape that locked the row and never wrote it.
var (
	domainWrite = regexp.MustCompile(`(?is)^\s*UPDATE\s+contract\s+SET`)
	auditWrite  = regexp.MustCompile(`(?is)INSERT\s+INTO\s+audit_log`)
	eventWrite  = regexp.MustCompile(`(?is)INSERT\s+INTO\s+event_outbox`)
)

// The shape's success, and the assertion that makes the refusals below mean
// something: all THREE writes are sent. A shape that quietly stopped sending
// one would pass every refusal test here, because a write that never happens
// cannot be refused.
func TestTheContractUpdateShapeSendsAllThreeWrites(t *testing.T) {
	tx := &recordingTx{}
	if err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update"); err != nil {
		t.Fatalf("the update shape refused a transaction that accepted everything: %v", err)
	}
	for what, statement := range map[string]*regexp.Regexp{
		"the contract row": domainWrite, "the audit row": auditWrite, "the event": eventWrite,
	} {
		if tx.find(statement) == nil {
			t.Errorf("%s was never written: an update is a row, a trail and an event, and a "+
				"shape that sends two of the three commits a change nobody can see.\n"+
				"Statements sent:\n\t%s", what, strings.Join(tx.sql(), "\n\t"))
		}
	}
}

// What the event CARRIES, which nothing else in the tree checks. Swapping
// patch.After() for patch.Before() at the emit leaves every other gate green
// and ships every consumer the pre-change values under the name "changed
// fields" — the drift a one-writer gate cannot see, because there is still only
// one writer.
func TestTheContractUpdateEventCarriesWhatChanged(t *testing.T) {
	tx := &recordingTx{}
	patch := aContractPatch()
	if err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		patch, nil, "contract update"); err != nil {
		t.Fatalf("applying the update: %v", err)
	}
	sent := tx.find(eventWrite)
	if sent == nil {
		t.Fatal("no event was written, so there is nothing to check what it carries")
	}
	body, isBytes := sent.args[len(sent.args)-1].([]byte)
	if !isBytes {
		t.Fatalf("the outbox row's last bind is %T, not the marshalled envelope", sent.args[len(sent.args)-1])
	}
	var envelope struct {
		Payload struct {
			ChangedFields map[string]any `json:"changed_fields"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("reading the envelope the shape emitted: %v", err)
	}
	after := patch.After()
	if len(envelope.Payload.ChangedFields) != len(after) {
		t.Fatalf("the event carries %v; the write changed %v", envelope.Payload.ChangedFields, after)
	}
	for field := range after {
		if _, present := envelope.Payload.ChangedFields[field]; !present {
			t.Errorf("the event omits %s, which this write changed", field)
			continue
		}
		// The RENDERED forms, because that is what a consumer reads: the write
		// holds Go values and the envelope holds what survived a JSON round
		// trip, where `int64(1)` and `float64(1)` are the same field.
		got := fieldText(envelope.Payload.ChangedFields, field)
		want := fieldText(after, field)
		if got != want {
			t.Errorf("the event says %s is %s; the write set it to %s", field, got, want)
		}
	}
}

// fieldText renders one field of a changed-field set as a consumer would read
// it, or a marker naming why it could not be rendered.
func fieldText(fields map[string]any, name string) string {
	rendered, err := json.Marshal(fields[name])
	if err != nil {
		return "<unrenderable: " + err.Error() + ">"
	}
	return string(rendered)
}

func TestARefusedContractPatchReachesTheCaller(t *testing.T) {
	refused := errors.New("deadlock detected")
	tx := &recordingTx{refuse: domainWrite, err: refused}
	err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update")
	if !errors.Is(err, refused) {
		t.Fatalf("the database's refusal did not reach the caller: %v", err)
	}
	if !strings.Contains(err.Error(), "write contract update") {
		t.Errorf("the error does not say which step failed: %v", err)
	}
}

// The audit row is the half a reader would not notice missing. A patch that
// lands while its audit insert fails silently is a change with no trail, and
// the row itself looks exactly like one that was recorded properly.
func TestARefusedAuditRowStopsTheContractUpdate(t *testing.T) {
	refused := errors.New("audit_log is full")
	tx := &recordingTx{refuse: auditWrite, err: refused}
	err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update")
	if !errors.Is(err, refused) {
		t.Fatalf("a refused audit row did not reach the caller: %v", err)
	}
	if !strings.Contains(err.Error(), "audit contract update") {
		t.Errorf("the error does not say which step failed: %v", err)
	}
}

// The event is the half no consumer would notice missing, because a consumer
// that never receives a message cannot tell it apart from one never sent.
func TestARefusedEventStopsTheContractUpdate(t *testing.T) {
	refused := errors.New("outbox unavailable")
	tx := &recordingTx{refuse: eventWrite, err: refused}
	err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update")
	if !errors.Is(err, refused) {
		t.Fatalf("a refused event did not reach the caller: %v", err)
	}
	if !strings.Contains(err.Error(), "emit contract.updated") {
		t.Errorf("the error does not say which step failed: %v", err)
	}
}

// A CHECK the database refuses is not an infrastructure failure — it is the
// caller having asked for something the schema forbids, and it has to arrive as
// the field-level refusal a caller can act on rather than a wrapped driver
// error. It is refused by the UPDATE, which is the only statement that can
// raise one: a fake that raised it from the row lock's SELECT would still pass
// while the UPDATE stopped propagating it.
func TestARefusedCheckBecomesAFieldLevelRefusal(t *testing.T) {
	tx := &recordingTx{refuse: domainWrite, err: &pgconn.PgError{
		Code: "23514", ConstraintName: "contract_term_order",
	}}
	err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update")
	var refusal *ContractCheckError
	if !errors.As(err, &refusal) {
		t.Fatalf("a refused CHECK did not become a field-level refusal: %v", err)
	}
	if refusal.Field != "ends_on" {
		t.Errorf("the refusal names %q, so a caller is pointed at the wrong field", refusal.Field)
	}
}

// An If-Match request takes a different branch of the guarded patch — a
// compare-and-set rather than a row lock — and that branch mints the skew a
// caller must be able to retry on. The new wrap must not bury it: a client that
// gets an opaque failure instead of a version conflict cannot tell "try again
// with a fresh read" from "this will never work".
func TestAStaleIfMatchSurvivesTheUpdateShapesWrap(t *testing.T) {
	stale := int64(3)
	tx := &recordingTx{missed: true, exists: true}
	err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), &stale, "contract update")
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a stale If-Match did not reach the caller as version skew: %v", err)
	}
	if tx.find(auditWrite) != nil {
		t.Error("a write that never landed still filed an audit row")
	}
}

// recordingTx answers each statement as though it had worked, unless it matches
// `refuse`. It keeps what every statement was handed, because the SQL alone
// leaves the payloads untested.
//
// It panics on the methods the shape does not use: a future step reaching for
// the database another way fails loudly here rather than being answered by a
// zero value that makes a test pass for the wrong reason.
type recordingTx struct {
	refuse *regexp.Regexp
	err    error
	// missed makes the compare-and-set match no row, and exists answers the
	// follow-up that tells a stale version from a deleted row.
	missed bool
	exists bool
	sent   []statement
}

type statement struct {
	text string
	args []any
}

func (r *recordingTx) record(sql string, args []any) bool {
	r.sent = append(r.sent, statement{text: sql, args: args})
	return r.refuse != nil && r.refuse.MatchString(sql)
}

// find returns the first statement matching want, or nil.
func (r *recordingTx) find(want *regexp.Regexp) *statement {
	for i := range r.sent {
		if want.MatchString(r.sent[i].text) {
			return &r.sent[i]
		}
	}
	return nil
}

func (r *recordingTx) sql() []string {
	var out []string
	for _, sent := range r.sent {
		out = append(out, strings.Join(strings.Fields(sent.text), " "))
	}
	return out
}

func (r *recordingTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if r.record(sql, args) {
		return pgconn.CommandTag{}, r.err
	}
	if r.missed && domainWrite.MatchString(sql) {
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (r *recordingTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if r.record(sql, args) {
		return answerRow{err: r.err}
	}
	return answerRow{id: ids.NewV7(), exists: r.exists}
}

func (r *recordingTx) Begin(context.Context) (pgx.Tx, error) { panic("recordingTx: Begin") }
func (r *recordingTx) Commit(context.Context) error          { panic("recordingTx: Commit") }
func (r *recordingTx) Rollback(context.Context) error        { panic("recordingTx: Rollback") }
func (r *recordingTx) Conn() *pgx.Conn                       { panic("recordingTx: Conn") }
func (r *recordingTx) LargeObjects() pgx.LargeObjects        { panic("recordingTx: LargeObjects") }

func (r *recordingTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("recordingTx: CopyFrom")
}

func (r *recordingTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("recordingTx: SendBatch")
}

func (r *recordingTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("recordingTx: Prepare")
}

func (r *recordingTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("recordingTx: Query")
}

// answerRow serves the two QueryRow the shape can issue: the row lock's
// `SELECT id … FOR UPDATE`, and the compare-and-set's follow-up `SELECT EXISTS`
// that tells a stale version from a row that is gone. The audit insert does not
// appear here — it is an Exec, and mints its own id in Go.
type answerRow struct {
	id     ids.UUID
	exists bool
	err    error
}

func (a answerRow) Scan(into ...any) error {
	if a.err != nil {
		return a.err
	}
	for _, target := range into {
		switch slot := target.(type) {
		case *ids.UUID:
			*slot = a.id
		case *bool:
			*slot = a.exists
		}
	}
	return nil
}
