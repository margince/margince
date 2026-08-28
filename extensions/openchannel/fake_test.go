// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The fakes this unit's suite runs against.
//
// A unit's own tests cannot reach the core: an extensions/ module imports only
// the published surface, so there is no pool, no custodian and no HTTP chassis
// here. What CAN be tested at this level is what this unit owns — the argument
// decoding, the refusals, which statements each handler issues, and above all
// the inbound verification, which is a pure function of the request, the stored
// row and the sealed secret. Whether those statements are correct SQL against a
// real schema is the migration gate's question.
//
// Every fake here answers what the CORE answers, including its refusals: a fake
// that accepts what the core rejects lets a whole suite agree with a bug, and
// the only thing left disagreeing is production.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// The two members the suite runs as, canonical because that is what the column
// they land in accepts.
const (
	ownerUserID     = "9f1d0c4a-3b2e-4f57-9a10-2c8e6b5d4f31"
	colleagueUserID = "1b7f6e25-8c43-4a90-b6d1-5e2a7c3f9081"
	endpointID      = "3c5a1f80-6d24-4b19-8e77-0a4b2d6c9e13"
	// colleagueEndpointID is a well-formed endpoint id that is not the owner's
	// — a colleague's, or simply a stranger's guess. It exists only to prove a
	// confirm-first request naming it is refused as not found rather than
	// acted on or answered with a permission error.
	colleagueEndpointID = "8b4e2a10-5f3c-4d67-9a21-6c8f1e0d4b7a"
	// ownerRef is the address on the owner's endpoint. A fixed value shaped
	// like a minted one, so a fixture reads as the thing it stands for.
	ownerRef = "Qm4xT2lZc0hkRGZLbGFwWQ"
)

// fakeRuntime is one invocation's Runtime.
type fakeRuntime struct {
	secrets *fakeSecrets
	tx      *fakeTx
	caller  extension.Caller

	// txErr stands in for the core refusing to open a transaction at all — an
	// expired Runtime, an unwired role — which every handler must propagate
	// rather than answer over.
	txErr error

	// What the drain handed the capture port, in order, and what the port
	// answered. The two scripts are consumed in step with the calls, so a test
	// scripts the third call's refusal without scripting the first two.
	ingested      []extension.Record
	ingestedFor   []extension.UserID
	ingestedAfter []int
	results       []extension.Result
	ingestErrs    []error
}

func newRuntime() *fakeRuntime {
	return &fakeRuntime{
		secrets: &fakeSecrets{stored: map[string][]byte{}},
		tx:      &fakeTx{noRows: map[int]bool{}},
		caller:  extension.Caller{Type: extension.CallerHuman, UserID: ownerUserID},
	}
}

// unattended is the Runtime a drain or outbound job tick holds: a scheduled
// invocation with no principal behind it, which is what the core mints for a
// job (see Job in the published surface).
func (r *fakeRuntime) unattended() *fakeRuntime {
	r.caller = extension.Caller{Type: extension.CallerSystem}
	return r
}

// connector is the Runtime the anonymous inbound edge holds: a bare
// CallerConnector with no user behind it and no permissions, which is exactly
// what inboundRuntimeFor mints for a request that carries no session — see
// TestInboundCallerIsABareConnector. It is deliberately not the same shape as
// unattended: a unit reading Caller() on this path sees "a connector, on
// nobody's behalf", not the system identity a job tick answers as.
func (r *fakeRuntime) connector() *fakeRuntime {
	r.caller = extension.Caller{Type: extension.CallerConnector, IsAgent: true}
	return r
}

func (r *fakeRuntime) Secrets() extension.Secrets { return r.secrets }

func (r *fakeRuntime) Caller() extension.Caller { return r.caller }

func (r *fakeRuntime) Tx(ctx context.Context, fn func(context.Context, extension.Tx) error) error {
	if r.txErr != nil {
		return r.txErr
	}
	r.tx.open = true
	defer func() { r.tx.open = false }()
	return fn(ctx, r.tx)
}

// Ingest records what was handed to the core and answers what the test
// scripted, one entry per call.
//
// IT REFUSES A NESTED CALL exactly as the core does, and that refusal is the
// whole reason this fake is not a stub returning success: the drain's central
// rule is that a record is ingested with none of the unit's transactions open,
// and a fake that accepted one would let the suite agree with the bug that
// hangs a small connection pool in production.
func (r *fakeRuntime) Ingest(_ context.Context, on extension.UserID, rec extension.Record) (extension.Result, error) {
	if r.tx.open {
		return extension.Result{}, extension.ErrNestedIngest
	}
	r.ingested = append(r.ingested, rec)
	r.ingestedFor = append(r.ingestedFor, on)
	// How many statements had been issued when this record was handed over. It
	// is what lets a test assert the ORDER the drain's whole safety argument
	// rests on: no row is advanced before the core has answered for it.
	r.ingestedAfter = append(r.ingestedAfter, len(r.tx.statements))
	// Both scripts are consumed in step, so an entry answers the call at its own
	// position rather than the first one that asks.
	var (
		result extension.Result
		err    error
	)
	if len(r.results) > 0 {
		result, r.results = r.results[0], r.results[1:]
	}
	if len(r.ingestErrs) > 0 {
		err, r.ingestErrs = r.ingestErrs[0], r.ingestErrs[1:]
	}
	return result, err
}

