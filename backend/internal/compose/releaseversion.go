// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The guard that keeps a torn release set from running.
//
// Each role image is pulled by tag in its own request, so a publish between two
// pulls can hand back roles from different releases. The registry cannot refuse
// that, so the refusal happens at run time.
//
// The api is the authority: it ships with the migrator, so the schema in use is
// the one its release brought. It records its own release, and every other role
// refuses to start when it disagrees. This asymmetry lets an upgrade proceed:
// the api moves first. The check compares for equality, never order, so a
// rollback is an ordinary move.
//
// A torn set crash-loops with both versions in its log until an operator
// deploys one release. Two bounds hold: the check runs once at boot, and the
// record is last writer wins during a rollout. refuseMixedRelease names the
// backwards move as one cause of a refusal, so a clean deploy is not audited.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

// installationReleaseObserved is the system_log action carrying the release the
// api recorded for this installation; one row per CHANGE, so the ledger reads as
// the installation's upgrade history rather than one row per restart.
const installationReleaseObserved = "release.version_observed"

// releaseLedgerFact names this fact in the advisory-lock key, so the release
// observation serializes against other release observations and against nothing
// else — the extension inventory takes its own key and the two never wait on each
// other.
const releaseLedgerFact = "release-version"

// lastObservedReleaseQuery reads the release the api recorded most recently.
//
// It carries no scope, because ADR-0091 §8 phase D took the tenant column off
// system_log and there is nothing left to scope by: one installation serves one
// company (ADR-0061), and this ledger records that installation's releases.
//
// The residue this DOES admit, named because it is not hypothetical: an
// installation that merged an ARCHIVED predecessor still holds that
// predecessor's release rows — 0272 exempts the ledgers from the residue gate
// by name, precisely because their immutability trigger makes clearing them
// impossible — and this read can no longer tell them from its own. What keeps
// the guard honest is that RecordInstallationRelease writes the same unscoped
// ledger, so the api's own row is the newest as soon as it boots. The window is
// a worker booting against residue before that api boot, which is #2196.
//
// THE ORDERING KEY OBSERVES THE LOCK, which occurred_at does not.
//
// system_log.occurred_at defaults to now(), the TRANSACTION timestamp, and the
// advisory lock above is taken inside the transaction. So a replica that begins
// first, is descheduled, and commits second carries the OLDER stamp — and this
// read would return the other replica's row although this one committed last.
// The lock serialises the write correctly; the key simply did not observe that.
//
// `observed_at` is clock_timestamp() read after the lock is held, so it orders
// by the moment the writer actually got its turn. Rows written before it exists
// fall back to occurred_at, which is what they were ordered by anyway: the
// coalesce makes this a strict improvement rather than a cliff at the deploy
// that introduces it.
//
// Cast rather than compared as text. A timestamp rendered one way sorts
// lexicographically and rendered another does not, and nothing here would say
// which it got.
//
// id stays the deterministic tiebreak, for the reason extensioninventory spells
// out: uuidv7 ids are monotonic only within one process, and concurrently
// booting replicas mint theirs independently. The outer COALESCE is a different
// one — an absent release key must read as the empty string, the same value "no
// record at all" produces, since both mean there is nothing to compare.
const lastObservedReleaseQuery = `
	SELECT COALESCE(detail->>'release_version', '')
	  FROM system_log
	 WHERE action = $1 AND detail->>'installation' = $2
	 ORDER BY COALESCE((detail->>'observed_at')::timestamptz, occurred_at) DESC, id DESC
	 LIMIT 1`

// RecordInstallationRelease records the release this api was built from as the
// installation's release, when it differs from the last one recorded.
//
// An unstamped binary records NOTHING and leaves the previous record standing.
// That is the same "unknown disables the comparison" rule buildinfo carries
// everywhere, and here it also protects the record: a locally built api run
// against a real installation must not erase the release a real one wrote.
//
// Pre-bootstrap there is no installation to record against. The observation is
// skipped and the first boot after bootstrap records it — which is early enough,
// because the worker cannot get past its own dependency probe on a database the
// api has not migrated yet.
func RecordInstallationRelease(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, version string) error {
	if !buildinfo.Comparable(version) {
		// Said out loud, symmetric with the assert path below. An unstamped api
		// disarms the guard for the WHOLE installation — every role that boots
		// after it has nothing to compare against — and an operator reading this
		// log must be able to tell that apart from a guard that is working.
		log.Info("release guard inactive: this api carries no release version")
		return nil
	}
	ctx, bootstrapped, err := bootLedgerScope(ctx, pool, "system:release-version")
	if err != nil {
		return fmt.Errorf("compose: resolving the installation to record its release: %w", err)
	}
	if !bootstrapped {
		log.Info("installation release not recorded: installation not bootstrapped yet", "release_version", version)
		return nil
	}

	var previous string
	recorded := false
	// The closure declares its own error at every step. Assigning the enclosing
	// err from in here would leave two variables of that name meaning different
	// things, one of which this call is about to overwrite.
	if err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, bootLedgerLock, releaseLedgerFact); err != nil {
			return fmt.Errorf("compose: serializing the release observation: %w", err)
		}
		last, err := lastObservedRelease(ctx, tx)
		if err != nil {
			return err
		}
		previous = last
		if last == version {
			return nil
		}
		// The instant this writer got its turn, read from the database AFTER the
		// lock is held. It is what the read above orders by, and taking it here
		// rather than at transaction start is the whole of the fix: a replica
		// descheduled between BEGIN and the lock would otherwise stamp a moment
		// earlier than a replica that started later and finished first.
		var observedAt time.Time
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&observedAt); err != nil {
			return fmt.Errorf("compose: reading the instant this release observation got its turn: %w", err)
		}
		if _, err := storekit.LogSystem(ctx, tx, installationReleaseObserved, map[string]any{
			"release_version": version,
			"installation":    installationMarker(ctx),
			"observed_at":     observedAt.UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
		recorded = true
		return nil
	}); err != nil {
		return fmt.Errorf("compose: recording the installation's release: %w", err)
	}
	// Logged only after the transaction COMMITTED, so the line never reports a
	// record the database rolled back.
	if recorded {
		// "from" is deliberately present even when empty: an operator reading
		// the first boot after this guard shipped needs to see that there was no
		// previous record, not wonder which value was omitted.
		log.Info("installation release recorded", "from", previous, "to", version)
	}
	return nil
}

