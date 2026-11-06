// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Margince reading each open deal's evidence at night and setting its close
// date, amount or win probability for the owner, each change with an Undo.
// A deal is read again only when its evidence, its fields, its owner's
// switches or the prompt changed, or its close date passed.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealVerdictBatch bounds how many deals one night reads per workspace; the
// rest wait for the next night, oldest verdict first.
const dealVerdictBatch = 50

// dealVerdictRulesVersion moves when the rules in dealverdict change what
// they let through, so every deal is read again under the new rules.
const dealVerdictRulesVersion = "3"

// verdictCorrections are the corrections a verdict may make, in a fixed order.
var verdictCorrections = []string{
	deals.VerdictCloseDateCorrection, deals.VerdictAmountCorrection, deals.VerdictProbabilityCorrection,
	deals.VerdictStageMoveCorrection,
}

// dealVerdictPass reads due deals and applies what the rules let through.
type dealVerdictPass struct {
	pool      *pgxpool.Pool
	brain     completer
	deals     *deals.Store
	users     *identity.Service
	notes     *activities.Store
	assurance *assurance.Store
	now       func() time.Time
	log       *slog.Logger
}

func newDealVerdictPass(pool *pgxpool.Pool, brain completer, log *slog.Logger) dealVerdictPass {
	return dealVerdictPass{
		pool: pool, brain: brain, log: log,
		deals: deals.NewStore(InstallationDB(pool), DealsInstallation()).
			WithOwnDomains(ownDomainReader{store: capture.NewOwnDomainStore(InstallationDB(pool))}),
		users:     identity.NewService(pool),
		notes:     activities.NewStore(InstallationDB(pool)),
		assurance: assurance.NewStore(InstallationDB(pool)),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// verdictCandidate is one open deal and what its last verdict stored.
type verdictCandidate struct {
	deal     ids.UUID
	owner    ids.UUID
	switched map[string]bool
	digest   string
	prior    json.RawMessage
}

// RunWorkspace reads the due deals of the workspace bound in ctx. A spent
// model budget ends the pass cleanly: the deals still differ the next night.
func (p dealVerdictPass) RunWorkspace(ctx context.Context) error {
	if p.brain == nil {
		return nil
	}
	now := p.now()
	var due []verdictCandidate
	night := verdictNight{lang: identity.BaseLanguageForPrompt(ctx, p.pool)}
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		if night.today, err = installationToday(ctx, tx, now); err != nil {
			return err
		}
		if night.mailRead, err = closeMailRead(ctx, tx, now, AssuranceCoverage(ctx, tx, now)); err != nil {
			return err
		}
		due, err = dueVerdictCandidates(ctx, tx, now)
		return err
	})
	if err != nil {
		return err
	}
	for _, candidate := range due {
		err := p.judge(ctx, candidate, night)
		if errors.Is(err, ai.ErrBudgetDeferred) || isDeferred(err) {
			p.log.InfoContext(ctx, "the deal verdicts stopped for tonight: the model budget is spent", "error", err)
			return nil
		}
		if err != nil {
			p.log.WarnContext(ctx, "Margince could not read a deal's evidence", "deal_id", candidate.deal, "error", err)
		}
	}
	return nil
}

// verdictNight is what every deal of one night's pass is judged against.
type verdictNight struct {
	today    time.Time
	lang     string
	mailRead mailReadFor
}

func isDeferred(err error) bool {
	_, deferred := ai.DeferredUntil(err)
	return deferred
}

// installationToday is the installation's calendar day, as a UTC midnight.
func installationToday(ctx context.Context, tx pgx.Tx, now time.Time) (time.Time, error) {
	zone, err := installationZoneTx(ctx, tx)
	if err != nil {
		return time.Time{}, err
	}
	y, m, d := now.In(zone).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), nil
}

