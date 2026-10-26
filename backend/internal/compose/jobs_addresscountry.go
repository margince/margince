// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The address-country check: a periodic pass that asks the geocoder which
// country a record's postal code and city lie in. The read then flags a stored
// country that does not fit. The pass runs on the geocode queue and asks
// through the client geocode_company uses. So the one pacer and the one worker
// hold it to the provider's rate.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/geocode"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AddressCountryCheckArgs is one pass. It carries no workspace, for
// GeocodeBackfillArgs's reason: the pass walks every workspace itself.
type AddressCountryCheckArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (AddressCountryCheckArgs) Kind() string { return "address_country_check" }

// InsertOpts carries the declared attempt cap and the backfill's priority, so an
// address a rep just wrote is located before this pass runs.
func (AddressCountryCheckArgs) InsertOpts() river.InsertOpts {
	return *jobs.QueuedAs[AddressCountryCheckArgs](&river.InsertOpts{
		MaxAttempts: 3,
		Priority:    geocodeBackfillPriority,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	})
}

// addressCountryLookupsPerRun bounds the provider requests of one pass. At the
// pacer's 15 seconds each, four hold the single geocode worker for about a
// minute. Answers from the place cache do not count.
const addressCountryLookupsPerRun = 4

// addressCountryRowsPerRead bounds the rows one read of due records returns.
const addressCountryRowsPerRead = 50

// addAddressCountryJobs registers the pass and returns its schedule. With no
// geocoder there is nobody to ask, so nothing is registered.
func addAddressCountryJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig) []*river.PeriodicJob {
	if cfg.Geocoder == nil {
		return nil
	}
	addDeclaredWorker[AddressCountryCheckArgs](reg, newAddressCountryWorker(pool, cfg.Geocoder, time.Now))
	return periodicFor(cfg, AddressCountryCheckArgs{})
}

// addressCountryWorker runs one pass of the check.
type addressCountryWorker struct {
	pool     *pgxpool.Pool
	geocoder geocode.Client
	clock    func() time.Time
}

func newAddressCountryWorker(pool *pgxpool.Pool, geocoder geocode.Client, clock func() time.Time) *addressCountryWorker {
	return &addressCountryWorker{pool: pool, geocoder: geocoder, clock: clock}
}

// Work checks due records across every workspace until the lookup budget is spent.
// A lookup that does not complete ends the pass with its error; the record stays
// unchecked, and the next pass asks again.
func (w *addressCountryWorker) Work(ctx context.Context, _ *river.Job[AddressCountryCheckArgs]) error {
	workspaces, err := enumerateWorkspaces(ctx, w.pool)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	pass := addressCountryPass{worker: w, lookupsLeft: addressCountryLookupsPerRun}
	for _, ws := range workspaces {
		if err := pass.checkWorkspace(ctx, ws); err != nil {
			return jobs.FaultContext(ctx, err)
		}
		if pass.lookupsLeft == 0 {
			break
		}
	}
	if pass.checked > 0 {
		slog.InfoContext(ctx, "address country check recorded a batch",
			"records", pass.checked, "lookups", addressCountryLookupsPerRun-pass.lookupsLeft)
	}
	return nil
}

// addressCountryPass is the budget and tally one Work call shares across workspaces.
type addressCountryPass struct {
	worker      *addressCountryWorker
	lookupsLeft int
	checked     int
}

func (p *addressCountryPass) checkWorkspace(ctx context.Context, ws ids.UUID) error {
	wsCtx := principal.SystemActing(principal.WithWorkspaceID(ctx, ws), "system:address-country-check")
	store := contacts.NewStore(database.Bind(p.worker.pool, func(context.Context) (ids.WorkspaceID, error) {
		return ids.From[ids.WorkspaceKind](ws), nil
	})).WithClock(p.worker.clock)
	for _, record := range contacts.CountryCheckedRecords {
		due, err := store.ListAddressCountryChecksDue(wsCtx, record, addressCountryRowsPerRead)
		if err != nil {
			return err
		}
		for _, check := range due {
			done, err := p.checkOne(wsCtx, store, check)
			if err != nil || !done {
				return err
			}
		}
	}
	return nil
}

// checkOne answers one record from the country cache when it can, and from the
// provider while the budget lasts. done is false once the budget is spent.
func (p *addressCountryPass) checkOne(ctx context.Context, store *contacts.Store, check contacts.AddressCountryCheck) (bool, error) {
	query := check.Query()
	codes, hit, err := store.LookupPlaceCountries(ctx, query)
	if err != nil {
		return false, err
	}
	if !hit {
		if p.lookupsLeft == 0 {
			return false, nil
		}
		p.lookupsLeft--
		if codes, err = p.worker.geocoder.Countries(ctx, query); err != nil {
			return false, fmt.Errorf("asking which countries a postal code and city lie in: %w", err)
		}
		if err := store.RememberPlaceCountries(ctx, query, codes, geocodeProvider); err != nil {
			return false, err
		}
	}
	written, err := store.RecordAddressCountries(ctx, check, codes)
	if written {
		p.checked++
	}
	return true, err
}
