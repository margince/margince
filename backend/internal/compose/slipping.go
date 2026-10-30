// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The pipeline-risk seams behind whats_slipping_this_week and
// draft_follow_ups_for (interfaces.md §2.2): the candidate set comes
// from the deals module's own row-scoped list path (RBAC + row scope
// apply exactly as on the HTTP surface — never raw SQL around the
// store), and each follow-up draft lands through the same composite
// provider write path every tool rides.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// slippingScanLimit bounds each list sweep. An honest bound: a workspace
// with more open deals than this sees the most recently created ones —
// the tool is a triage set, not an exhaustive report (run_report is).
const slippingScanLimit = 50

// slippingLister serves the candidate set at the window the tool asks for. It
// holds the open deals quiet that long and those past their expected close.
func slippingLister(pool *pgxpool.Pool) agents.SlippingLister {
	return func(ctx context.Context, quietDays int) ([]agents.SlippingDeal, error) {
		return quietDealLister(pool, quietDays)(ctx)
	}
}

// quietDealLister is the candidate set at one idle window.
//
// ONE candidate set, two patiences. The tool surface asks at the stalled
// threshold unless its caller names a window. The morning queue asks at the
// shorter window, because a queue that only speaks after two months is
// reporting rather than warning. Both run this function, so a change to how a
// candidate is built or evidenced reaches both. A second at-risk predicate
// beside this one is the failure this shape prevents.
//
// Deals whose expected close date has passed join the set at ANY window: an
// overdue close is late whatever the idle clock says.
func quietDealLister(pool *pgxpool.Pool, quietForDays int) func(context.Context) ([]agents.SlippingDeal, error) {
	scan := quietDealScan(pool, quietForDays)
	return func(ctx context.Context) ([]agents.SlippingDeal, error) {
		candidates, _, err := scan(ctx)
		return candidates, err
	}
}

// The scan-depth flag cannot be inferred from survivors of the bounded sweeps.
// Tool callers keep the unrestricted selection used by their existing contract.
func quietDealScan(
	pool *pgxpool.Pool, quietForDays int,
) func(context.Context) ([]agents.SlippingDeal, bool, error) {
	scan := quietDealScanWithClock(pool, quietForDays, clockNow)
	return func(ctx context.Context) ([]agents.SlippingDeal, bool, error) { return scan(ctx, nil) }
}

func quietDealScanWithClock(pool *pgxpool.Pool, quietForDays int, clock func() time.Time) func(context.Context, *storekit.OwnerSet) ([]agents.SlippingDeal, bool, error) {
	store := deals.NewStore(InstallationDB(pool), DealsInstallation())
	return func(ctx context.Context, owners *storekit.OwnerSet) ([]agents.SlippingDeal, bool, error) {
		zone, err := installationZone(ctx, pool)
		if err != nil {
			return nil, false, err
		}
		now := clock().UTC()
		limit := slippingScanLimit
		quiet, _, err := store.ListDeals(ctx, deals.ListDealsInput{
			QuietForDays: &quietForDays, Limit: &limit, ScopeOwners: owners,
		})
		if err != nil {
			return nil, false, err
		}
		openStatus := "open"
		today := storekit.WorkspaceDay(now, zone)
		sort := acceptFieldExpectedClose
		open, _, err := store.ListDeals(ctx, deals.ListDealsInput{Status: &openStatus, CloseBefore: &today, Sort: &sort, Limit: &limit, ScopeOwners: owners})
		if err != nil {
			return nil, false, err
		}
		// EITHER sweep at its bound means deals may sit past it. Asked of what
		// was READ, before anything is filtered, which is the whole reason this
		// question cannot be answered from the returned rows.
		cut := len(quiet) >= slippingScanLimit || len(open) >= slippingScanLimit

		// The quiet sweep already applied the window, so its rows are admitted
		// on that ground alone. candidate.Stalled is the row's flag at
		// StalledThresholdDays, FALSE for every deal a shorter window admits.
		// Testing it would fetch the right rows and drop them all.
		admitted := map[ids.UUID]bool{}
		for _, d := range quiet {
			admitted[ids.UUID(d.Id)] = true
		}

		// The INSTALLATION's zone, not the server's. A close date is a calendar
		// date a human picked, and asking whether it has passed in UTC gives a
		// different answer from the deal record's own hygiene flag for as long
		// as the two zones disagree about the day — hours wide, every day, for
		// any installation not running in UTC.

		out := make([]agents.SlippingDeal, 0, len(quiet))
		seen := map[ids.UUID]bool{}
		for _, d := range append(quiet, open...) {
			candidate := slippingCandidate(d, now, zone)
			if seen[candidate.DealID] {
				continue
			}
			if !admitted[candidate.DealID] && !candidate.CloseOverdue {
				continue
			}
			// Stalled means quiet at the window asked for, so the tool can
			// evidence that idle claim. The row's own flag is the answer at
			// deals.StalledThresholdDays, which also holds at any shorter window.
			candidate.Stalled = admitted[candidate.DealID] ||
				(candidate.Stalled && quietForDays <= deals.StalledThresholdDays)
			seen[candidate.DealID] = true
			out = append(out, candidate)
		}
		// Asked once, of the survivors. The sweep's third named signal is an
		// absence, so it cannot come off the deal rows the two sweeps above
		// read — and asking it before the filter would pay for deals that are
		// about to be dropped.
		if err := markNoOpenNextStep(ctx, pool, out, now); err != nil {
			return nil, false, err
		}
		return out, cut, nil
	}
}

