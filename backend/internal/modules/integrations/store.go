// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// objectIntegrations is the RBAC object every entry point below gates on.
const objectIntegrations = "integrations"

// The audit image keys this module's rows carry. Spelled once because a spend
// or access investigation filters on them: a typo in one writer would make
// that row invisible to the query that goes looking for it.
const (
	auditKeyProvider = "provider"
	auditKeyMode     = "mode"
	auditKeyPreset   = "preset"
	// The two toggles that decide whether an arrival is bought without anybody
	// asking. They belong in the image for the same reason mode does: an
	// investigation into why credits drained asks who switched automatic
	// buying on, and an audit row that records neither answers that with two
	// identical images.
	auditKeyAutoCreate = "automatic_individual_create"
	auditKeyAutoImport = "automatic_import"
)

// Store owns the four platform tables. Every exported method gates before it
// reads or writes: the connection is installation-wide configuration that
// spends the customer's money, so there is no ungated path to it.
type Store struct {
	// db knows the installation's singleton workspace (ADR-0061/A107), which
	// is where the vault seals this connection's credential. The connection
	// row itself carries no workspace — see doc.go.
	db *database.DB
	// vault custodies the API key. The row holds only an opaque handle.
	vault keyvault.Vault
	// registry is the closed set of adapters this build knows.
	registry *Registry
	now      func() time.Time
	// deleteClaims is the owning domain's own delete, supplied by compose.
	// Nil when no domain is bound, which is also when no claims exist.
	deleteClaims DeleteClaimsFunc

	// The owning domain's callbacks (runs.go). Nil until compose binds them,
	// and QueueRun refuses rather than guessing at a subject's consent.
	fence       FenceSubjectFunc
	cluster     DuplicateClusterFunc
	identifiers SubjectIdentifiersFunc
	// holdSubject is the same question as fence, asked while HOLDING the
	// subject's row. The hand-off uses it and queue time does not: only the
	// hand-off goes on to write about the subject, and only it therefore has
	// an erasure race to close. Nil until compose binds it.
	holdSubject FenceSubjectFunc
	// enqueueSubmit commits the submit job with the run row.
	enqueueSubmit EnqueueSubmitFunc
	// writeClaims is the owning domain's claim upsert (handoff.go). Nil until
	// compose binds it; every hand-off then waits on the sweep and exhausts
	// into claims_unwritten, the honest record for a build with no domain.
	writeClaims WriteClaimsFunc
	// applyStoredClaims folds a purchase already in the domain's table onto the
	// record, for runs that completed before a record could hold them.
	applyStoredClaims ApplyStoredClaimsFunc
	// revertFills takes a purchase back off the records it filled, for the
	// delete-data action.
	revertFills RevertFillsFunc
}

// DeleteClaimsFunc is the owning domain's delete of everything one provider
// asserted, run inside a transaction integrations already holds. It is a func
// type here rather than an interface in shared/ports/provider because that
// package is stdlib-only and cannot name a pgx.Tx — the same shape
// capture.EnqueueBackfill uses. Returns how many claims went.
type DeleteClaimsFunc func(ctx context.Context, tx pgx.Tx, provider string) (int64, error)

// RevertFillsFunc takes one provider's purchases back off the records they
// filled, one contact at a time. It answers which contacts are affected, and
// then reverts each — two calls rather than one, because the second runs in its
// own transaction per subject and this module must not hold an unbounded set of
// people's rows while the eraser wants them.
type RevertFillsFunc struct {
	// Subjects names whose records this provider's purchases wrote to.
	Subjects func(ctx context.Context, tx pgx.Tx, provider string) ([]ids.UUID, error)
	// RevertOne clears what it can on one contact and reports the fields.
	RevertOne func(ctx context.Context, tx pgx.Tx, provider string, subject ids.UUID) ([]string, error)
}

// WithFillReverter binds it. Without it the delete-data action still removes the
// claims and scrubs the ledger; what it cannot do is take the values back off
// the records, and it says so rather than reporting a clean sweep.
func (s *Store) WithFillReverter(fn RevertFillsFunc) *Store {
	s.revertFills = fn
	return s
}

// WithClaimDeleter binds the owning domain's claim delete. Compose calls it;
// without it, DeleteProviderData still scrubs the run ledger it owns.
func (s *Store) WithClaimDeleter(fn DeleteClaimsFunc) *Store {
	s.deleteClaims = fn
	return s
}

