// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// DuplicateDomainError carries the company already owning a domain: a domain
// maps to at most one company per workspace (data-model §4.2).
type DuplicateDomainError struct {
	Domain     string
	ExistingID ids.CompanyID
}

func (e *DuplicateDomainError) Error() string {
	return "domain " + e.Domain + " already belongs to a company"
}
func (e *DuplicateDomainError) Is(target error) bool { return target == apperrors.ErrConflict }

type CompanyDomainInput struct {
	Domain    string
	IsPrimary bool
}

// parseCompanyDomains is the parse-don't-validate seam for a company's domain
// rows: URL forms, www. prefixes, ports and case all reduce to the one
// normalized host the dedupe index compares (the SQL lower() stays as
// defense in depth). Values are written back in place.
func parseCompanyDomains(domains []CompanyDomainInput) error {
	for i, d := range domains {
		parsed, err := values.ParseDomain(d.Domain)
		if err != nil {
			return err
		}
		domains[i].Domain = parsed.String()
	}
	return nil
}

// claimedDomainOwner is the ONE reading of "somebody else already holds this
// domain", and the one place the disclosure rule for it lives.
//
// `self` is the company allowed to keep its own claim; the zero value means none,
// which is what a CREATE wants. It returns nil when the domain is free or
// already this company's.
//
// THE DISCLOSURE IS THE POINT. A domain maps to at most one company (data-model
// §4.2), so answering "taken" reveals that some company exists with that
// domain — and naming WHICH one reveals a record the caller may not be allowed
// to read. So `ExistingID` is filled only when `auth.VisibleTo` says the caller
// could have read that row anyway; otherwise the 409 stands without it. A
// second copy of an existence-hiding rule is where a leak appears, because
// nothing fails when one copy stops asking.
//
// Held by: TestEveryDomainClaimAnswersThroughOneProbe (backend/gates/domainclaimprobe_test.go)
// — it censuses every statement in the tree that reads company_domain to
// decide whether a domain is taken, and fails on a second one.
// It returns the typed error itself rather than a (value, error) pair: `nil,
// nil` for "free" is an invalid value beside a nil error, and the callers all
// want the same thing — return it if there is one.
func claimedDomainOwner(
	ctx context.Context,
	tx pgx.Tx,
	self ids.CompanyID,
	domain string,
) error {
	var existing ids.CompanyID
	err := tx.QueryRow(ctx,
		`SELECT company_id FROM company_domain
		  WHERE domain = lower($1) AND archived_at IS NULL`,
		domain).Scan(&existing)
	// Non-absence errors return before the ids are compared: a failed Scan
	// leaves `existing` at its zero value, which equals `self`'s zero value on
	// the CREATE path, so the comparison cannot tell a database failure from a
	// free domain.
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("probe domain dedupe: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) || existing == self {
		return nil
	}
	dup := &DuplicateDomainError{Domain: domain}
	visible, verr := auth.VisibleTo(ctx, tx, "company", existing.UUID)
	if verr != nil {
		return verr
	}
	if visible {
		dup.ExistingID = existing
	}
	return dup
}

// ensureCompanyDomainsUnclaimed answers the domain dedupe probe with the
// contract's 409, disclosing the existing company id only when the caller
// could read that row (a domain maps to at most one company per workspace,
// data-model §4.2).
func ensureCompanyDomainsUnclaimed(ctx context.Context, tx pgx.Tx, domains []CompanyDomainInput) error {
	for _, d := range domains {
		// Deliberately creating a company ON a refused domain IS the override.
		// A human who types the domain into a company they are making has said
		// something stronger than any verdict, so the refusal is lifted here
		// rather than the create being rejected — and lifted as `admitted` with
		// a human source, so no later newsletter can quietly put it back.
		//
		// This is the seam EVERY domain claim passes through, which is why the
		// reconciliation lives here: manual create, an edit that adds a domain,
		// and the cold-start accept all reach it, and each of them could
		// otherwise leave a company standing on a domain capture still refuses
		// to attach anybody to.
		if err := admitClaimedDomainTx(ctx, tx, d.Domain); err != nil {
			return err
		}
		// No `self`: a company being CREATED holds no claim to keep.
		if err := claimedDomainOwner(ctx, tx, ids.CompanyID{}, d.Domain); err != nil {
			return err
		}
	}
	return nil
}

// insertCompanyDomains lands the company's domains; the unique index remains the
// structural guarantee under races, mapping uq_company_domain to the typed
// 409 and a second primary domain to a plain conflict.
func insertCompanyDomains(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, source, by string, domains []CompanyDomainInput) error {
	for _, d := range domains {
		if _, err := tx.Exec(ctx,
			`INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
			 VALUES ($1, lower($2), $3, $4, $5)`,
			companyID, d.Domain, d.IsPrimary, source, by); err != nil {
			if name, ok := storekit.UniqueViolation(err); ok {
				if name == "uq_company_domain" {
					return &DuplicateDomainError{Domain: d.Domain}
				}
				return apperrors.ErrConflict // e.g. a second primary domain
			}
			return fmt.Errorf("insert company domain: %w", err)
		}
	}
	return nil
}