// markNoOpenNextStep sets NoOpenNextStep on each candidate, in one read.
func markNoOpenNextStep(ctx context.Context, pool *pgxpool.Pool, candidates []agents.SlippingDeal, now time.Time) error {
	dealIDs := make([]ids.UUID, 0, len(candidates))
	for _, d := range candidates {
		dealIDs = append(dealIDs, d.DealID)
	}
	stepless, err := dealsWithNoOpenNextStep(ctx, pool, dealIDs, now)
	if err != nil {
		return err
	}
	for i := range candidates {
		candidates[i].NoOpenNextStep = stepless[candidates[i].DealID]
	}
	return nil
}

// quietDealTail continues the candidate set quietDealScanWithClock sweeps, past
// its sweeps. Its predicate is the union of the two sweeps' filters, ordered by
// expected close, so every candidate is reached once. Each candidate carries
// the position after it.
func quietDealTail(pool *pgxpool.Pool, quietForDays int) func(
	ctx context.Context, asOf time.Time, owners *storekit.OwnerSet, after string, limit int,
) ([]agents.SlippingDeal, []string, bool, error) {
	store := deals.NewStore(InstallationDB(pool), DealsInstallation())
	return func(ctx context.Context, asOf time.Time, owners *storekit.OwnerSet, after string, limit int) ([]agents.SlippingDeal, []string, bool, error) {
		zone, err := installationZone(ctx, pool)
		if err != nil {
			return nil, nil, false, err
		}
		in := atRiskList(quietForDays, asOf, zone, owners)
		in.Limit = &limit
		if after != "" {
			in.Cursor = &after
		}
		read, positions, page, err := store.ListDealsPositioned(ctx, in)
		if err != nil {
			return nil, nil, false, err
		}
		out := make([]agents.SlippingDeal, len(read))
		for i, d := range read {
			out[i] = slippingCandidate(d, asOf, zone)
		}
		if err := markNoOpenNextStep(ctx, pool, out, asOf); err != nil {
			return nil, nil, false, err
		}
		return out, positions, page.HasMore, nil
	}
}

// installationZone loads the zone the installation's days are counted in.
func installationZone(ctx context.Context, pool *pgxpool.Pool) (*time.Location, error) {
	var name string
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		name, err = identity.TimezoneOf(ctx, tx)
		return err
	}); err != nil {
		return nil, fmt.Errorf("read the installation's timezone: %w", err)
	}
	return zoneNamed(name), nil
}

// zoneNamed resolves a stored zone name, answering UTC for one Go cannot load.
//
// The same fallback the deals module's own date arithmetic makes for a nil
// zone, so a setting nobody can load cannot make the two surfaces disagree in a
// NEW way — they are then both wrong about the day in the same direction, which
// is a different and much smaller problem.
func zoneNamed(name string) *time.Location {
	if zone, err := time.LoadLocation(name); err == nil {
		return zone
	}
	return time.UTC
}

