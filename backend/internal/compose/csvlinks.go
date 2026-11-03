// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Finding the record a link names, for the dry run and the commit alike.
//
// A link cell names a record three ways, tried in this order. First by its ref,
// as a row of this upload or of an earlier import under the same source name.
// Then by its natural key, then by its id, as a record the CRM holds. Every step
// reads only what the caller may see. A record outside their row scope answers
// in the same words as one that does not exist.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// linkKey is one ref of one record type.
type linkKey struct{ Object, Key string }

// linkAnswer is what a ref resolved to. Pending is a row of this upload that
// will land but has not yet, which only the dry run sees.
type linkAnswer struct {
	ID      ids.UUID
	Pending bool
	Found   bool
	Why     migration.Reason
}

// resolves reports whether the link can be written when the run lands.
func (a linkAnswer) resolves() bool { return a.Found || a.Pending }

// linkStep is one way of finding a record; the resolver tries them in order.
type linkStep func(ctx context.Context, object, key string) (linkAnswer, error)

// linkResolver is shared by every part of one run, so a row of one file can
// name a row of another.
type linkResolver struct {
	sourceSystem string
	identities   *migration.RunStore
	pool         *pgxpool.Pool
	pending      map[linkKey]bool
	// refused holds the rows of this run that were skipped, so a link to one
	// says so instead of claiming the row never existed.
	refused map[linkKey]bool
	// pendingNatural holds the natural keys of those rows (a contact's email,
	// a company's domain), so a link written with one resolves as the commit's does.
	pendingNatural map[linkKey]bool
	// landed holds the rows of this slice that named their record by id, so a
	// link answers before the identity map is read.
	landed map[linkKey]ids.UUID
	cache  map[linkKey]linkAnswer
	// employers caches one answer per folded company name, bounded by the
	// names the file carries rather than by the estate.
	employers map[string]employerCandidate
	// names holds, per folded company name, the rows of this upload the dry
	// run predicts will land under it.
	names                  map[string]pendingName
	plannedEmployerNames   map[string]bool
	plannedEmployerDomains map[string]string

	byBinding, byNaturalKey, byID linkStep
}

func newLinkResolver(db *database.DB, sourceSystem string) *linkResolver {
	r := &linkResolver{
		sourceSystem:   sourceSystem,
		identities:     migration.NewRunStore(db),
		pool:           db.Pool(),
		pending:        map[linkKey]bool{},
		refused:        map[linkKey]bool{},
		pendingNatural: map[linkKey]bool{},
		landed:         map[linkKey]ids.UUID{},
		cache:          map[linkKey]linkAnswer{},
		employers:      map[string]employerCandidate{},
		names:          map[string]pendingName{},
	}
	r.byBinding, r.byNaturalKey, r.byID = r.bindingOf, r.naturalKeyOf, r.visibleByID
	return r
}

// linkNotFoundWhy is the one sentence for a ref no step answers to, whether
// nothing carries it or only records the caller cannot see do.
func linkNotFoundWhy(object, ref string) migration.Reason {
	code := migration.ReasonLinkNotFound
	switch object {
	case migration.ObjectContact:
		code = migration.ReasonContactNotFound
	case migration.ObjectLead:
		code = migration.ReasonLeadNotFound
	case migration.ObjectDeal:
		code = migration.ReasonDealNotFound
	}
	return migration.Why(code, "ref", strings.TrimSpace(ref))
}

// markPending records a row the dry run predicts will land, so a later file's
// link to it resolves before the row exists.
func (r *linkResolver) markPending(object, ref string) {
	key := linkKey{Object: object, Key: strings.TrimSpace(ref)}
	r.pending[key] = true
	delete(r.cache, key)
}

// markRefused records a row this run skipped. A link to it follows the run's
// broken-link policy, in the dry run and in the commit alike.
func (r *linkResolver) markRefused(object, ref string) {
	key := linkKey{Object: object, Key: strings.TrimSpace(ref)}
	if r.refused == nil {
		r.refused = map[linkKey]bool{}
	}
	r.refused[key] = true
	delete(r.cache, key)
}

