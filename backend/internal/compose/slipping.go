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
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// slippingScanLimit bounds each list sweep. An honest bound: a workspace
// with more open deals than this sees the most recently created ones —
// the tool is a triage set, not an exhaustive report (run_report is).
const slippingScanLimit = 50

// slippingLister serves the formulas-§8 candidate set: stalled open
// deals plus open deals whose expected close date is already past.
func slippingLister(pool *pgxpool.Pool) agents.SlippingLister {
	return quietDealLister(pool, deals.StalledThresholdDays)
}

// quietDealLister is slippingLister with the idle window named by the caller.
//
// ONE candidate set, two patiences. The tool surface asks at the stalled
// threshold, because "slipping" is the product-wide status; the morning queue
// asks at the shorter window, because a queue that only speaks after two months
// is reporting rather than warning. Both run this function, so a change to how
// a candidate is built or evidenced reaches both — a second at-risk predicate
// beside this one is the failure this shape exists to prevent.
//
// Deals whose expected close date has passed join the set at ANY window: an
// overdue close is late whatever the idle clock says.
func quietDealLister(pool *pgxpool.Pool, quietForDays int) agents.SlippingLister {
	scan := quietDealScan(pool, quietForDays)
	return func(ctx context.Context) ([]agents.SlippingDeal, error) {
		candidates, _, err := scan(ctx)
		return candidates, err
	}
}

// quietDealScan is quietDealLister with its SCAN DEPTH reported.
//
// The bool says a sweep stopped at its bound, and it travels with the rows
// because it cannot be recovered from them: this reader filters after it scans,
// keeping only what is quiet or overdue out of two bounded sweeps, so ten
// survivors can be all that is left of a full fifty. A caller comparing the
// returned count against the bound would read a truncated scan with few
// survivors as a complete one — under-reporting, which is the one direction a
// figure about somebody's workload must never fail in.
//
// One function with two callers rather than two predicates: quietDealLister
// above is this, with the depth dropped, because agents.SlippingLister is a
// shared type this cannot widen.
func quietDealScan(
	pool *pgxpool.Pool, quietForDays int,
) func(context.Context) ([]agents.SlippingDeal, bool, error) {
	store := deals.NewStore(InstallationDB(pool), DealsInstallation())
	return func(ctx context.Context) ([]agents.SlippingDeal, bool, error) {
		limit := slippingScanLimit
		quiet, _, err := store.ListDeals(ctx, deals.ListDealsInput{
			QuietForDays: &quietForDays, Limit: &limit,
		})
		if err != nil {
			return nil, false, err
		}
		openStatus := "open"
		open, _, err := store.ListDeals(ctx, deals.ListDealsInput{Status: &openStatus, Limit: &limit})
		if err != nil {
			return nil, false, err
		}
		// EITHER sweep at its bound means deals may sit past it. Asked of what
		// was READ, before anything is filtered, which is the whole reason this
		// question cannot be answered from the returned rows.
		cut := len(quiet) >= slippingScanLimit || len(open) >= slippingScanLimit

		// The quiet sweep already applied the window, so its rows are admitted
		// on that ground alone. Testing candidate.Stalled instead would ask the
		// deal row's own 60-day flag, which is FALSE for every deal a shorter
		// window admits — the lane would fetch the right rows and drop them all.
		admitted := map[ids.UUID]bool{}
		for _, d := range quiet {
			admitted[ids.UUID(d.Id)] = true
		}

		// The INSTALLATION's zone, not the server's. A close date is a calendar
		// date a human picked, and asking whether it has passed in UTC gives a
		// different answer from the deal record's own hygiene flag for as long
		// as the two zones disagree about the day — hours wide, every day, for
		// any installation not running in UTC.
		zone, err := installationZone(ctx, pool)
		if err != nil {
			return nil, false, err
		}
		now := time.Now().UTC()
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
			seen[candidate.DealID] = true
			out = append(out, candidate)
		}
		return out, cut, nil
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
		DealID:         ids.UUID(d.Id),
		Name:           d.Name,
		AmountMinor:    d.AmountMinor,
		Currency:       d.Currency,
		Stalled:        d.Stalled != nil && *d.Stalled,
		LastActivityAt: d.LastActivityAt,
		CreatedAt:      d.CreatedAt,
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
		}, "")
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
			// A person asked for this through an assistant, which is the same
			// origin as a person asking through a form; captured_by comes from
			// the principal, never from here, and it is what says which.
			Source: agents.ToolSource,
		})
		if err != nil {
			return ids.Nil, "", err
		}
		return ref.ID, subject, nil
	}
}