// AssertInstallationRelease refuses to boot a role whose release is not the one
// the api recorded for this installation.
//
// Every role EXCEPT the api calls it. The api is the authority (see the file
// comment) and has nothing to check itself against; a role that checked its own
// record would only ever confirm it.
//
// Three outcomes are all "start": this binary is unstamped, the installation is
// not bootstrapped, or no api has recorded a release yet. None of them is a
// match, but none of them is a MISMATCH either, and refusing on the absence of a
// fact would take down installations whose only defect is that their api has not
// restarted since this guard shipped.
func AssertInstallationRelease(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, version string) error {
	if !buildinfo.Comparable(version) {
		log.Info("release guard inactive: this binary carries no release version")
		return nil
	}
	ctx, bootstrapped, err := bootLedgerScope(ctx, pool, "system:release-version")
	if err != nil {
		return fmt.Errorf("compose: resolving the installation to check its release: %w", err)
	}
	if !bootstrapped {
		log.Info("release guard inactive: installation not bootstrapped yet", "release_version", version)
		return nil
	}

	var installation string
	if err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		recorded, err := lastObservedRelease(ctx, tx)
		installation = recorded
		return err
	}); err != nil {
		return fmt.Errorf("compose: reading the installation's release: %w", err)
	}
	if err := refuseMixedRelease(version, installation); err != nil {
		return err
	}
	if installation == "" {
		log.Info("release guard inactive: no api has recorded this installation's release yet", "release_version", version)
		return nil
	}
	log.Info("release matches the installation", "release_version", version)
	return nil
}

// refuseMixedRelease answers the refusal a role owes when the installation runs
// a different release than the role was built from, or nil when there is nothing
// to refuse.
//
// THE MESSAGE IS THE DELIVERABLE, and it has to end the investigation rather than
// start one. What an operator sees is a crash-looping role beside an api serving
// happily, and there are TWO ways to arrive there — the message used to state
// only the first as though it were the only one:
//
//   - The deployment genuinely supplied two releases. Redeploy at one.
//   - The deployment supplied ONE release, and the installation's record moved
//     backwards underneath it. The record is last writer wins (see the file
//     comment), so an api replica from the previous release restarting after the
//     new one recorded puts the older release back — and every correctly deployed
//     role then refuses exactly like this. An operator who knows their deploy was
//     clean and reads a message blaming the deploy has been sent to look at the
//     one thing that is right.
//
// The second is why the refusal names the api's restart as a shape rather than
// leaving it to be rediscovered: the bound is deliberate and tracked, and what it
// costs is diagnosis, so the diagnosis is what this pays back.
//
// It names no internals: an operator reading a role's log needs the two versions
// and what to do, and needs nothing about the ledger the answer came from.
//
// IT NAMES NO DEPLOYMENT MECHANISM EITHER. Container images and a registry are
// how the release reaches most installations, but they are not the only way — this
// software also runs from a plain host — and an error that tells an operator to
// re-pull an image is wrong for anyone who did not pull one. What is true of every
// installation is the fact and the correction: the roles disagree, and every role
// has to be at one release. How the release arrives is the deployment's business,
// and docs/how-to/deploy-margince.md is where that belongs.
func refuseMixedRelease(mine, installation string) error {
	if !buildinfo.SkewBetween(mine, installation) {
		return nil
	}
	return fmt.Errorf(
		"this role is release %q but this installation runs release %q: "+
			"every role has to be at one release, and this one will not run half of "+
			"one release beside half of another. "+
			"Deploy every role (api, web, worker) at one release and restart. "+
			"If the set WAS deployed at one release, the other shape that produces "+
			"this is a rollout: the installation's release is whichever one its api "+
			"recorded LAST, so an api still on the previous release restarting after "+
			"the new one recorded puts the older release back on the record, and every "+
			"correctly deployed role then refuses exactly like this. Restarting the "+
			"api at the intended release restores the record",
		mine, installation,
	)
}

// lastObservedRelease reads the release THIS installation's api recorded most
// recently; no such record reads as the empty string, which every caller treats
// as "nothing to compare" rather than as a value.
//
// Scoped to the installation marker rather than to the newest row of all: an
// installation that merged an archived predecessor still holds that
// predecessor's release rows, and the ledgers are exempt from the residue gate
// because their immutability trigger makes clearing them impossible.
func lastObservedRelease(ctx context.Context, tx pgx.Tx) (string, error) {
	var version string
	err := tx.QueryRow(ctx, lastObservedReleaseQuery,
		installationReleaseObserved, installationMarker(ctx)).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		// Returned unwrapped: both callers already name what they were doing, and
		// wrapping here made the boot failure an operator reads say it twice.
		return "", err
	}
	return version, nil
}
