// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The daily reading of how far apart the two outbound authorities are.
//
// The engine has run in observe mode since it shipped: it decides, records, and
// the old purpose gate rules. Ending that is a decision somebody has to make on
// evidence, and this pass is what puts the evidence in front of them without
// anybody remembering to ask.
//
// It exists because the same reading as a typed subcommand was one nobody would
// ever run. The number that decides a rollout is not one an operator thinks to
// go and fetch — a stored measurement nobody sweeps is not a control, which is
// the rule this repository already applies to a retention deadline and applies
// here for the same reason.
//
// It DECIDES nothing and writes no domain row. Enforcement is a setting a human
// changes after reading this; a pass that flipped a category itself would be a
// second authority over what may be sent, and the whole point of the observe
// period is that there is exactly one until somebody decides otherwise.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AuthzDisagreementArgs carries nothing: the pass reads a fixed window back from
// its own run time, so there is no argument a caller could supply that would not
// be a second answer to what the cadence already says.
type AuthzDisagreementArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (AuthzDisagreementArgs) Kind() string { return "comms_authz_disagreement" }

// disagreementWindow is how far back one pass reads.
//
// Matched to the cadence rather than chosen independently: the pass runs daily
// and reads a day, so consecutive lines describe consecutive periods and can be
// compared. A window shorter than the cadence would leave gaps no reading ever
// covers, and a longer one would report the same disagreement on several days
// running, which reads as a growing problem when it is one problem counted
// repeatedly.
//
// Slightly wider than the cadence, because a pass that starts late must not
// leave the minutes before it unread.
const disagreementWindow = 25 * time.Hour

// authzDisagreementWorker reads the two authorities against each other.
type authzDisagreementWorker struct {
	identity *identity.Service
	store    *consent.Store
	now      func() time.Time
	log      *slog.Logger
}

func newAuthzDisagreementWorker(
	idsvc *identity.Service, store *consent.Store, now func() time.Time, log *slog.Logger,
) *authzDisagreementWorker {
	return &authzDisagreementWorker{identity: idsvc, store: store, now: now, log: log}
}

func (w *authzDisagreementWorker) Work(ctx context.Context, _ *river.Job[AuthzDisagreementArgs]) error {
	// The installation's own workspace, on the context rather than left to the
	// store's handle to resolve, so this pass and the rows it reads agree on
	// which workspace it is acting in.
	ctx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("comms_authz_disagreement: resolving the installation: %w", err))
	}
	// The system principal, because this is the installation asking about its
	// own rollout rather than a seat asking about a person. Nothing the reading
	// returns names a subject: no address, no consent state, only how two rules
	// have compared.
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:authz_disagreement",
	})

	since := w.now().Add(-disagreementWindow)
	report, err := w.store.DisagreementReportSince(ctx, since)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("comms_authz_disagreement: reading the disagreement: %w", err))
	}
	w.report(ctx, since, report)
	return nil
}

// report writes the pass's finding as structured lines.
//
// AGREEMENT IS LOGGED TOO, at info and once. A pass that spoke only when it
// found something would leave an operator unable to tell "the authorities agree"
// from "the pass has not run since March", and those call for opposite actions.
func (w *authzDisagreementWorker) report(
	ctx context.Context, since time.Time, report []consent.Disagreement,
) {
	if len(report) == 0 {
		w.log.InfoContext(ctx, "the outbound engine and the old purpose gate agreed on every delivery",
			"since", since)
		return
	}
	var stopping, starting int
	for _, d := range report {
		if d.EngineIsStricter {
			stopping += d.Deliveries
		} else {
			starting += d.Deliveries
		}
		// One line per SHAPE, because the shape is the finding. A total alone
		// reads as alarming and may be entirely correct — four thousand
		// deliveries under the legacy transactional purpose are exactly what
		// the engine exists to stop — so the category and the reason travel
		// with the count.
		w.log.InfoContext(ctx, "the outbound authorities differed",
			"category", d.Category, "reason", d.ReasonCode,
			"engine", d.EngineVerdict, "old_gate", d.LegacyVerdict,
			"engine_is_stricter", d.EngineIsStricter,
			"deliveries", d.Deliveries, "recipients", d.Decisions)
	}
	// WARN on the summary, because a non-zero `stopping` is what an operator has
	// to weigh before enforcing: it is the mail that would have stopped. The
	// other direction is real and worth seeing, and is not what makes a rollout
	// dangerous.
	w.log.WarnContext(ctx, "enforcing the outbound engine would have changed what was sent",
		"since", since, "would_stop", stopping, "would_start", starting)
}

// addAuthzDisagreementWorker wires the pass. Its SCHEDULE is placed with the
// others in wireJobs, because this pass takes no conditional wiring — it reads a
// table every installation has — and a helper that only ever returns
// periodicFor's answer would hide the cadence from the list where every other
// unconditional one is read.
func addAuthzDisagreementWorker(reg *jobRegistry, pool *pgxpool.Pool, log *slog.Logger) {
	addDeclaredWorker[AuthzDisagreementArgs](reg, newAuthzDisagreementWorker(
		identity.NewService(pool), consent.NewStore(InstallationDB(pool)), time.Now, log))
}
