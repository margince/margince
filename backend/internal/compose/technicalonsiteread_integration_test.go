// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Reading a company's site asks what that company publicly runs.
//
// The claim is worth a real river_job table because the enqueue happens through
// the client WORKING the read's own job, and a unit test cannot tell a context
// that carries such a client from one that does not — both compile, and the one
// without it takes the silent path this lane deliberately chose. So the test
// that matters drives the real worker over a real client and reads the row back
// out of the queue.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
)

// workClient is the insert-only client a working job carries, built here
// because rivertest.WorkContext needs the *river.Client itself and
// platform/jobs holds its own unexported.
func workClient(t *testing.T, e *integration.Env) *river.Client[pgx.Tx] {
	t.Helper()
	client, err := river.NewClient(riverpgxv5.New(e.Pool), &river.Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("river.NewClient: %v", err)
	}
	return client
}

func TestASiteReadAsksWhatTheCompanyPubliclyRuns(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	org := insertOrg(t, e, e.Rep1, "acme.example", "")
	worker, _ := newDeepReadTestWorker(e, acmeDeepSite(), acmeDeepBrain())
	_, args := startDeepRead(t, e, org)

	ctx := rivertest.WorkContext(context.Background(), workClient(t, e))
	if err := worker.run(ctx, args); err != nil {
		t.Fatalf("run: %v", err)
	}

	// The lookup the reader never had to ask for. Its args name the company the
	// read was about — a lookup pointed at another record would enrich the
	// wrong account while looking exactly like this one.
	job := rivertest.RequireInserted(ctx, t, riverpgxv5.New(e.Pool),
		TechnicalEnrichOrganizationArgs{}, nil)
	if job.Args.OrganizationID != org || job.Args.Workspace != e.WS {
		t.Fatalf("queued lookup = %+v, want the company this read was about (%s in %s)",
			job.Args, org, e.WS)
	}
	if job.Queue != technicalLookupQueue {
		t.Fatalf("queued on %q, want %q — the lookup's pacing is the queue's, not the crawl's",
			job.Queue, technicalLookupQueue)
	}
}

// A read that resolved no company has nothing to look up. The triage lane runs
// before an account exists, so an enqueue here would name a nil record — and
// the args are not nullable, so the failure would be a panic inside a lane
// whose whole contract is that it cannot fail the read.
func TestASiteReadWithNoCompanyQueuesNoLookup(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	worker, _ := newDeepReadTestWorker(e, acmeDeepSite(), acmeDeepBrain())
	ctx := rivertest.WorkContext(context.Background(), workClient(t, e))

	// The triage lane's shape: a claim whose read is about a DOMAIN, with no
	// account resolved behind it yet.
	worker.askWhatTheCompanyRuns(ctx, people.SiteReadClaim{
		OrganizationID: nil,
		TargetKind:     "domain",
		SeedURL:        "https://acme.example",
	})

	rivertest.RequireNotInserted(ctx, t, riverpgxv5.New(e.Pool),
		TechnicalEnrichOrganizationArgs{}, nil)
}
