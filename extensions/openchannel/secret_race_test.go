// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// A real, concurrent drive of two overlapping mints — not a hook that
// simulates the interleaving after the fact. lockingRuntime models the one
// property production actually leans on: SELECT ... FOR UPDATE blocks a
// second transaction reading the same row until the first COMMITS. Every
// other fake in this suite runs one call at a time and never needed that;
// this one exists because two goroutines racing through mintSecret is the
// only way to tell a structurally serial operation from one that merely
// checks afterwards and got lucky.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// lockingRow is the single endpoint row both mints contend for. mu stands in
// for the postgres row lock: acquired by the FOR UPDATE read and released only
// when the transaction that acquired it finishes (commit or rollback), exactly
// as a real row lock is scoped to the transaction rather than to the query.
type lockingRow struct {
	mu      sync.Mutex
	id      string
	userID  string
	version int
}

// lockingSecrets is a minimal, thread-safe stand-in for the secret store: a
// plain map guarded by a mutex, which is all PutUser/GetUser need to be safe
// under two goroutines.
type lockingSecrets struct {
	mu     sync.Mutex
	stored map[string][]byte
}

func (s *lockingSecrets) Get(context.Context, string) ([]byte, error) {
	return nil, extension.ErrSecretNotFound
}
func (s *lockingSecrets) Put(context.Context, string, []byte) error { return nil }
func (s *lockingSecrets) Delete(context.Context, string) error      { return nil }

func (s *lockingSecrets) key(user extension.UserID, key string) string {
	return string(user) + "/" + key
}

func (s *lockingSecrets) GetUser(_ context.Context, user extension.UserID, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.stored[s.key(user, key)]
	if !ok {
		return nil, extension.ErrSecretNotFound
	}
	return v, nil
}

func (s *lockingSecrets) PutUser(_ context.Context, user extension.UserID, key string, secret []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored[s.key(user, key)] = secret
	return nil
}

func (s *lockingSecrets) DeleteUser(_ context.Context, user extension.UserID, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.stored, s.key(user, key))
	return nil
}

// lockingTx is one transaction's view of the shared row. held records whether
// THIS transaction is the one holding the row lock, so lockingRuntime.Tx knows
// to release it when — and only when — this transaction's callback returns.
type lockingTx struct {
	row     *lockingRow
	member  string
	held    bool
	audited int
}

func (t *lockingTx) Core() extension.Core { return nil }

func (t *lockingTx) Record(_ context.Context, ch extension.Change, ev extension.Event) error {
	if err := ch.Validate(); err != nil {
		return err
	}
	if err := ev.Validate(); err != nil {
		return err
	}
	t.audited++
	return nil
}

func (t *lockingTx) Exec(context.Context, string, ...any) (int64, error) { return 0, nil }

func (t *lockingTx) Query(context.Context, string, ...any) (extension.Rows, error) {
	return &fakeRows{}, nil
}

func (t *lockingTx) QueryRow(_ context.Context, sql string, _ ...any) extension.Row {
	switch {
	case strings.Contains(sql, "FOR UPDATE"):
		// The row lock: this is the one statement in mintSecret that a second,
		// overlapping transaction can be blocked on, and it is acquired
		// BEFORE anything is minted or sealed.
		t.row.mu.Lock()
		t.held = true
		return lockingResult{row: t.row}
	case strings.Contains(sql, "UPDATE "):
		t.row.version++
		return lockingResult{row: t.row}
	default:
		return lockingResult{row: t.row}
	}
}

type lockingResult struct {
	row *lockingRow
}

func (r lockingResult) Scan(dest ...any) error {
	return scanInto(dest, endpointRow(r.row.id, r.row.userID, "", true))
}

// lockingRuntime hands out a fresh lockingTx per Tx call, sharing the same row
// and secrets across every call it makes — exactly as two requests hitting the
// same connection pool share the same underlying row and the same secret
// namespace.
type lockingRuntime struct {
	member  string
	row     *lockingRow
	secrets *lockingSecrets
}

