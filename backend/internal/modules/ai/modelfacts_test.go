// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Callers arriving while the catalog is being read share that one read, and a
// caller who gives up leaves the read standing for the others.
func TestACatalogReadInFlightIsJoinedNotRepeated(t *testing.T) {
	facts := &catalogFact[string]{now: time.Now}
	var asks atomic.Int32
	arrived, release := make(chan struct{}), make(chan struct{})
	ask := func(context.Context) (string, error) {
		if asks.Add(1) == 1 {
			close(arrived)
		}
		<-release
		return "catalog", nil
	}

	impatient, giveUp := context.WithCancel(context.Background())
	gaveUp := make(chan error, 1)
	go func() {
		_, err := facts.get(impatient, ask)
		gaveUp <- err
	}()
	<-arrived
	giveUp()
	if err := <-gaveUp; !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller who gave up got %v, want its own cancellation", err)
	}

	answers := make([]string, 3)
	var wg sync.WaitGroup
	for i := range answers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			answers[i], _ = facts.get(context.Background(), ask)
		}()
	}
	close(release)
	wg.Wait()
	for _, answer := range answers {
		if answer != "catalog" {
			t.Fatalf("a joining caller got %q, want the one read's answer", answer)
		}
	}
	if n := asks.Load(); n != 1 {
		t.Fatalf("the catalog was read %d times, want 1", n)
	}
}

// A list past the bound is refused by name rather than cut to a prefix.
func TestAModelListPastItsBoundIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(" ", listBodyLimit+1)))
	}))
	t.Cleanup(srv.Close)
	_, err := getListBody(context.Background(), srv.Client(), "openai-compat", srv.URL, func(*http.Request) {})
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("an oversized list = %v, want it refused as too large", err)
	}
}