// ensureCompanyDomainsUnclaimedExcept is ensureCompanyDomainsUnclaimed for an edit:
// a domain already live on THIS company is not a conflict (keeping it is a
// no-op), only one owned by a DIFFERENT company is. The existing-id is
// disclosed under the same visibility gate as the create-path probe.
func ensureCompanyDomainsUnclaimedExcept(ctx context.Context, tx pgx.Tx, self ids.CompanyID, domains []CompanyDomainInput) error {
	for _, d := range domains {
		if err := claimedDomainOwner(ctx, tx, self, d.Domain); err != nil {
			return err
		}
	}
	return nil
}

// readLiveDomains returns the company's live domains as a lookup set, the
// audit-before rows, and the current primary domain (or "") in one pass.
func readLiveDomains(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) (live map[string]bool, before []map[string]any, currentPrimary string, err error) {
	rows, err := tx.Query(ctx,
		`SELECT domain, is_primary FROM company_domain
		 WHERE company_id = $1 AND archived_at IS NULL`, companyID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("read current domains: %w", err)
	}
	defer rows.Close()
	live = map[string]bool{}
	before = []map[string]any{}
	for rows.Next() {
		var domain string
		var isPrimary bool
		if err := rows.Scan(&domain, &isPrimary); err != nil {
			return nil, nil, "", fmt.Errorf("scan current domain: %w", err)
		}
		live[domain] = true
		if isPrimary {
			currentPrimary = domain
		}
		before = append(before, map[string]any{"domain": domain, "is_primary": isPrimary})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, "", fmt.Errorf("read current domains: %w", err)
	}
	return live, before, currentPrimary, nil
}

// dedupeDomains collapses domains that normalize to the same host (parse
// already normalized them), OR-ing is_primary. A replace-set that names the
// same host twice then reconciles once instead of self-colliding on
// uq_company_domain with a misleading ownership 409.
func dedupeDomains(domains []CompanyDomainInput) []CompanyDomainInput {
	at := map[string]int{}
	out := make([]CompanyDomainInput, 0, len(domains))
	for _, d := range domains {
		if i, ok := at[d.Domain]; ok {
			if d.IsPrimary {
				out[i].IsPrimary = true
			}
			continue
		}
		at[d.Domain] = len(out)
		out = append(out, d)
	}
	return out
}

// livePrimaryDomain answers which domain a company currently holds as
// primary, or "" for none.
//
// One question, one spelling. The patch path needs it to elect on the slice it
// audits and reconcileCompanyDomains needs it to write, and two SELECTs asking it
// would be two definitions of "live" to keep in step with the archival column.
func livePrimaryDomain(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) (string, error) {
	_, _, primary, err := readLiveDomains(ctx, tx, companyID)
	return primary, err
}

// electPrimary makes sure a non-empty domain set names a primary, because an
// company with live domains and none primary is not a state any reader of
// this table can act on.
//
// Three readers key on is_primary and each fails differently without one. The
// auto-enrich sweep INNER JOINs it (capture/autoenrich.go), so such a company is
// never a candidate and is never enriched — silently, with no job, no error and
// no state saying so. The company read derives website_url from it, so the page
// shows no website. Provider enrichment reads it for the employer domain.
//
// Nothing else elects one later: there is no path that promotes a sole domain,
// so a record born this way stays this way. The contract admits the state —
// is_primary defaults to false and only domain is required — which makes an
// agent constructing the minimal valid body the ordinary way to reach it.
//
// current is the primary already live on the record, and keeping it is what
// makes an edit that merely adds a domain leave the choice alone. A caller who
// named a primary is obeyed; only silence is filled in, and it is filled with
// the first domain because the caller offered nothing else to prefer.
func electPrimary(desired []CompanyDomainInput, current string) []CompanyDomainInput {
	if len(desired) == 0 {
		return desired
	}
	for _, d := range desired {
		if d.IsPrimary {
			return desired
		}
	}
	elected := 0
	for i, d := range desired {
		if d.Domain == current {
			elected = i
			break
		}
	}
	out := slices.Clone(desired)
	out[elected].IsPrimary = true
	return out
}

// singleDesiredPrimary returns the one domain marked primary, or "" for
// none. uq_company_domain_primary is a single-row invariant, so more than one
// is the typed 409 up front rather than a constraint failure mid-write.
func singleDesiredPrimary(desired []CompanyDomainInput) (string, error) {
	primary := ""
	count := 0
	for _, d := range desired {
		if d.IsPrimary {
			primary = d.Domain
			count++
		}
	}
	if count > 1 {
		return "", apperrors.ErrConflict
	}
	return primary, nil
}

