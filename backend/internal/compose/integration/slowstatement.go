// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
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
	go func() {
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
			var next slowSeen
			err := conn.QueryRow(ctx, `SELECT query, extract(epoch FROM now() - query_start)::float8
			  FROM pg_stat_activity WHERE datname = current_database() AND state = 'active'
			  AND pid <> pg_backend_pid() AND now() - query_start > make_interval(secs => $1)
			  ORDER BY query_start LIMIT 1`, after.Seconds()).Scan(&next.query, &next.seconds)
			if err == nil && next.seconds > longest.seconds {
				longest = next
			}
		}
	}()
	var report string
	return func() string {
		if cancel != nil {
			cancel()
			cancel = nil
			report = (<-seen).describe(conn)
		}
		return report
	}
}

type slowSeen struct {
	query   string
	seconds float64
}

func (s slowSeen) describe(conn *pgx.Conn) string {
	if s.query == "" {
		return "no statement was seen running long"
	}
	// Sent raw: the text holds $n placeholders that GENERIC_PLAN leaves unbound,
	// and pgx would refuse them with no arguments.
	results, err := conn.PgConn().Exec(context.Background(), "EXPLAIN (GENERIC_PLAN) "+s.query).ReadAll()
	if err != nil {
		return fmt.Sprintf("ran %.1fs: %s\n(no plan: %v)", s.seconds, s.query, err)
	}
	var plan []string
	for _, result := range results {
		for _, row := range result.Rows {
			plan = append(plan, string(row[0]))
		}
	}
	return fmt.Sprintf("ran %.1fs: %s\nplan:\n%s", s.seconds, s.query, strings.Join(plan, "\n"))
}