// naturalKeyFor is the key the commit's natural-key step reads a link by: a
// contact's folded email, a company's canonical domain. Marking a pending row
// and looking a link up both spell the key here.
func naturalKeyFor(object, raw string) (linkKey, bool) {
	raw = strings.TrimSpace(raw)
	switch {
	case object == migration.ObjectContact && strings.Contains(raw, "@"):
		return linkKey{Object: object, Key: canonicalFor(fieldEmail, raw)}, true
	case object == migration.ObjectCompany && strings.Contains(raw, ".") && !strings.ContainsRune(raw, ' '):
		return linkKey{Object: object, Key: canonicalFor(fieldDomain, raw)}, true
	}
	return linkKey{}, false
}

// markNaturalPending records the natural key of a row the dry run predicts will
// land. A link naming it by email or domain then resolves before the row exists.
func (r *linkResolver) markNaturalPending(object, raw string) {
	if key, ok := naturalKeyFor(object, raw); ok {
		r.pendingNatural[key] = true
		clear(r.cache)
	}
}

// pendingName counts the rows of this upload that will land under one company
// name. creating is how many of them make a new company.
type pendingName struct{ landing, creating int }

// markNamesPending records the names of a company row the dry run predicts will
// land. A later file's name link to it then resolves before the row exists.
// Only the dry run calls it, so a commit's resolver answers from the estate alone.
func (r *linkResolver) markNamesPending(creates bool, names ...string) {
	seen := map[string]bool{}
	for _, name := range names {
		key := employerKey(name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		p := r.names[key]
		p.landing++
		if creates {
			p.creating++
		}
		r.names[key] = p
	}
}

// over is a name's answer once this upload's rows have landed. A created
// company is one more holder of the name. An updated one is already counted,
// unless the row renames it to this name.
func (p pendingName) over(hit employerCandidate) employerCandidate {
	if p.landing == 0 {
		return hit
	}
	count := max(hit.count+p.creating, 1)
	return employerCandidate{id: hit.id, count: min(count, 2), pending: true}
}

// markLanded records a row of this run that updated the record it named by id.
// A later file's link to its ref then finds that record, as the test run said.
func (r *linkResolver) markLanded(object, ref string, id ids.UUID) {
	key := linkKey{Object: object, Key: strings.TrimSpace(ref)}
	r.landed[key] = id
	delete(r.cache, key)
}

// resolveCompany answers which company a link cell names. A ref is trimmed and
// keeps its case, because it is an identifier.
func (r *linkResolver) resolveCompany(ctx context.Context, raw string) (linkAnswer, error) {
	return r.resolveLink(ctx, migration.ObjectCompany, raw)
}

// resolveLink is resolveCompany for any object a link may name.
func (r *linkResolver) resolveLink(ctx context.Context, object, raw string) (linkAnswer, error) {
	key := linkKey{Object: object, Key: strings.TrimSpace(raw)}
	if answer, ok := r.cache[key]; ok {
		return answer, nil
	}
	answer, err := r.lookup(ctx, key)
	if err != nil {
		return linkAnswer{}, err
	}
	r.cache[key] = answer
	return answer, nil
}

func (r *linkResolver) lookup(ctx context.Context, key linkKey) (linkAnswer, error) {
	if r.pending[key] {
		return linkAnswer{Pending: true}, nil
	}
	if natural, ok := naturalKeyFor(key.Object, key.Key); ok && r.pendingNatural[natural] {
		return linkAnswer{Pending: true}, nil
	}
	if id, ok := r.landed[key]; ok {
		return linkAnswer{ID: id, Found: true}, nil
	}
	answer, err := r.byBinding(ctx, key.Object, key.Key)
	if err != nil || answer.Found {
		return answer, err
	}
	// An ambiguous name stops here with its own reason: an id cannot carry it.
	if answer, err = r.byNaturalKey(ctx, key.Object, key.Key); err != nil || answer.resolves() || answer.Why.Code != "" {
		return answer, err
	}
	if _, parseErr := ids.Parse(key.Key); parseErr == nil {
		if answer, err = r.byID(ctx, key.Object, key.Key); err != nil || answer.Found {
			return answer, err
		}
	}
	if r.refused[key] {
		return linkAnswer{Why: migration.Why(migration.ReasonLinkRowSkipped, "ref", key.Key)}, nil
	}
	return linkAnswer{Why: linkNotFoundWhy(key.Object, key.Key)}, nil
}

// bindingOf finds a row this source landed before, through the identity map,
// and then asks whether the caller may still see it live.
func (r *linkResolver) bindingOf(ctx context.Context, object, ref string) (linkAnswer, error) {
	binding, found, err := r.identities.LookupBinding(ctx, r.sourceSystem, object, ref)
	if err != nil || !found {
		return linkAnswer{}, err
	}
	return r.visible(ctx, object, binding.NativeID)
}

// visibleByID finds a record by the id Margince gave it.
func (r *linkResolver) visibleByID(ctx context.Context, object, raw string) (linkAnswer, error) {
	id, err := ids.Parse(raw)
	if err != nil {
		return linkAnswer{}, fmt.Errorf("import: %q is not an id: %w", raw, err)
	}
	return r.visible(ctx, object, id)
}

// visible answers the record when it exists, is live and is in the caller's
// row scope. A miss on any of the three is the same not-found.
func (r *linkResolver) visible(ctx context.Context, object string, id ids.UUID) (linkAnswer, error) {
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		return auth.EnsureVisibleLive(ctx, tx, object, id)
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return linkAnswer{}, nil
	}
	if err != nil {
		return linkAnswer{}, fmt.Errorf("import: reading the linked %s: %w", object, err)
	}
	return linkAnswer{ID: id, Found: true}, nil
}