// reconcileCompanyDomains makes the company's live domain set equal `desired`
// (add missing, archive removed, set the single primary). It returns the
// prior live set as audit-before rows. Primaries are cleared before the new
// one is set so the transient state never trips uq_company_domain_primary, and
// adds reuse insertCompanyDomains so the uniqueness→409 mapping stays one
// spelling. Callers validate the domains (parse + unclaimed) first.
func reconcileCompanyDomains(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, companyID ids.CompanyID, by string, desired []CompanyDomainInput) ([]map[string]any, error) {
	live, before, currentPrimary, err := readLiveDomains(ctx, tx, companyID)
	if err != nil {
		return nil, err
	}
	// The election happens against the LIVE primary, so an edit that only adds
	// a domain keeps the one the record already had rather than moving it to
	// whatever the caller happened to list first.
	//
	// The patch path has already elected on the slice it audits, and this
	// re-elects the same domain from the same inputs — it is not the belt to
	// that braces. It is what makes the rule hold for a caller that reconciles
	// WITHOUT staging, and staging is a property of the HTTP patch rather than
	// of this function.
	primary, err := singleDesiredPrimary(electPrimary(desired, currentPrimary))
	if err != nil {
		return nil, err
	}

	// The primary only moves when it actually changes — a no-op re-submit
	// must not touch the row. Clearing before setting keeps the transient
	// state from racing uq_company_domain_primary against a still-primary row.
	if primary != currentPrimary {
		if _, err := tx.Exec(ctx,
			`UPDATE company_domain SET is_primary = false
			 WHERE company_id = $1 AND archived_at IS NULL AND is_primary`, companyID); err != nil {
			return nil, fmt.Errorf("clear domain primaries: %w", err)
		}
	}

	desiredSet := map[string]bool{}
	var adds []CompanyDomainInput
	for _, d := range desired {
		desiredSet[d.Domain] = true
		if !live[d.Domain] {
			adds = append(adds, CompanyDomainInput{Domain: d.Domain, IsPrimary: false})
		}
	}
	if err := archiveRemovedDomains(ctx, tx, companyID, live, desiredSet); err != nil {
		return nil, err
	}
	if len(adds) > 0 {
		if err := insertCompanyDomains(ctx, tx, companyID, "manual", by, adds); err != nil {
			return nil, err
		}
	}
	if primary != "" && primary != currentPrimary {
		if _, err := tx.Exec(ctx,
			`UPDATE company_domain SET is_primary = true
			 WHERE company_id = $1 AND domain = lower($2) AND archived_at IS NULL`,
			companyID, primary); err != nil {
			return nil, fmt.Errorf("set primary domain: %w", err)
		}
	}
	return before, nil
}

// archiveRemovedDomains soft-deletes the company's live domains absent from the
// desired set.
func archiveRemovedDomains(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, live, desiredSet map[string]bool) error {
	for domain := range live {
		if desiredSet[domain] {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE company_domain SET archived_at = now()
			 WHERE company_id = $1 AND domain = lower($2) AND archived_at IS NULL`,
			companyID, domain); err != nil {
			return fmt.Errorf("archive removed domain: %w", err)
		}
	}
	return nil
}

// domainSummaries renders the desired set as audit-after rows.
func domainSummaries(domains []CompanyDomainInput) []map[string]any {
	out := make([]map[string]any, 0, len(domains))
	for _, d := range domains {
		out = append(out, map[string]any{"domain": d.Domain, "is_primary": d.IsPrimary})
	}
	return out
}

func attachCompanyDomains(ctx context.Context, tx pgx.Tx, companies []crmcontracts.Company) error {
	if len(companies) == 0 {
		return nil
	}
	idx := make(map[openapi_types.UUID]*crmcontracts.Company, len(companies))
	companyIDs := make([]ids.UUID, len(companies))
	for i := range companies {
		idx[companies[i].Id] = &companies[i]
		companyIDs[i] = ids.UUID(companies[i].Id)
	}

	rows, err := tx.Query(ctx,
		`SELECT company_id, id, domain, is_primary, source, captured_by
		 FROM company_domain WHERE company_id = ANY($1) AND archived_at IS NULL
		 ORDER BY is_primary DESC, created_at`, companyIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var companyID, domainID ids.UUID
		var d crmcontracts.CompanyDomain
		if err := rows.Scan(&companyID, &domainID, &d.Domain, &d.IsPrimary, &d.Source, &d.CapturedBy); err != nil {
			return err
		}
		d.Id = openapi_types.UUID(domainID)
		o := idx[openapi_types.UUID(companyID)]
		if o.Domains == nil {
			o.Domains = &[]crmcontracts.CompanyDomain{}
		}
		*o.Domains = append(*o.Domains, d)
		// website_url is DERIVED, never stored (ADR-0085): the primary domain
		// row is the canonical fact and a second column for it would be the
		// duplication that decision closes. The query already orders primary
		// first, so the first row for a company is the one to render.
		if o.WebsiteUrl == nil && d.IsPrimary {
			website := "https://" + d.Domain
			o.WebsiteUrl = &website
		}
	}
	return rows.Err()
}
