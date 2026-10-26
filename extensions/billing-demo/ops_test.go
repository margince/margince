// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package billingdemo

// The handlers and the sync run here against a scripted runtime: it answers what
// the unit's statements ask and records what the unit did. Whether the SQL is
// right against the real schema is the migration gate's question.

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

const (
	memberID   = "01a17608-0c4a-7086-b086-1caba78f6630"
	pipelineID = "11111111-1111-4111-8111-111111111111"
	openID     = "22222222-2222-4222-8222-222222222222"
	wonID      = "33333333-3333-4333-8333-333333333333"
)

type fakeRows struct {
	rows [][]any
	at   int
}

func (r *fakeRows) Next() bool { r.at++; return r.at <= len(r.rows) }
func (r *fakeRows) Err() error { return nil }
func (r *fakeRows) Close()     {}
func (r *fakeRows) Scan(dest ...any) error {
	for i, d := range dest {
		reflect.ValueOf(d).Elem().Set(reflect.ValueOf(r.rows[r.at-1][i]))
	}
	return nil
}

type fakeRow struct {
	vals []any
	err  error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return (&fakeRows{rows: [][]any{r.vals}, at: 1}).Scan(dest...)
}

// fakeTx answers by statement prefix and records every statement it ran.
type fakeTx struct {
	extension.Tx
	execs    []string
	affected int64
	conn     fakeRow
	pending  [][]any
	status   [][]any
	execErr  error
}

func (t *fakeTx) Exec(_ context.Context, sql string, _ ...any) (int64, error) {
	t.execs = append(t.execs, sql)
	return t.affected, t.execErr
}

func (t *fakeTx) QueryRow(context.Context, string, ...any) extension.Row { return t.conn }

func (t *fakeTx) Query(_ context.Context, sql string, _ ...any) (extension.Rows, error) {
	if strings.Contains(sql, "synced_paid IS NULL") {
		return &fakeRows{rows: t.pending}, nil
	}
	return &fakeRows{rows: t.status}, nil
}

type fakeSecrets struct {
	extension.Secrets
	filed []string
}

func (s *fakeSecrets) PutUser(_ context.Context, u extension.UserID, key string, _ []byte) error {
	s.filed = append(s.filed, string(u)+"/"+key)
	return nil
}

type fakeRuntime struct {
	extension.Runtime
	caller   extension.Caller
	tx       *fakeTx
	secrets  *fakeSecrets
	synced   []extension.JobName
	ingested []extension.Record
	result   extension.Result
	ingErr   error
	txErr    error
}

func newRuntime() *fakeRuntime {
	return &fakeRuntime{
		caller:  extension.Caller{Type: extension.CallerHuman, UserID: memberID},
		tx:      &fakeTx{affected: 1},
		secrets: &fakeSecrets{},
	}
}

func (r *fakeRuntime) Caller() extension.Caller   { return r.caller }
func (r *fakeRuntime) Secrets() extension.Secrets { return r.secrets }
func (r *fakeRuntime) SyncNow(_ context.Context, j extension.JobName) error {
	r.synced = append(r.synced, j)
	return nil
}

func (r *fakeRuntime) Tx(ctx context.Context, fn func(context.Context, extension.Tx) error) error {
	if r.txErr != nil {
		return r.txErr
	}
	return fn(ctx, r.tx)
}

func (r *fakeRuntime) Ingest(_ context.Context, _ extension.UserID, rec extension.Record) (extension.Result, error) {
	r.ingested = append(r.ingested, rec)
	return r.result, r.ingErr
}

func connectInput() json.RawMessage {
	return json.RawMessage(`{"member_id":"` + memberID + `","pipeline_id":"` + pipelineID +
		`","open_stage_id":"` + openID + `","won_stage_id":"` + wonID + `"}`)
}

func TestEveryOperationRefusesANonHumanCaller(t *testing.T) {
	for name, call := range map[string]func(*fakeRuntime) error{
		"connect": func(r *fakeRuntime) error { _, err := connect(context.Background(), r, connectInput()); return err },
		"update": func(r *fakeRuntime) error {
			_, err := updateInvoice(context.Background(), r, json.RawMessage(`{}`))
			return err
		},
		"status": func(r *fakeRuntime) error { _, err := status(context.Background(), r, nil); return err },
	} {
		for _, c := range []extension.Caller{{Type: extension.CallerSystem}, {Type: extension.CallerHuman, IsAgent: true}} {
			r := newRuntime()
			r.caller = c
			if err := call(r); !errors.Is(err, extension.ErrForbidden) {
				t.Errorf("%s as %+v = %v, want forbidden", name, c, err)
			}
		}
	}
}