// naturalKeyOf finds a company by the key a human would write: its domain, or
// the one name it carries. A contact's is its email address. A lead or a deal
// has none a file can write. It is found by ref or id, never by a name that
// happens to be a company's.
func (r *linkResolver) naturalKeyOf(ctx context.Context, object, key string) (linkAnswer, error) {
	switch object {
	case migration.ObjectContact:
		return r.visibleContactByEmail(ctx, key)
	case migration.ObjectLead, migration.ObjectDeal:
		return linkAnswer{}, nil
	}
	// A domain holds no space, so a name such as "Acme Inc." skips the read.
	if strings.Contains(key, ".") && !strings.ContainsRune(key, ' ') {
		answer, err := r.visibleByDomain(ctx, canonicalFor(fieldDomain, key))
		if err != nil || answer.Found {
			return answer, err
		}
	}
	hit, err := r.employerFor(ctx, employerKey(key))
	return employerAnswer(hit, key), err
}

// employerAnswer reads a name's candidates as a link. One links, and several is
// a question for a human. None falls through to the next step.
func employerAnswer(hit employerCandidate, name string) linkAnswer {
	switch {
	case hit.count == 1 && hit.pending:
		return linkAnswer{Pending: true}
	case hit.count == 1:
		return linkAnswer{ID: hit.id.UUID, Found: true}
	case hit.count > 1:
		return linkAnswer{Why: ambiguousEmployerWhy(name)}
	default:
		return linkAnswer{}
	}
}

// visibleByDomain finds the live company holding a domain inside the caller's
// row scope. A domain is unique among live rows, so one row is the whole answer.
func (r *linkResolver) visibleByDomain(ctx context.Context, domain string) (linkAnswer, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return linkAnswer{}, err
	}
	var answer linkAnswer
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		domainPos := arg(domain)
		scope, err := auth.ScopeClauseFor(ctx, "company", "o", arg)
		if err != nil {
			return err
		}
		if scope == "" {
			scope = "TRUE"
		}
		err = tx.QueryRow(ctx, storekit.SQLf(`
			SELECT o.id FROM company_domain d JOIN company o ON o.id = d.company_id
			 WHERE d.domain = $%d AND d.archived_at IS NULL
			   AND o.archived_at IS NULL AND o.merged_into_id IS NULL AND %s
			 LIMIT 1`, domainPos, scope), args...).Scan(&answer.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		answer.Found = err == nil
		return err
	})
	if err != nil {
		return linkAnswer{}, fmt.Errorf("import: finding the linked company: %w", err)
	}
	return answer, nil
}

// naturalKeyField is the column holding a row's natural key. It is empty when
// a record of that type has none a link can name.
func naturalKeyField(object string) string {
	switch object {
	case migration.ObjectContact:
		return fieldEmail
	case migration.ObjectCompany:
		return fieldDomain
	}
	return ""
}
