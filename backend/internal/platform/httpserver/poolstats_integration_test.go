// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// TestEveryDeclaredPoolSeriesReachesTheScrape closes the other half. The census
// above proves the tables name every statistic; this proves the tables are what
// the handler renders, so a series can neither be declared and dropped on the
// way out nor emitted under a name nothing declared.
//
// In the integration lane because it holds a pool, and a pool is what
// check-test-lanes.sh keeps out of the unit one. It needs the database for
// nothing else: every figure it asserts on is a zero or a count the pool keeps
// for itself, and no statement is issued.
func TestEveryDeclaredPoolSeriesReachesTheScrape(t *testing.T) {
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set — run `make db-up` and try again (integration tests fail loudly, they never skip)")
	}
	pool, err := testdb.OwnPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("opening a pool to read the exposition off: %v", err)
	}
	defer pool.Close()

	rec := httptest.NewRecorder()
	Metrics(MetricsInput{Pool: pool})(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	for _, level := range poolLevels {
		if want := `margince_pgxpool_conns{state="` + level.state + `"}`; !strings.Contains(body, want) {
			t.Errorf("the scrape carries no %s, so %s is declared and never published:\n%s", want, level.stat, body)
		}
	}
	for _, counter := range poolCounters {
		if !strings.Contains(body, "\n"+counter.name+" ") {
			t.Errorf("the scrape carries no %s, so %s is declared and never published:\n%s", counter.name, counter.stat, body)
		}
		if !strings.Contains(body, "# TYPE "+counter.name+" counter") {
			t.Errorf("%s is published without the TYPE line that tells Prometheus it is a counter", counter.name)
		}
	}
}
