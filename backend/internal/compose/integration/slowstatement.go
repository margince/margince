// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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
	var report string
	// The watcher shares conn, which is not safe for two goroutines. Cleanups
	// run last-in first-out, so this stops it before the conn is closed.
	stop := func() { once.Do(func() { cancel(); report = (<-seen).describe(conn) }) }
	t.Cleanup(stop)
	return func() string {
		stop()
		return report
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
		var next slowSeen
		err := conn.QueryRow(probe, `SELECT query, extract(epoch FROM now() - query_start)::float8,
		  octet_length(query) >= pg_size_bytes(current_setting('track_activity_query_size')) - 1
		  FROM pg_stat_activity WHERE datname = current_database() AND state = 'active'
		  AND pid <> pg_backend_pid() AND now() - query_start > make_interval(secs => $1)
		  ORDER BY query_start LIMIT 1`, after.Seconds()).Scan(&next.query, &next.seconds, &next.cut)
		done()
		if err == nil && next.seconds > longest.seconds {
			longest = next
		}
	}
}

type slowSeen struct {
	query   string
	seconds float64
	cut     bool
}

func (s slowSeen) describe(conn *pgx.Conn) string {
	if s.query == "" {
		return "no statement was seen running long"
	}
	head := fmt.Sprintf("ran %.1fs: %s", s.seconds, s.query)
	switch {
	case s.cut:
		return head + "\n(not explained: the text is cut at track_activity_query_size)"
	case strings.Contains(s.query, ";"):
		return head + "\n(not explained: the text holds more than one statement)"
	}
	// Sent raw: the text holds $n placeholders that GENERIC_PLAN leaves unbound,
	// and pgx would refuse them with no arguments.
	results, err := conn.PgConn().Exec(context.Background(), "EXPLAIN (GENERIC_PLAN) "+s.query).ReadAll()
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
