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
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
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
	Reason  string
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
	cache        map[linkKey]linkAnswer
	// employers caches one answer per folded company name, bounded by the
	// names the file carries rather than by the estate.
	employers map[string]employerCandidate

	byBinding, byNaturalKey, byID linkStep
}

func newLinkResolver(db *database.DB, sourceSystem string) *linkResolver {
	r := &linkResolver{
		sourceSystem: sourceSystem,
		identities:   migration.NewRunStore(db),
		pool:         db.Pool(),
		pending:      map[linkKey]bool{},
		cache:        map[linkKey]linkAnswer{},
		employers:    map[string]employerCandidate{},
	}
	r.byBinding, r.byNaturalKey, r.byID = r.bindingOf, r.naturalKeyOf, r.visibleByID
	return r
}

// linkNotFoundReason is the one sentence for a ref no step answers to, whether
// nothing carries it or only records the caller cannot see do.
func linkNotFoundReason(ref string) string {
	return fmt.Sprintf("no company with %q in this upload, in an earlier import, or in Margince", strings.TrimSpace(ref))
}

// markPending records a row the dry run predicts will land, so a later file's
// link to it resolves before the row exists.
func (r *linkResolver) markPending(object, ref string) {
	key := linkKey{Object: object, Key: strings.TrimSpace(ref)}
	r.pending[key] = true
	delete(r.cache, key)
}

// resolve answers which record a link cell names. A ref is trimmed and keeps
// its case, because it is an identifier; an email folds as the store keeps it.
func (r *linkResolver) resolve(ctx context.Context, object, raw string) (linkAnswer, error) {
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
	answer, err := r.byBinding(ctx, key.Object, key.Key)
	if err != nil || answer.Found {
		return answer, err
	}
	natural := key.Key
	if key.Object == migration.ObjectContact {
		natural = correspondence.Fold(natural)
	}
	// An ambiguous name stops here with its own reason: an id cannot carry it.
	if answer, err = r.byNaturalKey(ctx, key.Object, natural); err != nil || answer.Found || answer.Reason != "" {
		return answer, err
	}
	if _, parseErr := ids.Parse(key.Key); parseErr == nil {
		if answer, err = r.byID(ctx, key.Object, key.Key); err != nil || answer.Found {
			return answer, err
		}
	}
	return linkAnswer{Reason: linkNotFoundReason(key.Key)}, nil
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

// naturalKeyOf finds a record by the key a human would write: a company by
// its domain or its one name, a contact by an address.
func (r *linkResolver) naturalKeyOf(ctx context.Context, object, key string) (linkAnswer, error) {
	switch object {
	case migration.ObjectCompany:
		// Matching a company by its domain or name reads the company itself.
		if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
			return linkAnswer{}, err
		}
		if strings.Contains(key, ".") {
			answer, err := r.visibleByKey(ctx, companyByDomain, canonicalFor(fieldDomain, key))
			if err != nil || answer.Found {
				return answer, err
			}
		}
		hit, err := r.employerFor(ctx, employerKey(key))
		return employerAnswer(hit, key), err
	case migration.ObjectContact:
		if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
			return linkAnswer{}, err
		}
		return r.visibleByKey(ctx, contactByEmail, key)
	default:
		return linkAnswer{}, fmt.Errorf("import: a link cannot name a %q: %w", object, apperrors.ErrConflict)
	}
}

// employerAnswer reads a name's candidates as a link. One links, and several is
// a question for a human. None falls through to the next step.
func employerAnswer(hit employerCandidate, name string) linkAnswer {
	switch {
	case hit.count == 1:
		return linkAnswer{ID: hit.id.UUID, Found: true}
	case hit.count > 1:
		return linkAnswer{Reason: ambiguousEmployerReason(name)}
	default:
		return linkAnswer{}
	}
}

// naturalKeyRead is one table's natural-key match: the record it names and the
// statement that finds it, with `$%[1]d` the key and `%[2]s` the row scope.
type naturalKeyRead struct {
	object, statement string
}

var (
	companyByDomain = naturalKeyRead{migration.ObjectCompany, `
		SELECT o.id FROM company_domain d JOIN company o ON o.id = d.company_id
		 WHERE d.domain = $%[1]d AND d.archived_at IS NULL
		   AND o.archived_at IS NULL AND o.merged_into_id IS NULL AND %[2]s
		 LIMIT 1`}
	contactByEmail = naturalKeyRead{migration.ObjectContact, `
		SELECT o.id FROM contact_email e JOIN contact o ON o.id = e.contact_id
		 WHERE e.email = $%[1]d AND e.archived_at IS NULL
		   AND o.archived_at IS NULL AND o.merged_into_id IS NULL AND %[2]s
		 LIMIT 1`}
)

// visibleByKey runs one natural-key read inside the caller's row scope. Both
// keys are unique among live rows, so one row is the whole answer.
func (r *linkResolver) visibleByKey(ctx context.Context, read naturalKeyRead, key string) (linkAnswer, error) {
	var answer linkAnswer
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		keyPos := arg(key)
		scope, err := auth.ScopeClauseFor(ctx, read.object, "o", arg)
		if err != nil {
			return err
		}
		if scope == "" {
			scope = "TRUE"
		}
		err = tx.QueryRow(ctx, storekit.SQLf(read.statement, keyPos, scope), args...).Scan(&answer.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		answer.Found = err == nil
		return err
	})
	if err != nil {
		return linkAnswer{}, fmt.Errorf("import: finding the linked %s: %w", read.object, err)
	}
	return answer, nil
}
