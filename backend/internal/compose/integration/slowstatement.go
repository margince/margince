// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// SlowStatement watches this database for the longest statement running past
// after. The answer names it with its generic plan, so a timeout that only a
// loaded runner meets says which statement it was. It reads pg_stat_activity
// and nothing else, so it never changes what a test asserts.
func SlowStatement(t *testing.T, after time.Duration) func() string {
	t.Helper()
	conn := FreshOwnerConn(t)
	ctx, cancel := context.WithCancel(context.Background())
	seen := make(chan slowSeen, 1)
	go watchSlow(ctx, conn, after, seen)
	var once sync.Once
	var longest slowSeen
	// The watcher shares conn, which is not safe for two goroutines. Cleanups
	// run last-in first-out, so this stops it before the conn is closed.
	stop := func() { once.Do(func() { cancel(); longest = <-seen }) }
	t.Cleanup(stop)
	return func() string {
		stop()
		return longest.describe(conn, after)
	}
}

func watchSlow(ctx context.Context, conn *pgx.Conn, after time.Duration, seen chan<- slowSeen) {
	ticker := time.NewTicker(after / 4)
	defer ticker.Stop()
	var longest slowSeen
	for {
		select {
		case <-ctx.Done():
			seen <- longest
			return
		case <-ticker.C:
		}
		// Its own deadline, so stopping the watch never cuts a read in half.
		probe, done := context.WithTimeout(context.WithoutCancel(ctx), after/4)
		found, err := testdb.LongestRunning(probe, conn, after)
		next := slowSeen{query: found.Query, seconds: found.Seconds, cut: found.Cut, waitsOn: found.WaitsOn, blockers: found.Blockers}
		done()
		switch {
		case err == nil && next.seconds > longest.seconds:
			next.probeErr = longest.probeErr
			longest = next
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			longest.probeErr = err.Error()
		}
	}
}

type slowSeen struct {
	query   string
	seconds float64
	cut     bool
	// waitsOn is the statement's wait event when seen, and empty while it runs.
	// blockers counts the sessions holding a lock it needs. Without them, a
	// statement stalled on a lock looks like one that is slow to run.
	waitsOn  string
	blockers int
	// probeErr is the last check that failed, so a watch that saw nothing
	// because every check failed does not read as a quiet database.
	probeErr string
}

func (s slowSeen) describe(conn *pgx.Conn, limit time.Duration) string {
	if s.query == "" && s.probeErr != "" {
		return "no statement was seen running long, and the last check failed: " + s.probeErr
	}
	if s.query == "" {
		return "no statement was seen running long"
	}
	head := fmt.Sprintf("ran %.1fs (wait event %q, %d blocking pids): %s", s.seconds, s.waitsOn, s.blockers, s.query)
	switch {
	case s.cut:
		return head + "\n(not explained: the text is cut at track_activity_query_size)"
	case strings.Contains(s.query, ";"):
		return head + "\n(not explained: the text holds more than one statement)"
	}
	// Sent raw: the text holds $n placeholders that GENERIC_PLAN leaves unbound,
	// and pgx would refuse them with no arguments.
	// Planning locks each relation the statement reads, so a held lock must not stall the report.
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	results, err := conn.PgConn().Exec(ctx, "EXPLAIN (GENERIC_PLAN) "+s.query).ReadAll()
	if err != nil {
		return fmt.Sprintf("%s\n(no plan: %v)", head, err)
	}
	var plan []string
	for _, result := range results {
		for _, row := range result.Rows {
			plan = append(plan, string(row[0]))
		}
	}
	return head + "\nplan:\n" + strings.Join(plan, "\n")
}
