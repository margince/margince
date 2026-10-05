// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// fakeHealthStore is an in-memory ProviderHealthStore. Each write is announced
// on writes after it lands, so a test waits on the write rather than sleeping.
type fakeHealthStore struct {
	mu      sync.Mutex
	rows    map[string]model.ProviderHealthStatus
	writes  chan string
	gate    chan struct{} // when set, every write blocks until it is closed
	loadErr error
	putErr  error
}

func newFakeHealthStore() *fakeHealthStore {
	return &fakeHealthStore{rows: map[string]model.ProviderHealthStatus{}, writes: make(chan string, 16)}
}

func (f *fakeHealthStore) wait(t *testing.T) string {
	t.Helper()
	select {
	case w := <-f.writes:
		return w
	case <-time.After(5 * time.Second):
		t.Fatal("the shared status was never written")
		return ""
	}
}

func (f *fakeHealthStore) Publish(_ context.Context, provider string, st model.ProviderHealthStatus) error {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		f.writes <- "failed " + provider
		return f.putErr
	}
	f.rows[provider] = st
	f.writes <- "publish " + provider
	return nil
}

func (f *fakeHealthStore) Clear(_ context.Context, provider string) error {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, provider)
	f.writes <- "clear " + provider
	return nil
}

func (f *fakeHealthStore) Load(context.Context) (map[string]model.ProviderHealthStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	out := map[string]model.ProviderHealthStatus{}
	for k, v := range f.rows {
		out[k] = v
	}
	return out, nil
}

func sharedBook(store ProviderHealthStore) (*providerBook, *healthClock) {
	clock := newClock()
	book := newProviderBook(clock.now)
	book.sharer.use(store)
	return book, clock
}

func TestATripAndARecoveryAreSharedAndADegradedSpellIsToo(t *testing.T) {
	t.Parallel()
	store := newFakeHealthStore()
	book, clock := sharedBook(store)
	tr := book.tracker("openai")

	tr.observe(admission{}, ErrProviderQuota)
	if got := store.wait(t); got != "publish openai" {
		t.Fatalf("a trip wrote %q", got)
	}
	if st := store.rows["openai"]; st.Health != model.HealthOutOfCredit || st.RetryAfter.IsZero() {
		t.Fatalf("shared status = %+v, want out_of_credit with a retry moment", st)
	}
	clock.advance(accountReprobe)
	_, probe, _ := tr.admit()
	tr.observe(probe, nil)
	if got := store.wait(t); got != "clear openai" {
		t.Fatalf("a recovery wrote %q", got)
	}

	slow := book.tracker("gemini")
	for range consecutiveToTrip {
		slow.observe(admission{}, context.DeadlineExceeded)
	}
	store.wait(t)
	if st := store.rows["gemini"]; st.Health != model.HealthDegraded {
		t.Fatalf("shared status = %+v, want degraded", st)
	}
}

func TestAnUnchangedHealthyProviderWritesNothing(t *testing.T) {
	t.Parallel()
	store := newFakeHealthStore()
	book, _ := sharedBook(store)
	book.tracker("openai").observe(admission{}, nil)
	book.tracker("openai").observe(admission{}, errors.New("a message the provider refused"))
	book.tracker("anthropic").observe(admission{}, ErrProviderUnauthorized)
	if got := store.wait(t); got != "publish anthropic" {
		t.Fatalf("first write = %q: a provider that never failed must not be written", got)
	}
}

func TestForgetClearsTheSharedKeyEvenWhenThisProcessNeverSawTheFault(t *testing.T) {
	t.Parallel()
	store := newFakeHealthStore()
	store.rows["openai"] = model.ProviderHealthStatus{Health: model.HealthDown, Since: time.Now()}
	book, _ := sharedBook(store)

	book.forget("openai")

	if got := store.wait(t); got != "clear openai" {
		t.Fatalf("forget wrote %q", got)
	}
	if len(store.rows) != 0 {
		t.Fatalf("shared rows = %v, want none", store.rows)
	}
}

func TestSharingNeverMakesTheCallPathWait(t *testing.T) {
	t.Parallel()
	store := newFakeHealthStore()
	store.gate = make(chan struct{})
	book, _ := sharedBook(store)

	done := make(chan struct{})
	go func() {
		book.tracker("openai").observe(admission{}, ErrProviderQuota)
		book.forget("openai")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("observe waited on the shared store")
	}
	close(store.gate)
	store.wait(t)
}