// slippingCandidate carries a deal row across the seam with its risk
// flags and the fields that evidence them; the tool drops any flag its
// evidence field cannot ground.
func slippingCandidate(d crmcontracts.Deal, now time.Time, zone *time.Location) agents.SlippingDeal {
	candidate := agents.SlippingDeal{
		DealID:               ids.UUID(d.Id),
		Name:                 d.Name,
		AmountMinor:          d.AmountMinor,
		Currency:             d.Currency,
		Stalled:              d.Stalled != nil && *d.Stalled,
		LastActivityAt:       d.LastActivityAt,
		CreatedAt:            d.CreatedAt,
		CloseDateProvisional: d.CloseDateProvisional,
		ForecastCategory:     forecastCategoryName(d.ForecastCategory),
	}
	if d.StageId != nil {
		stage := ids.UUID(*d.StageId)
		candidate.StageID = &stage
	}
	if d.OwnerId != nil {
		owner := ids.UUID(*d.OwnerId)
		candidate.OwnerID = &owner
	}
	if d.ExpectedCloseDate != nil {
		closeDate := d.ExpectedCloseDate.Time
		candidate.ExpectedCloseDate = &closeDate
		// The deals module's own judgement, not a second one: the flag on the
		// record and the answer this tool gives are the same question, and two
		// implementations of it disagree the moment their "today" does.
		candidate.CloseOverdue = deals.CloseIsOverdue(closeDate, now, zone)
	}
	return candidate
}

// followUpDrafter persists one follow-up as a draft NOTE on the deal's
// timeline — a note never reads as a sent (or even existing) email, so
// the draft cannot masquerade as communication that happened. It shares
// the deterministic draft voice with draft_email.
func followUpDrafter(provider datasource.SystemOfRecordProvider) agents.FollowUpDrafter {
	return func(ctx context.Context, deal agents.SlippingDeal) (ids.UUID, string, error) {
		// A slipping deal is by definition one nobody has touched lately, so
		// the floor writes at the weeks band: it acknowledges the gap instead
		// of opening as if the conversation were still live. The deal name is
		// the topic, not a thread subject, so it earns no reply prefix.
		subject, body := activities.DeterministicEmailDraft(activities.DraftContext{
			Topic: deal.Name,
			Band:  convstate.BandWeeks,
		})
		fields, err := json.Marshal(map[string]any{
			"kind":    "note",
			"subject": "Draft follow-up: " + subject,
			"body":    body,
			"links": []map[string]any{
				{"entity_type": "deal", "entity_id": deal.DealID},
			},
		})
		if err != nil {
			return ids.Nil, "", err
		}
		ref, err := provider.Create(ctx, datasource.CreateInput{
			EntityType: datasource.EntityActivity,
			Fields:     fields,
			// A contact asked for this through an assistant, which is the same
			// origin as a contact asking through a form; captured_by comes from
			// the principal, never from here, and it is what says which.
			Source: agents.ToolSource,
		})
		if err != nil {
			return ids.Nil, "", err
		}
		return ref.ID, subject, nil
	}
}

func forecastCategoryName(category *crmcontracts.DealForecastCategory) *string {
	if category == nil {
		return nil
	}
	name := string(*category)
	return &name
}

// atRiskList is the list read of the whole at-risk candidate set, by expected close.
func atRiskList(quietForDays int, asOf time.Time, zone *time.Location, owners *storekit.OwnerSet) deals.ListDealsInput {
	sort := deals.AtRiskSort
	return deals.ListDealsInput{
		AtRisk: &deals.AtRiskWindow{QuietForDays: quietForDays, CloseBefore: storekit.WorkspaceDay(asOf, zone)},
		Sort:   &sort, ScopeOwners: owners,
	}
}

// quietDealsAmong answers which of these deals are in the candidate set
// quietDealTail continues.
func quietDealsAmong(pool *pgxpool.Pool, quietForDays int) func(
	ctx context.Context, asOf time.Time, owners *storekit.OwnerSet, among []ids.UUID,
) (map[ids.UUID]bool, error) {
	store := deals.NewStore(InstallationDB(pool), DealsInstallation())
	return func(ctx context.Context, asOf time.Time, owners *storekit.OwnerSet, among []ids.UUID) (map[ids.UUID]bool, error) {
		zone, err := installationZone(ctx, pool)
		if err != nil {
			return nil, err
		}
		in := atRiskList(quietForDays, asOf, zone, owners)
		in.IDs, in.Limit = among, new(len(among))
		read, _, err := store.ListDeals(ctx, in)
		if err != nil {
			return nil, err
		}
		held := make(map[ids.UUID]bool, len(read))
		for _, d := range read {
			held[ids.UUID(d.Id)] = true
		}
		return held, nil
	}
}
