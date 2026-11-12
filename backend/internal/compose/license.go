// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation's entitlement, resolved before a process role serves
// (UC-E11-05 E1). Composed here because both serving roles need the same
// answer from the same two inputs — the deployment file's token reference and
// the bundled validation module — and one spelling of "what happens when the
// license is refused" is the point: an api that refuses to boot beside a worker
// that shrugs would be a licensing posture nobody could describe.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/installtrial"
	"github.com/margince/margince/backend/internal/compose/licensereport"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// EnsureLicense resolves the installation's license posture and hands back the
// watcher that holds it. The caller starts the watcher's re-check loop and
// wires its posture into whatever reports it.
//
// Two postures refuse the boot: a license the bundled module will not honor, and
// a module that could not run at all. No license is not one of them. A
// production installation without one runs on its trial (installtrial.Trial),
// and a development or test one runs unlicensed with no trial at all.
func EnsureLicense(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool, vault keyvault.Vault, cfg *config.Config, env runtimeenv.Environment) (*licensecheck.Watcher, error) {
	// The SOURCE is handed over, not a token: the watcher re-reads it. A
	// license.token_file renewed in place takes effect on the next re-check;
	// license.token is fixed at boot.
	//
	// The source reads the deployment's declaration first and the key vault
	// where there is none, so an installation that has sealed its token and
	// dropped the variable keeps booting — and one whose vault it cannot open
	// is told THAT, rather than being told it has no license.
	source := SealedLicenseTokenSource(pool, vault, cfg, log)
	watcher, err := licensecheck.NewWatcher(ctx, source, time.Now, log, env)
	if err != nil {
		// The setting to correct is named HERE, where the token's source is
		// known: platform's check is handed a token, not a configuration file.
		return nil, fmt.Errorf("%w — %s", err, correctTheToken(cfg))
	}
	licensereport.LogPosture(ctx, log, watcher.Posture(), licenseTokenOrigin(cfg, watcher.Posture()), env)
	return watcher, nil
}

// correctTheToken is the second half of a refused-license error: what to do.
//
// It is not one sentence for both sources. "Remove the key vault and start
// again" — which naming the origin uniformly would produce — is advice that
// destroys the only copy of the license along with every connector credential
// the installation holds, to fix a token the operator can simply re-declare.
func correctTheToken(cfg *config.Config) string {
	if origin := licenseTokenDeclaredAt(cfg); origin != licenseTokenOriginNone {
		return "correct or remove " + origin + " and start again"
	}
	return "this token came from the key vault, where it was sealed from a declaration since deleted: " +
		"re-declare license.token and the next boot re-seals it"
}

// licenseTokenOrigin names where the token came from, for the boot log and for
// the refusal that tells an operator what to correct.
//
// The declaration speaks only for the boot sources and has never heard of the
// vault. An installation that sealed its token and dropped the declaration
// would otherwise be told its token came from "none" while running on one.
// Naming the vault tells an operator where to look, instead of sending them to
// a file that no longer mentions it. A grep token rather than prose, because every other value of
// this field is one.
func licenseTokenOrigin(cfg *config.Config, posture licensecheck.Posture) string {
	if origin := licenseTokenDeclaredAt(cfg); origin != licenseTokenOriginNone {
		return origin
	}
	if posture.State == licensecheck.StateAbsent {
		// Nothing declared and nothing sealed. Naming the vault here would
		// describe an installation that had a token, which this one does not.
		return licenseTokenOriginNone
	}
	return "keyvault"
}

// WithLicensePosture wires the resolved posture into everything this role does
// with it: the /metrics section, the entitlement surface, and the ceiling seat
// creation is held to. The function is read at scrape and call time rather than
// the value being copied in, so each of them reports and enforces what the
// watcher last resolved instead of what the process booted with.
func WithLicensePosture(posture func() licensecheck.Posture) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.licensePosture = posture
		// Wired here, not in the assembly: the assembly runs before the options,
		// so a handler built there would hold a nil posture and answer 501. The
		// seat count stays the assembly's; withPosture only adds the posture.
		s.licenseHandlers = s.withPosture(posture)
		// The same posture is the seat writer's ceiling, read at call time, so an
		// admin is held to the number they are shown. A refused or removed license
		// keeps the last valid one's ceiling (Posture.SeatCeiling).
		s.authHandlers = s.WithSeatCeiling(func() (int, bool) { return posture().SeatCeiling() })
	}
}

// WithInstallationTrial wires the trial into every surface it locks: the
// admission doors and /me, the agent tool surface, and the entitlement read.
// posture is read at call time, so a key entered later ends the trial.
func WithInstallationTrial(env runtimeenv.Environment, posture func() licensecheck.Posture, now func() time.Time) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		trial := installtrial.New(pool, env, posture, now)
		s.authHandlers = s.WithTrialLock(trial.Ended)
		s.trial = trial
	}
}

// WithLicenseRecheck wires the watcher's re-check into "Check now". It adds to
// the handler the assembly built, so the seat count and posture stay.
func WithLicenseRecheck(recheck func(context.Context) error) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.recheck = recheck
	}
}

// writeLicenseMetrics renders the entitlement section. A role that wired no
// posture writes nothing, the same "declared or absent" posture every other
// section takes — a state gauge nobody resolved would read as an installation
// that had been checked.
func (s Server) writeLicenseMetrics(w io.Writer) {
	if s.licensePosture == nil {
		return
	}
	licensereport.WriteMetrics(w, s.licensePosture())
}