func TestConnectSeedsTheInvoicesFilesTheSecretAndSyncs(t *testing.T) {
	r := newRuntime()
	if _, err := connect(context.Background(), r, connectInput()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if got, want := len(r.tx.execs), 1+len(sampleInvoices); got != want {
		t.Errorf("statements = %d, want %d", got, want)
	}
	if len(r.secrets.filed) != 1 || r.secrets.filed[0] != memberID+"/feed" {
		t.Errorf("secrets filed = %v, want the feed under the member", r.secrets.filed)
	}
	if len(r.synced) != 1 || r.synced[0] != jobSync {
		t.Errorf("synced = %v, want one sync", r.synced)
	}
}

func TestConnectRefusesBadArgumentsAndAFailingStore(t *testing.T) {
	r := newRuntime()
	if _, err := connect(context.Background(), r, json.RawMessage(`{"member_id":"x"}`)); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a bad id = %v, want invalid", err)
	}
	if _, err := connect(context.Background(), r, json.RawMessage(`not json`)); err == nil {
		t.Error("malformed json was accepted")
	}
	r.tx.execErr = errors.New("boom")
	if _, err := connect(context.Background(), r, connectInput()); err == nil {
		t.Error("a failing store was swallowed")
	}
	if len(r.secrets.filed) != 0 || len(r.synced) != 0 {
		t.Error("a failed connect filed a secret or asked for a sync")
	}
}

func TestUpdateInvoiceNeedsAChangeAndAKnownInvoice(t *testing.T) {
	for name, in := range map[string]string{
		"nothing named":   `{"erp_id":"INV-1"}`,
		"negative amount": `{"erp_id":"INV-1","amount_minor":-1}`,
	} {
		if _, err := updateInvoice(context.Background(), newRuntime(), json.RawMessage(in)); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("%s = %v, want invalid", name, err)
		}
	}
	r := newRuntime()
	r.tx.affected = 0
	if _, err := updateInvoice(context.Background(), r, json.RawMessage(`{"erp_id":"NOPE","paid":true}`)); !errors.Is(err, extension.ErrNotFound) {
		t.Errorf("an unknown invoice = %v, want not found", err)
	}
	r = newRuntime()
	if _, err := updateInvoice(context.Background(), r, json.RawMessage(`{"erp_id":"INV-1","paid":true}`)); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(r.synced) != 1 {
		t.Errorf("synced = %v, want one sync after an update", r.synced)
	}
	r.txErr = errors.New("boom")
	if _, err := updateInvoice(context.Background(), r, json.RawMessage(`{"erp_id":"INV-1","paid":true}`)); err == nil {
		t.Error("a failing transaction was swallowed")
	}
	if _, err := updateInvoice(context.Background(), r, json.RawMessage(`not json`)); err == nil {
		t.Error("malformed json was accepted")
	}
}

func TestStatusListsInvoicesAndAnEmptyConnectionIsNotAnError(t *testing.T) {
	r := newRuntime()
	r.tx.status = [][]any{{"INV-1", "Acme", "EUR", int64(5000), true, true, "deal-1", []string{"amount"}}}
	out, err := status(context.Background(), r, nil)
	if err != nil || !strings.Contains(string(out), `"erp_id":"INV-1"`) || !strings.Contains(string(out), `"withheld":["amount"]`) {
		t.Fatalf("status = %s, %v", out, err)
	}
	r.txErr = extension.ErrNoRows
	if out, err = status(context.Background(), r, nil); err != nil || !strings.Contains(string(out), `"invoices":[]`) {
		t.Errorf("no rows = %s, %v, want an empty list", out, err)
	}
	r.txErr = errors.New("boom")
	if _, err = status(context.Background(), r, nil); err == nil {
		t.Error("a failing transaction was swallowed")
	}
}

func syncRuntime() *fakeRuntime {
	r := newRuntime()
	r.caller = extension.Caller{Type: extension.CallerSystem}
	r.tx.conn = fakeRow{vals: []any{memberID, pipelineID, openID, wonID}}
	r.tx.pending = [][]any{{"INV-1", "Acme", "EUR", int64(5000), true}}
	r.result = extension.Result{Ref: extension.Ref{ID: "deal-1"}}
	return r
}

func TestTheSyncOffersEachPendingInvoiceAndRecordsTheAnswer(t *testing.T) {
	r := syncRuntime()
	if err := syncTick(context.Background(), r); err != nil {
		t.Fatalf("syncTick: %v", err)
	}
	if len(r.ingested) != 1 || r.ingested[0].Deal.StageID != wonID {
		t.Errorf("ingested = %+v, want one paid invoice on the won stage", r.ingested)
	}
	if len(r.tx.execs) != 1 {
		t.Errorf("answers recorded = %d, want 1", len(r.tx.execs))
	}
}

func TestTheSyncDoesNothingBeforeTheConnectionExists(t *testing.T) {
	r := syncRuntime()
	r.tx.conn = fakeRow{err: extension.ErrNoRows}
	if err := syncTick(context.Background(), r); err != nil || len(r.ingested) != 0 {
		t.Errorf("no connection = %v with %d ingested, want a quiet no-op", err, len(r.ingested))
	}
}

func TestTheSyncFailsWithItsClassOnEveryRefusal(t *testing.T) {
	cases := map[string]func(*fakeRuntime){
		"store":      func(r *fakeRuntime) { r.txErr = errors.New("boom") },
		"core error": func(r *fakeRuntime) { r.ingErr = errors.New("refused") },
		"unrepresentable": func(r *fakeRuntime) {
			r.result = extension.Result{Disposition: extension.DispositionUnrepresentable, Reason: "x"}
		},
		"answer": func(r *fakeRuntime) { r.tx.execErr = errors.New("boom") },
	}
	for name, arrange := range cases {
		r := syncRuntime()
		arrange(r)
		err := syncTick(context.Background(), r)
		if err == nil {
			t.Errorf("%s: the sync succeeded", name)
		}
	}
}