func TestTheReportMergesProcessesAndPrefersTheBlockingThenTheNewest(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	local := []ProviderHealthEntry{
		{Provider: "gemini", Status: model.ProviderHealthStatus{Health: model.HealthDegraded, Since: base}},
		{Provider: "mistral", Status: model.ProviderHealthStatus{Health: model.HealthDown, Since: base}},
		{Provider: "xai", Status: model.ProviderHealthStatus{Health: model.HealthDown, Since: base}},
	}
	shared := map[string]model.ProviderHealthStatus{
		"anthropic": {Health: model.HealthDown, Since: base},
		"gemini":    {Health: model.HealthDown, Since: base.Add(-time.Hour)},
		"mistral":   {Health: model.HealthDegraded, Since: base.Add(time.Minute)},
		"xai":       {Health: model.HealthUnauthorized, Since: base.Add(time.Minute)},
	}
	got := mergeProviderHealth(local, shared)
	want := map[string]model.ProviderHealth{
		"anthropic": model.HealthDown, "gemini": model.HealthDown,
		"mistral": model.HealthDown, "xai": model.HealthUnauthorized,
	}
	if len(got) != len(want) {
		t.Fatalf("merged = %+v", got)
	}
	for i, e := range got {
		if e.Status.Health != want[e.Provider] {
			t.Errorf("%s = %s, want %s", e.Provider, e.Status.Health, want[e.Provider])
		}
		if i > 0 && got[i-1].Provider >= e.Provider {
			t.Errorf("not sorted by name: %v before %v", got[i-1].Provider, e.Provider)
		}
	}
}

func TestAWorkerOnlyOutageShowsInAnotherProcessesReport(t *testing.T) {
	t.Parallel()
	store := newFakeHealthStore()
	worker, _ := sharedBook(store)
	api, _ := sharedBook(store)

	worker.tracker("openai").observe(admission{}, ErrProviderQuota)
	store.wait(t)

	got := api.report(t.Context())
	if len(got) != 1 || got[0].Provider != "openai" || got[0].Status.Health != model.HealthOutOfCredit {
		t.Fatalf("api report = %+v, want the worker's out_of_credit", got)
	}
	api.forget("openai")
	store.wait(t)
	if got := api.report(t.Context()); len(got) != 0 {
		t.Fatalf("after the fix the report = %+v, want none", got)
	}
}

func TestARedisFaultIsLoggedOnceAndTheLocalViewStillAnswers(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	store := newFakeHealthStore()
	store.loadErr = errors.New("connection refused")
	store.putErr = errors.New("connection refused")
	book, _ := sharedBook(store)
	book.tracker("openai").observe(admission{}, ErrProviderQuota)
	book.tracker("anthropic").observe(admission{}, ErrProviderUnauthorized)
	store.wait(t)
	store.wait(t)

	for range 3 {
		if got := book.report(t.Context()); len(got) != 2 {
			t.Fatalf("report = %+v, want both local entries despite Redis being down", got)
		}
	}
	if n := strings.Count(logs.String(), "could not be read"); n != 1 {
		t.Errorf("read failure logged %d times, want once per run:\n%s", n, logs.String())
	}
	if n := strings.Count(logs.String(), "could not be written"); n != 1 {
		t.Errorf("write failure logged %d times, want once per run:\n%s", n, logs.String())
	}
}

func TestABookWithNoStoreStaysLocal(t *testing.T) {
	t.Parallel()
	book := newProviderBook(newClock().now)
	book.tracker("openai").observe(admission{}, ErrProviderQuota)
	book.forget("anthropic")
	if got := book.report(t.Context()); len(got) != 1 || got[0].Provider != "openai" {
		t.Fatalf("report = %+v", got)
	}
}

func TestAProviderClearedByAnotherProcessIsClearedHereToo(t *testing.T) {
	t.Parallel()
	store := newFakeHealthStore()
	book, clock := sharedBook(store)
	tr := book.tracker("openai")
	tr.observe(admission{}, ErrProviderQuota)
	store.wait(t)
	if err := store.Clear(context.Background(), "openai"); err != nil {
		t.Fatalf("clearing the shared record: %v", err)
	}
	store.wait(t)

	book.reconcile(context.Background())
	if tr.current().Health != model.HealthOutOfCredit {
		t.Fatal("a status published a moment ago was read as cleared: it may simply not have reached the store")
	}
	clock.advance(shareSettle)
	book.reconcile(context.Background())
	if h := tr.current().Health; h != model.HealthOK {
		t.Errorf("health %s after another process cleared the record, want ok", h)
	}
}

func TestAnUnreachableStoreIsNotReadAsACleared(t *testing.T) {
	t.Parallel()
	store := newFakeHealthStore()
	book, clock := sharedBook(store)
	tr := book.tracker("openai")
	tr.observe(admission{}, ErrProviderQuota)
	store.wait(t)
	clock.advance(shareSettle)
	store.loadErr = errors.New("redis down")
	book.reconcile(context.Background())
	if tr.current().Health != model.HealthOutOfCredit {
		t.Error("an unreachable store cleared a blocked provider")
	}
}

func TestAnIdleUnhealthyProviderIsRepublishedBeforeTheStoreExpiresIt(t *testing.T) {
	t.Parallel()
	store := newFakeHealthStore()
	book, clock := sharedBook(store)
	tr := book.tracker("openai")
	tr.observe(admission{}, ErrProviderQuota)
	store.wait(t)
	clock.advance(shareRefresh)
	book.reconcile(context.Background())
	if got := store.wait(t); got != "publish openai" {
		t.Errorf("the refresh wrote %q, want a publish", got)
	}
}