// dueVerdictCandidates is the open deals with a live owner and no legal hold
// that are due. A deal is due when its input or basis changed since its last
// verdict, or its close date passed. A deal nobody can act for waits on the
// team list for an owner.
func dueVerdictCandidates(ctx context.Context, tx pgx.Tx, now time.Time) ([]verdictCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT d.id, d.owner_id, `+dealVerdictChecksumSQL()+`, r.verdict_input_checksum,
		       r.verdict_basis_digest,
		       r.verdict_next_due_at <= $1
		         OR (`+verdictActionsSQL("r.verdict_actions")+` @> $2::jsonb
		             AND `+deals.RateStoredSinceSQL("r.verdict_assessed_at")+`),
		       r.verdict_actions
		  FROM deal d LEFT JOIN assurance_deal_receipt r ON r.deal_id = d.id
		 WHERE d.status = 'open' AND d.archived_at IS NULL AND NOT d.legal_hold AND NOT `+deals.NoLiveOwnerSQL("d")+`
		 ORDER BY r.verdict_assessed_at NULLS FIRST, d.id`, now, `[{"outcome":"`+dealverdict.DroppedNoRate+`"}]`)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the deals a verdict may read: %w", err)
	}
	type row struct {
		candidate                verdictCandidate
		checksum                 string
		storedChecksum, storedBy *string
		clockDue                 *bool
	}
	scanned, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var out row
		err := r.Scan(&out.candidate.deal, &out.candidate.owner, &out.checksum, &out.storedChecksum,
			&out.storedBy, &out.clockDue, &out.candidate.prior)
		return out, err
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading the deals a verdict may read: %w", err)
	}
	// The base currency joins the basis: a price in another currency converts
	// through it. A rate stored after a no-rate drop makes the deal due above.
	base, err := identity.BaseCurrencyOf(ctx, tx)
	if err != nil {
		return nil, err
	}
	modes := map[ids.UUID]map[string]bool{}
	var due []verdictCandidate
	for _, r := range scanned {
		switched, ok := modes[r.candidate.owner]
		if !ok {
			if switched, err = ownerVerdictSwitches(ctx, tx, r.candidate.owner); err != nil {
				return nil, err
			}
			modes[r.candidate.owner] = switched
		}
		r.candidate.switched, r.candidate.digest = switched, verdictBasisDigest(switched, base)
		moved := r.storedChecksum == nil || *r.storedChecksum != r.checksum ||
			r.storedBy == nil || *r.storedBy != r.candidate.digest || (r.clockDue != nil && *r.clockDue)
		if moved {
			due = append(due, r.candidate)
		}
		if len(due) == dealVerdictBatch {
			break
		}
	}
	return due, nil
}

// ownerVerdictSwitches is which verdict corrections the owner lets Margince
// make without asking.
func ownerVerdictSwitches(ctx context.Context, tx pgx.Tx, owner ids.UUID) (map[string]bool, error) {
	out := make(map[string]bool, len(verdictCorrections))
	for _, correction := range verdictCorrections {
		mode, err := approvals.AutoApplyModeTx(ctx, tx, owner, deals.VerdictSwitch(correction))
		if err != nil {
			return nil, fmt.Errorf("compose: reading the owner's switches: %w", err)
		}
		out[correction] = mode == approvals.ModeAuto
	}
	// An admin may stop every stage move for the installation.
	moves, err := config.StoredApplyTx(ctx, tx, config.Bound(), deals.StageAutopilotEnabled)
	if err != nil {
		return nil, fmt.Errorf("compose: reading whether deals may move: %w", err)
	}
	out[deals.VerdictStageMoveCorrection] = out[deals.VerdictStageMoveCorrection] && moves
	return out, nil
}

// verdictBasisDigest names what can change a verdict without changing the
// deal: the prompt, the rules, the base currency and the owner's switches.
func verdictBasisDigest(switched map[string]bool, base string) string {
	parts := []string{dealverdict.PromptVersion, dealVerdictRulesVersion, base}
	for _, correction := range verdictCorrections {
		parts = append(parts, fmt.Sprintf("%s=%t", correction, switched[correction]))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}
