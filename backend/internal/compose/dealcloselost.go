// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Margince closing, as their owners' agent, the open deals the customer stopped
// answering. The evidence is the nightly input check's own (assuranceSubjects).
// After an Undo, a note, call or meeting a colleague logs restarts the silence
// count. Without one, the next check closes the deal again.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// marginceActorID names Margince in the audit trail of a change it makes for
// a deal's owner. The row also carries on_behalf_of: the owner.
const marginceActorID = "agent:margince"

// silentDealCloser closes the deals whose customer stopped answering.
type silentDealCloser struct {
	pool       *pgxpool.Pool
	deals      *deals.Store
	users      *identity.Service
	activities *activities.Store
	now        func() time.Time
	log        *slog.Logger
}

func newSilentDealCloser(pool *pgxpool.Pool, log *slog.Logger) silentDealCloser {
	return silentDealCloser{
		pool:       pool,
		deals:      deals.NewStore(InstallationDB(pool), DealsInstallation()),
		users:      identity.NewService(pool),
		activities: activities.NewStore(InstallationDB(pool)),
		now:        func() time.Time { return time.Now().UTC() },
		log:        log,
	}
}

// closeDecision is one deal the check would close, and why.
type closeDecision struct {
	deal   ids.DealID
	owner  ids.UUID
	code   string
	reason string
}

// CloseWorkspace closes every due deal in the workspace bound in ctx and
// reports how many it closed. One deal that cannot close does not stop the
// others; it is logged and tried again at the next check.
func (c silentDealCloser) CloseWorkspace(ctx context.Context) (int, error) {
	now := c.now()
	var due []closeDecision
	err := database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		var err error
		due, err = closeDecisions(ctx, tx, now, nil)
		return err
	})
	if err != nil {
		return 0, err
	}
	closed := 0
	for _, decision := range due {
		ok, err := c.closeOne(ctx, decision, now)
		if err != nil {
			c.log.WarnContext(ctx, "Margince could not close a silent deal",
				"deal_id", decision.deal, "error", err)
			continue
		}
		if ok {
			closed++
		}
	}
	return closed, nil
}

// closeOne closes one deal as its owner's agent. Under the deal row lock the
// owner's switch must be on and the deal still due.
func (c silentDealCloser) closeOne(ctx context.Context, decision closeDecision, now time.Time) (bool, error) {
	ownerCtx, err := ownersAgent(ctx, c.users, decision.owner, marginceActorID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return c.closeAs(ownerCtx, decision, now)
}

// closeAs closes as an owner's agent bound before the write. The owner may
// leave or lose the deal grant in between, so the close binds them again.
func (c silentDealCloser) closeAs(ownerCtx context.Context, decision closeDecision, now time.Time) (bool, error) {
	return c.deals.CloseAsLost(ownerCtx, deals.CloseLostInput{
		DealID: decision.deal, Owner: decision.owner, ReasonCode: decision.code, Reason: decision.reason,
		StillDue: func(ctx context.Context, tx pgx.Tx) (bool, error) {
			mode, err := approvals.AutoApplyModeTx(ctx, tx, decision.owner, deals.DealCloseLostKind)
			if err != nil || mode != approvals.ModeAuto {
				return false, err
			}
			again, err := closeDecisions(ctx, tx, now, []ids.UUID{decision.deal.UUID})
			return len(again) == 1 && again[0] == decision, err
		},
		Record: func(ctx context.Context, tx pgx.Tx, _ ids.UUID) error {
			return c.recordOnTimeline(ctx, tx, decision)
		},
		AsOwner: ownersAgentTx(c.users, decision.owner, marginceActorID),
	})
}

// recordOnTimeline writes the close on the deal's timeline as a note. Its
// origin keeps it out of every recency clock, so it never reads as the
// deal being worked on.
func (c silentDealCloser) recordOnTimeline(ctx context.Context, tx pgx.Tx, decision closeDecision) error {
	subject := "Margince closed this deal as lost"
	body := decision.reason + ". Undo it from Home if the deal is still alive, and add a note on why."
	_, _, err := c.activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindNote), Subject: &subject, Body: &body, Origin: activities.OriginSystemRemediation,
		Links: []activities.ActivityLinkInput{{EntityType: entityTypeDeal, EntityID: decision.deal.UUID}},
	})
	if err != nil {
		return fmt.Errorf("compose: writing the close on the deal's timeline: %w", err)
	}
	return nil
}

