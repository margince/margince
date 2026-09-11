// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestReleaseGuardStopsATornSetAndLetsAnUpgradeThrough walks the whole life of
// the guard against a real database, through the two functions the process roles
// actually call: the api records, every other role asserts.
//
// One scenario rather than a test per branch, because the branches are not
// independent — what a role may do next depends on what the api has recorded so
// far, and the ordering is the part worth pinning. A torn set has to stop; an
// upgrade and a rollback have to get through; and an unstamped binary has to do
// neither, in either direction.
func TestReleaseGuardStopsATornSetAndLetsAnUpgradeThrough(t *testing.T) {
	env := Setup(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	record := func(version string) {
		t.Helper()
		if err := compose.RecordInstallationRelease(ctx, env.Pool, logger, version); err != nil {
			t.Fatalf("RecordInstallationRelease(%q): %v", version, err)
		}
	}
	mustStart := func(version string) {
		t.Helper()
		if err := compose.AssertInstallationRelease(ctx, env.Pool, logger, version); err != nil {
			t.Fatalf("a role at release %q refused to start: %v", version, err)
		}
	}
	mustRefuse := func(version string) {
		t.Helper()
		if err := compose.AssertInstallationRelease(ctx, env.Pool, logger, version); err == nil {
			t.Fatalf("a role at release %q started against a different installation release; a mixed set must not run", version)
		}
	}
	rows := func() int {
		t.Helper()
		return env.WsCount(t, `SELECT count(*) FROM system_log WHERE action = 'release.version_observed'`)
	}

	// Nothing recorded yet: no api on this installation has ever carried a
	// release, so there is nothing to disagree with and every role starts.
	mustStart("1970.41")
	mustStart("1970.42")

	// The api records its release. A restart at the same release records nothing
	// more — the ledger is the installation's upgrade history, not its boot count.
	record("1970.42")
	if got := rows(); got != 1 {
		t.Fatalf("recording a release wrote %d rows, want 1", got)
	}
	record("1970.42")
	if got := rows(); got != 1 {
		t.Fatalf("restarting at the same release wrote a new row (%d total), want still 1", got)
	}

	// The set as pulled: a worker from the same release runs, one from another
	// release does not.
	mustStart("1970.42")
	mustRefuse("1970.41")
	mustRefuse("1970.43")

	// An unstamped binary is not a release, in either direction. It never
	// refuses, and — the half that is easy to get wrong — it never erases the
	// release a real api recorded, which would silently disarm the guard for
	// every role that boots after it.
	mustStart("dev")
	mustStart("")
	record("dev")
	record("")
	if got := rows(); got != 1 {
		t.Fatalf("an unstamped api wrote %d rows, want still 1", got)
	}
	mustRefuse("1970.41")

	// An upgrade: the api moves first by definition, and the roles that follow
	// match the release it recorded. The role still on the old one now refuses,
	// which is what makes the rollout converge instead of deadlock.
	record("1970.43")
	if got := rows(); got != 2 {
		t.Fatalf("an upgrade wrote %d rows, want 2", got)
	}
	mustStart("1970.43")
	mustRefuse("1970.42")

	// A rollback is an ordinary move, not a special case: the api states the
	// release, so going backwards needs no permission. This is why the guard
	// compares for equality and never for order.
	record("1970.42")
	if got := rows(); got != 3 {
		t.Fatalf("a rollback wrote %d rows, want 3", got)
	}
	mustStart("1970.42")
	mustRefuse("1970.43")
}

// TestReleaseGuardIsInertOnAnUnbootstrappedInstallation: the arm the guard takes
// before an installation has an organization at all.
//
// It is the arm worth a test of its own precisely because nothing would report it
// wrong. A first boot has no workspace to record against, so there is nothing to
// write and nothing to compare — and if either function treated that as an error
// the api would refuse to bootstrap the installation it was about to create,
// while if the read treated an absent record as a VALUE the worker would refuse
// over a release nobody ever recorded. Both failures look like the guard working.
func TestReleaseGuardIsInertOnAnUnbootstrappedInstallation(t *testing.T) {
	env := Setup(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Empty the database the harness just seeded, workspace included: this is the
	// genuine pre-bootstrap state, reached through the reset the harness itself
	// uses rather than by deleting rows by hand.
	if err := testdb.Reset(ctx, OwnerConn(t)); err != nil {
		t.Fatalf("resetting to the pre-bootstrap state: %v", err)
	}

	if err := compose.RecordInstallationRelease(ctx, env.Pool, logger, "1970.42"); err != nil {
		t.Fatalf("recording a release before bootstrap is an error, and must not be: %v", err)
	}
	if err := compose.AssertInstallationRelease(ctx, env.Pool, logger, "1970.42"); err != nil {
		t.Fatalf("a role refused to start on an unbootstrapped installation: %v", err)
	}
	// And nothing was written against no workspace. Counted on the OWNER
	// connection with row security off, because the app pool's workspace-bound
	// read could not see such a row even if one existed — which is the whole
	// failure this assertion is here to be able to see.
	var rows int
	if err := OwnerConn(t).QueryRow(ctx,
		`SELECT count(*) FROM system_log WHERE action = 'release.version_observed'`).Scan(&rows); err != nil {
		t.Fatalf("counting release observations: %v", err)
	}
	if rows != 0 {
		t.Fatalf("a pre-bootstrap boot wrote %d release observations, want 0", rows)
	}
}

// TestReleaseGuardIgnoresAPredecessorsRelease is the property a workspace
// predicate used to give and the installation marker gives now.
//
// An installation that merged an archived predecessor still holds that
// predecessor's release rows: the ledgers are exempt from the archived-residue
// gate BY NAME, because their immutability trigger makes clearing them
// impossible. So the rows are there, they can be newer than ours by occurred_at,
// and before the marker the guard read one of them as this installation's — a
// worker would refuse to start against a release nobody here is running.
func TestReleaseGuardIgnoresAPredecessorsRelease(t *testing.T) {
	env := Setup(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := compose.RecordInstallationRelease(ctx, env.Pool, logger, "1970.42"); err != nil {
		t.Fatalf("recording this installation's release: %v", err)
	}

	// The predecessor's row, written directly because no live code can produce
	// one any more — an installation records only under its own marker. Stamped
	// an hour AHEAD so it wins every ordering the read could use, which is what
	// makes this about the marker rather than about occurred_at.
	if _, err := OwnerConn(t).Exec(ctx, `
		INSERT INTO system_log (actor_type, actor_id, action, detail, occurred_at)
		VALUES ('system', 'system:release-version', 'release.version_observed',
		        jsonb_build_object('release_version', '1970.99', 'installation', $1::text),
		        now() + interval '1 hour')`, ids.NewV7()); err != nil {
		t.Fatalf("seeding the predecessor's release row: %v", err)
	}

	// A role at THIS installation's release still starts.
	if err := compose.AssertInstallationRelease(ctx, env.Pool, logger, "1970.42"); err != nil {
		t.Fatalf("a role at this installation's release refused to start, so a predecessor's row was read as ours: %v", err)
	}
	// The guard is still armed rather than reading nothing at all.
	if err := compose.AssertInstallationRelease(ctx, env.Pool, logger, "1970.41"); err == nil {
		t.Fatal("a role at the wrong release started; the marked read must still find our own row")
	}
	// And the predecessor's release is not ours even though it is the newest
	// row in the ledger — which is what proves the marker rather than the order.
	if err := compose.AssertInstallationRelease(ctx, env.Pool, logger, "1970.99"); err == nil {
		t.Fatal("a role matched the PREDECESSOR's release; that row is not this installation's")
	}
}

// "LATEST" MEANS LAST COMMITTED, not first begun.
//
// system_log.occurred_at defaults to now(), which is the TRANSACTION timestamp,
// and the advisory lock this write takes is inside the transaction. So a replica
// that begins first, is descheduled, and commits second carries the OLDER stamp
// — and a read ordered by occurred_at returns the other replica's row although
// this one committed last. The lock serialises the writes; the key did not
// observe it.
//
// That is ordinary during a rollout, when replicas from two releases are alive
// at once, and what it costs is which release the whole fleet is held to: every
// role that disagrees with the winning row refuses to boot.
//
// The two rows are written directly rather than raced, because a race
// reproduces this about as often as the scheduler happens to interleave that
// way — the shape is what is under test, and a case that reproduces it
// sometimes is one that reports a regression as a flake. What IS production's
// here is the read: the assertion goes through AssertInstallationRelease, which
// is what every non-api role boots on.
func TestTheRecordedReleaseIsTheOneCommittedLast(t *testing.T) {
	env := Setup(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// The api records once, so the installation marker and the row shape are
	// production's rather than this test's guess at them.
	if err := compose.RecordInstallationRelease(ctx, env.Pool, logger, "1970.42"); err != nil {
		t.Fatalf("recording the first release: %v", err)
	}

	// The straggler: it BEGAN before the row above (older occurred_at) and got
	// its turn under the lock after it (later observed_at). Copied from the
	// recorded row so every other field — the marker especially — is the one
	// production wrote.
	if _, err := env.Pool.Exec(ctx, `
		INSERT INTO system_log (id, actor_type, actor_id, action, detail, occurred_at)
		SELECT uuidv7(), actor_type, actor_id, action,
		       jsonb_set(
		           jsonb_set(detail, '{release_version}', '"1970.43"'),
		           '{observed_at}', to_jsonb(to_char(occurred_at + interval '1 hour',
		                                             'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'))),
		       occurred_at - interval '1 hour'
		  FROM system_log
		 WHERE action = 'release.version_observed'`); err != nil {
		t.Fatalf("seeding the late-committing replica's row: %v", err)
	}

	// The row that committed last is the one the fleet is held to, even though
	// its transaction began first.
	if err := compose.AssertInstallationRelease(ctx, env.Pool, logger, "1970.43"); err != nil {
		t.Errorf("a role at the last-committed release refused to start: %v — the read is ordering by "+
			"when a writer BEGAN, so whichever replica was descheduled longest wins the record", err)
	}
	if err := compose.AssertInstallationRelease(ctx, env.Pool, logger, "1970.42"); err == nil {
		t.Error("a role at the superseded release started — with the ordering wrong in the other " +
			"direction this passes for free, so both halves are asserted")
	}
}