// fakeSecrets is the unit's namespace, user scope only — the installation scope
// is unused by this unit and a fake that served it would invite a handler to
// start using one.
type fakeSecrets struct {
	stored map[string][]byte
	putErr error
	getErr error
	// gets counts user-scoped reads, which is how the constant-shape refusal
	// paths are asserted: they must do the same work as a wrong signature.
	gets int
}

func (s *fakeSecrets) Get(context.Context, string) ([]byte, error) {
	return nil, extension.ErrSecretNotFound
}
func (s *fakeSecrets) Put(context.Context, string, []byte) error { return nil }
func (s *fakeSecrets) Delete(context.Context, string) error      { return nil }

func (s *fakeSecrets) userKey(user extension.UserID, key string) string {
	return string(user) + "/" + key
}

func (s *fakeSecrets) GetUser(_ context.Context, user extension.UserID, key string) ([]byte, error) {
	s.gets++
	if s.getErr != nil {
		return nil, s.getErr
	}
	secret, ok := s.stored[s.userKey(user, key)]
	if !ok {
		return nil, extension.ErrSecretNotFound
	}
	return secret, nil
}

func (s *fakeSecrets) PutUser(_ context.Context, user extension.UserID, key string, secret []byte) error {
	if s.putErr != nil {
		return s.putErr
	}
	s.stored[s.userKey(user, key)] = secret
	return nil
}

func (s *fakeSecrets) DeleteUser(_ context.Context, user extension.UserID, key string) error {
	delete(s.stored, s.userKey(user, key))
	return nil
}

// fakeTx records what a handler asked the database to do and answers with what
// the test scripted.
type fakeTx struct {
	statements []string
	args       [][]any
	open       bool

	// singleRows is what successive QueryRow calls answer, one entry each and
	// CONSUMED in order. A handler that reads a row and then writes one issues
	// two, and a fake that answered the same values to both could not tell a
	// read-then-write from a write alone.
	singleRows [][]any
	// noRows makes the nth QueryRow (1-based) answer the core's empty result,
	// which every read here treats as "there is no such row" rather than as a
	// failure.
	noRows map[int]bool
	// queryRows is what the next Query hands back.
	queryRows [][]any

	err      error
	failFrom int

	audited   []extension.Change
	published []extension.Event
	rowCalls  int
}

func (t *fakeTx) record(sql string, args []any) {
	t.statements = append(t.statements, sql)
	t.args = append(t.args, args)
}

func (t *fakeTx) failure() error {
	if t.err == nil || len(t.statements) < t.failFrom {
		return nil
	}
	return t.err
}

// Core is nil: this unit files nothing through the governed core port, and a
// fake that handed one back would let a handler start using it unnoticed.
func (t *fakeTx) Core() extension.Core { return nil }

func (t *fakeTx) Record(_ context.Context, ch extension.Change, ev extension.Event) error {
	// The published grammar, run here rather than waved through: an entity
	// outside this unit's namespace, an id that is not a UUID, an image that is
	// not JSON and a verb that is not a verb are all refusals the core makes.
	if err := ch.Validate(); err != nil {
		return err
	}
	if err := ev.Validate(); err != nil {
		return err
	}
	t.audited = append(t.audited, ch)
	t.published = append(t.published, ev)
	return nil
}

func (t *fakeTx) Exec(_ context.Context, sql string, args ...any) (int64, error) {
	t.record(sql, args)
	return 0, t.failure()
}

func (t *fakeTx) Query(_ context.Context, sql string, args ...any) (extension.Rows, error) {
	t.record(sql, args)
	if err := t.failure(); err != nil {
		return nil, err
	}
	rows := &fakeRows{rows: t.queryRows}
	t.queryRows = nil
	return rows, nil
}

func (t *fakeTx) QueryRow(_ context.Context, sql string, args ...any) extension.Row {
	t.record(sql, args)
	t.rowCalls++
	if t.noRows[t.rowCalls] {
		return fakeRow{err: extension.ErrNoRows}
	}
	var values []any
	if len(t.singleRows) > 0 {
		values, t.singleRows = t.singleRows[0], t.singleRows[1:]
	}
	return fakeRow{values: values, err: t.failure()}
}

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.values == nil {
		// The PUBLISHED sentinel, not the driver's own wording: the core
		// translates the driver's empty result precisely so a unit does not
		// match on a driver's text, and a fake that invented one would be a
		// suite testing itself.
		return extension.ErrNoRows
	}
	return scanInto(dest, r.values)
}

type fakeRows struct {
	rows   [][]any
	cursor int
	closed bool
	err    error
}