// NewStore builds the store. vault and registry are required: a connection
// store that cannot seal a credential or name a provider would accept a key
// it then had nowhere to put.
func NewStore(db *database.DB, vault keyvault.Vault, reg *Registry, now func() time.Time) (*Store, error) {
	if db == nil || vault == nil || reg == nil || now == nil {
		return nil, errors.New("integrations: store needs a db, a vault, a registry and a clock")
	}
	return &Store{db: db, vault: vault, registry: reg, now: now}, nil
}

// Connection is one provider's connection as the surfaces read it. It carries
// no credential material and no vault reference — only whether a key is
// present at all.
// CategoryCost is one category's price, as the settings card and a buy button
// read it.
type CategoryCost struct {
	Category string
	Free     bool
	Cost     map[string]int
}

type Connection struct {
	Provider string
	// Catalog is what this provider sells, with what each entry costs.
	//
	// Held by: TestTheCatalogPricesEveryCategoryTheProviderDeclares
	// (backend/internal/modules/integrations/catalog_test.go) — the entries are
	// derived from the descriptor's own category list, so a provider that adds
	// one cannot leave it unpriced on the settings card.
	Catalog           []CategoryCost
	Status            string
	CredentialPresent bool
	Mode              string
	Preset            string
	AutomaticCreate   bool
	AutomaticImport   bool
	Categories        []string
	RefreshAfterDays  *int
	DailyRunLimit     *int
	Budgets           []PoolBudget
	// Spend is what THIS installation consumed, per month per pool — our
	// ledger, never the provider's balance beside it (PI-FORM-3).
	Spend []MonthlySpend
	// Backlog is how many contacts still owe a lookup, and whether the sweep
	// is moving. Present only for a connected provider: a card offering to
	// connect has no backlog to report.
	Backlog        *BacklogCount
	Version        int64
	SafeStatusCode string
	ConnectedAt    *time.Time
	LastVerifiedAt *time.Time
	LastUsedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// PoolBudget is one credit pool's ceiling, pause threshold and last known
// balance.
type PoolBudget struct {
	Pool              string
	MonthlyCeiling    *int
	PauseBelowBalance *int
	LastKnownBalance  *int
	BalanceReadAt     *time.Time
}

// List returns every registered provider's connection state, including the
// providers that have no row yet — a card that only appeared after connecting
// would give the admin nowhere to connect FROM.
func (s *Store) List(ctx context.Context) ([]Connection, error) {
	if err := auth.Require(ctx, objectIntegrations, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []Connection
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := s.loadConnections(ctx, tx)
		if err != nil {
			return err
		}
		for _, name := range s.registry.Names() {
			d, err := s.registry.Descriptor(name)
			if err != nil {
				return err
			}
			if c, ok := rows[name]; ok {
				// The card shows what the provider says is LEFT and what this
				// installation SPENT side by side, so both arrive in one read
				// — two round trips could show a balance and a history from
				// different moments.
				spend, err := s.readSpendHistory(ctx, tx, name)
				if err != nil {
					return err
				}
				c.Spend = spend
				c.Catalog = catalogOf(d)
				// Read inside this transaction, beside the spend, so the card
				// shows a balance, a history and a backlog from one moment
				// rather than three.
				backlog, err := s.backlogInTx(ctx, tx, name)
				if err != nil {
					return err
				}
				c.Backlog = &backlog
				out = append(out, c)
				continue
			}
			// Never connected: report the honest zero state rather than
			// omitting the provider entirely.
			out = append(out, Connection{
				Provider: name,
				Status:   "disconnected",
				Mode:     string(defaultMode),
				Preset:   d.DefaultPreset,
				// The free categories, not the descriptor's default preset:
				// what an admin is first offered should be the set that costs
				// them nothing, and the priced ones an explicit choice.
				Categories: categoryStrings(d.Free()),
				Catalog:    catalogOf(d),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Get reads one provider's connection.
func (s *Store) Get(ctx context.Context, name string) (Connection, error) {
	all, err := s.List(ctx)
	if err != nil {
		return Connection{}, err
	}
	for _, c := range all {
		if c.Provider == name {
			return c, nil
		}
	}
	return Connection{}, apperrors.ErrNotFound
}

// loadConnections reads every connection row plus its budgets, keyed by
// provider. It does NOT gate: callers above have already done so.
func (s *Store) loadConnections(ctx context.Context, tx pgx.Tx) (map[string]Connection, error) {
	rows, err := tx.Query(ctx, `
		SELECT provider, status, credential_ref IS NOT NULL, mode, preset,
		       automatic_individual_create, automatic_import, categories,
		       refresh_after_days, daily_run_limit, version, last_safe_status_code,
		       connected_at, last_verified_at, last_used_at, created_at, updated_at
		  FROM provider_connection`)
	if err != nil {
		return nil, fmt.Errorf("integrations: reading connections: %w", err)
	}
	defer rows.Close()

	out := map[string]Connection{}
	for rows.Next() {
		var c Connection
		var safeCode *string
		if err := rows.Scan(&c.Provider, &c.Status, &c.CredentialPresent, &c.Mode, &c.Preset,
			&c.AutomaticCreate, &c.AutomaticImport, &c.Categories,
			&c.RefreshAfterDays, &c.DailyRunLimit, &c.Version, &safeCode,
			&c.ConnectedAt, &c.LastVerifiedAt, &c.LastUsedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("integrations: scanning a connection: %w", err)
		}
		if safeCode != nil {
			c.SafeStatusCode = *safeCode
		}
		out[c.Provider] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("integrations: reading connections: %w", err)
	}
	return s.attachBudgets(ctx, tx, out)
}

func (s *Store) attachBudgets(ctx context.Context, tx pgx.Tx, conns map[string]Connection) (map[string]Connection, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.provider, b.pool, b.monthly_ceiling, b.pause_below_balance,
		       b.last_known_balance, b.balance_read_at
		  FROM provider_connection_budget b
		  JOIN provider_connection c ON c.id = b.connection_id
		 ORDER BY c.provider, b.pool`)
	if err != nil {
		return nil, fmt.Errorf("integrations: reading budgets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var b PoolBudget
		if err := rows.Scan(&name, &b.Pool, &b.MonthlyCeiling, &b.PauseBelowBalance,
			&b.LastKnownBalance, &b.BalanceReadAt); err != nil {
			return nil, fmt.Errorf("integrations: scanning a budget: %w", err)
		}
		c := conns[name]
		c.Budgets = append(c.Budgets, b)
		conns[name] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("integrations: reading budgets: %w", err)
	}
	return conns, nil
}

func categoryStrings(cats []provider.Category) []string {
	out := make([]string, 0, len(cats))
	for _, c := range cats {
		out = append(out, string(c))
	}
	return out
}

// catalogOf is what each category costs, derived from the descriptor so a
// price and the fact of being free can never disagree.
func catalogOf(d provider.Descriptor) []CategoryCost {
	free := map[provider.Category]bool{}
	for _, c := range d.Free() {
		free[c] = true
	}
	out := make([]CategoryCost, 0, len(d.Categories))
	for _, category := range d.Categories {
		// Priced WITH its trigger where it has one. A cascade bills only when
		// both its own category and the category it follows were requested,
		// so pricing the fallback alone reads as free — which is exactly the
		// understatement a button must not make about what it can spend.
		cost, err := d.WorstCase(pricedWith(d, category))
		if err != nil {
			// An unmetered or subscription provider prices nothing per
			// category; the whole catalog reads free, which it is.
			cost = map[provider.Pool]int{}
		}
		priced := map[string]int{}
		for pool, n := range cost {
			if n > 0 {
				priced[string(pool)] = n
			}
		}
		out = append(out, CategoryCost{
			Category: string(category),
			Free:     free[category],
			Cost:     priced,
		})
	}
	return out
}

// pricedWith is the category set whose worst case is the true price of asking
// for one category: itself, plus the trigger of any cascade it is the fallback
// for.
//
// ONE hop. No adapter declares a cascade whose trigger is itself a fallback,
// and a chain would need this to walk it — said here rather than built for,
// because the descriptor that needed it would also be the one to prove what
// walking should cost.
func pricedWith(d provider.Descriptor, category provider.Category) []provider.Category {
	out := []provider.Category{category}
	for _, cascade := range d.Cascades {
		if cascade.Category == category {
			out = append(out, cascade.After)
		}
	}
	return out
}

func categoriesFrom(raw []string) []provider.Category {
	out := make([]provider.Category, 0, len(raw))
	for _, c := range raw {
		out = append(out, provider.Category(c))
	}
	return out
}

// emptyCredits is the "we did not read a balance" value for a policy update:
// patching a ceiling is not a reason to call the provider.
func emptyCredits() provider.Credits { return provider.Credits{} }