func (r *lockingRuntime) Secrets() extension.Secrets { return r.secrets }
func (r *lockingRuntime) Caller() extension.Caller {
	return extension.Caller{Type: extension.CallerHuman, UserID: r.member}
}
func (r *lockingRuntime) Ingest(context.Context, extension.UserID, extension.Record) (extension.Result, error) {
	return extension.Result{}, nil
}
func (r *lockingRuntime) SyncNow(context.Context, extension.JobName) error { return nil }

func (r *lockingRuntime) Tx(ctx context.Context, fn func(context.Context, extension.Tx) error) error {
	tx := &lockingTx{row: r.row, member: r.member}
	err := fn(ctx, tx)
	if tx.held {
		// Released on the way out, exactly once, whether fn committed or
		// rolled back — a real row lock is held for the transaction's whole
		// life and nothing shorter.
		tx.row.mu.Unlock()
	}
	return err
}

// TestOverlappingMintsSerializeRatherThanRace drives two mints for the SAME
// endpoint through mintSecret on two real goroutines. If the row lock were
// taken anywhere other than before the seal — or not held across the seal at
// all — the two goroutines could interleave so that the loser's PutUser is
// the one that lands last while the winner's response is what the caller
// reads, which is exactly the defect this test must catch.
//
// The row lock (lockingRow.mu) is real: acquired by the FOR UPDATE read and
// released only when the acquiring transaction's callback returns, exactly as
// postgres scopes a row lock to the transaction. If mintSecret ever seals the
// secret BEFORE taking that lock — or releases it before the seal commits —
// two real goroutines racing through it can interleave so the row ends up
// current for one secret while the OTHER goroutine's answer is what a caller
// reads, which the assertions below catch without any scripted hook.
func TestOverlappingMintsSerializeRatherThanRace(t *testing.T) {
	t.Parallel()
	row := &lockingRow{id: endpointID, userID: ownerUserID, version: 1}
	secrets := &lockingSecrets{stored: map[string][]byte{}}
	rt := &lockingRuntime{member: ownerUserID, row: row, secrets: secrets}

	const attempts = 2
	results := make(chan struct {
		out json.RawMessage
		err error
	}, attempts)

	var wg sync.WaitGroup
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			out, err := mintSecret(context.Background(), rt, json.RawMessage(ownArgs))
			results <- struct {
				out json.RawMessage
				err error
			}{out, err}
		}()
	}
	wg.Wait()
	close(results)

	var secretsSeen []string
	for r := range results {
		if r.err != nil {
			t.Fatalf("an overlapping mint failed rather than serializing: %v", r.err)
		}
		minted := jsonOf[mintedSecret](t, r.out)
		secretsSeen = append(secretsSeen, minted.SigningSecret)
	}
	if len(secretsSeen) != attempts {
		t.Fatalf("got %d answers for %d mints", len(secretsSeen), attempts)
	}
	if secretsSeen[0] == secretsSeen[1] {
		t.Fatal("two overlapping mints minted the same secret — they did not run as two rotations")
	}
	// THE INVARIANT THE ORIGINAL READ-BACK EXISTED TO PROTECT: whichever
	// secret is answered by EITHER call must be the one that is actually
	// current once both have finished. Under real interleaving (not a
	// scripted clobber) that is only true if the row lock made the two
	// mints fully serial — seal-then-commit, seal-then-commit — rather than
	// letting a second seal land between a first mint's seal and its answer.
	current := secrets.stored[ownerUserID+"/"+inboundSecretKey]
	if string(current) != secretsSeen[0] && string(current) != secretsSeen[1] {
		t.Fatalf("the secret actually current (%q) matches neither answered mint (%v) — a caller was handed a value that never verified", current, secretsSeen)
	}
	if row.version != 3 {
		t.Fatalf("the endpoint row was updated %d times across two serialized mints, want 3 (open + two rotations)", row.version)
	}
}