func (r *fakeRows) Next() bool {
	if r.cursor >= len(r.rows) {
		return false
	}
	r.cursor++
	return true
}

func (r *fakeRows) Scan(dest ...any) error { return scanInto(dest, r.rows[r.cursor-1]) }
func (r *fakeRows) Close()                 { r.closed = true }
func (r *fakeRows) Err() error             { return r.err }

// scanInto copies scripted values into a handler's scan destinations. It knows
// only the types this unit's projections actually scan; a new column type is a
// loud failure here rather than a silent zero value.
//
// Every assertion is CHECKED. An unchecked one answers the handler with a zero
// value instead of the mistake — a fixture that scripts an int where the
// projection scans a string reads back "" and the assertion under test passes
// for a reason the test never states.
func scanInto(dest, values []any) error {
	if len(dest) != len(values) {
		return errWidth{want: len(dest), got: len(values)}
	}
	for at, value := range values {
		switch t := dest[at].(type) {
		case *string:
			got, ok := value.(string)
			if !ok {
				return errScripted{at: at, want: "string", got: value}
			}
			*t = got
		case *bool:
			got, ok := value.(bool)
			if !ok {
				return errScripted{at: at, want: "bool", got: value}
			}
			*t = got
		case *int:
			got, ok := value.(int)
			if !ok {
				return errScripted{at: at, want: "int", got: value}
			}
			*t = got
		case *[]byte:
			got, ok := value.([]byte)
			if !ok {
				return errScripted{at: at, want: "[]byte", got: value}
			}
			*t = got
		case *int64:
			got, ok := value.(int64)
			if !ok {
				return errScripted{at: at, want: "int64", got: value}
			}
			*t = got
		case *time.Time:
			got, ok := value.(time.Time)
			if !ok {
				return errScripted{at: at, want: "time.Time", got: value}
			}
			*t = got
		case **time.Time:
			// A TIME, because the column is timestamptz and the driver refuses
			// to scan one into a string. The fake takes the type the handler
			// asks for, so a projection that goes back to text fails here
			// rather than in production.
			if value == nil {
				*t = nil
				continue
			}
			got, ok := value.(time.Time)
			if !ok {
				return errScripted{at: at, want: "time.Time", got: value}
			}
			*t = &got
		default:
			return errScripted{at: at, want: "a type the projection scans", got: dest[at]}
		}
	}
	return nil
}

type errWidth struct{ want, got int }

func (e errWidth) Error() string {
	return fmt.Sprintf("the scripted row is the wrong width for the projection: it scans %d columns and the row has %d — the order is the projection constant", e.want, e.got)
}

// errScripted is a fixture that scripted the wrong TYPE for a column. It names
// the position rather than the column, because that is what a caller can act
// on: the scripted rows are built in projection order, so the position is the
// column.
type errScripted struct {
	at   int
	want string
	got  any
}

func (e errScripted) Error() string {
	return fmt.Sprintf("column %d of the scripted row is a %T, and the projection scans it as %s — count along the projection constant to find it",
		e.at+1, e.got, e.want)
}

// endpointRow scripts one row of endpointColumns, in that order, so a column
// added to the projection is ONE edit in the fixtures rather than one per
// scripted row.
func endpointRow(id, userID, url string, enabled bool) []any {
	return []any{id, userID, inboundSlug, ownerRef, url, enabled, int64(0), int64(0), nil, nil, 1}
}

// jsonOf decodes a handler's answer, failing the test rather than the caller.
func jsonOf[T any](tb testing.TB, raw json.RawMessage) T {
	tb.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		tb.Fatalf("the answer does not decode: %v\n%s", err, raw)
	}
	return out
}

// statementMentioning finds the one statement a test is about, so an assertion
// does not depend on how many others the handler issued around it.
//
// The needles callers pass are deliberately NOT statement openers with a table
// after them ("ON CONFLICT", not "INSERT INTO"): the tree's SQL-scope gate reads
// string literals looking for the table a statement names, and a needle shaped
// like the start of one reads as a statement whose table it cannot resolve.
func (t *fakeTx) statementMentioning(tb testing.TB, needle string) (string, []any) {
	tb.Helper()
	for i, sql := range t.statements {
		if strings.Contains(sql, needle) {
			return sql, t.args[i]
		}
	}
	tb.Fatalf("no statement mentions %q; the handler issued:\n%s", needle, strings.Join(t.statements, "\n---\n"))
	return "", nil
}

// SyncNow answers for the one job this unit declares and refuses every other
// name, which is the core's rule: a name is resolved against the CALLING
// unit's declarations, so a unit cannot ask for a job it does not own. A fake
// that accepted any string would let a handler reach for a name that fails at
// run time and still pass here.
func (*fakeRuntime) SyncNow(_ context.Context, job extension.JobName) error {
	if job != declaredJob {
		return extension.ErrNoSuchJob
	}
	return nil
}

// declaredJob is the job named in api/jobs.yaml, spelled here so the fake
// refuses exactly what the core refuses.
const declaredJob = extension.JobName("poll_inbox")