// closeDecisions reads the open deals the check would close now, narrowed to
// only when it is not nil. The run and the re-check under the row lock both
// read it.
func closeDecisions(ctx context.Context, tx pgx.Tx, now time.Time, only []ids.UUID) ([]closeDecision, error) {
	threshold, err := config.StoredApplyTx(ctx, tx, config.Bound(), deals.SilentCloseDays)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the silent close threshold: %w", err)
	}
	zone, err := installationZoneTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	coverage := AssuranceCoverage(ctx, tx, now)
	subjects, err := assuranceSubjects(ctx, tx, now, coverage, only)
	if err != nil {
		return nil, err
	}
	facts, err := closeFactsFor(ctx, tx, now, subjects)
	if err != nil {
		return nil, err
	}
	mailRead, err := closeMailRead(ctx, tx, now, coverage)
	if err != nil {
		return nil, err
	}
	var out []closeDecision
	for _, subject := range subjects {
		dealFacts := facts[subject.DealID]
		owner, err := ids.Parse(subject.Owner)
		if err != nil || !mailRead(&owner, dealFacts.mailboxes) {
			continue
		}
		if decision, ok := decideClose(subject, dealFacts, threshold, now, zone); ok {
			out = append(out, decision)
		}
	}
	return out, nil
}

// closeMailRead answers whether every mailbox a deal's customer mail came
// through was read on time. The close counts mail of every audience, so a
// broken mailbox that carried a private thread blocks it too.
func closeMailRead(
	ctx context.Context, tx pgx.Tx, now time.Time, coverage []assurance.SourceCoverage,
) (func(owner *ids.UUID, mailboxes []string) bool, error) {
	byOwner, err := mailCoverageByOwner(ctx, tx, now)
	if err != nil {
		return nil, err
	}
	byConnection, err := mailCoverageByConnection(ctx, tx, now)
	if err != nil {
		return nil, err
	}
	return dealMailCovered(sourceChecked(coverage, sourceMail), byOwner, byConnection), nil
}

// decideClose is the rule; it needs the owner's mail read on time. With no
// wait or accepted meeting ahead, a deal closes when the customer refused, or
// after more than threshold days of silence.
func decideClose(
	subject assurance.Subject, facts closeFacts, threshold int, now time.Time, zone *time.Location,
) (closeDecision, bool) {
	owner, err := ids.Parse(subject.Owner)
	assessed := subject.Viability
	if err != nil || assessed == nil || slices.Contains(assessed.Reasons, viability.ReasonCoverageUnknown) {
		return closeDecision{}, false
	}
	deal, err := ids.Parse(subject.DealID)
	if err != nil {
		return closeDecision{}, false
	}
	decision := closeDecision{deal: ids.From[ids.DealKind](deal), owner: owner}
	keptOpen := facts.keptOpenAt()
	silentDays := min(assessed.SilentDays, daysSince(facts.anyProgressAt, now, zone))
	switch {
	case facts.deferredAt(now):
		return closeDecision{}, false
	case slices.Contains(assessed.Reasons, viability.ReasonCustomerDeclined) &&
		after(facts.refusalAt, keptOpen) && after(facts.refusalAt, facts.anyProgressAt):
		decision.code, decision.reason = deals.LossReasonOther, "The customer wrote that they will not go ahead"
		return decision, true
	case silentReason(assessed.Reasons) && silentDays > threshold && daysSince(keptOpen, now, zone) > threshold:
		decision.code = deals.LossReasonWentSilent
		decision.reason = silenceReason(silentDays, assessed.SilentDays, threshold)
		return decision, true
	}
	return closeDecision{}, false
}

// silenceReason names the silence the close was decided on. The reason reaches
// every reader of the deal. When private mail is newer than all visible mail,
// it names the threshold, not a day that private mail set.
func silenceReason(silentDays, visibleDays, threshold int) string {
	if silentDays < visibleDays {
		return fmt.Sprintf("No word from the customer for more than %d days", threshold)
	}
	return fmt.Sprintf("No word from the customer for %d days", silentDays)
}

func silentReason(reasons []viability.Reason) bool {
	return slices.Contains(reasons, viability.ReasonCustomerSilent) ||
		slices.Contains(reasons, viability.ReasonNeverContacted)
}

// after reports whether at is later than since; a missing since is always
// earlier, and a missing at is never later.
func after(at, since *time.Time) bool {
	return at != nil && (since == nil || at.After(*since))
}

// daysSince counts days from at to now; a missing at is longer ago than any
// threshold.
func daysSince(at *time.Time, now time.Time, zone *time.Location) int {
	if at == nil {
		return int(^uint(0) >> 1)
	}
	return elapsed.Days(*at, now, zone)
}
