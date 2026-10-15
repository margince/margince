// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The held-key refusals. An address, a domain or a LinkedIn URL names one
// live record across the estate. The store refuses a write that would give it
// to a second one, and the commit skips the row. The preview asks first, so it
// does not promise a landing the commit skips.

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rowKey is one estate-wide unique key a row claims.
type rowKey struct {
	// ledger is the key as the store compares it, which is how the dry run
	// files it for the file's later rows.
	ledger string
	why    migration.Reason
	// held asks the estate whether a live record other than except holds the
	// key. It is nil where the store files the key without asking.
	held func(ctx context.Context, except *ids.UUID) (bool, error)
}

// createPathRefusal is why a create is refused before the collision check, or
// the zero Reason. It refuses a row naming no record, or a unique key already
// held. The preview and the commit both ask it first. Not visibility-filtered,
// because the store refuses a held key whoever holds it. A refused row then
// reads the same whoever holds the key, and the answer is no existence oracle.
func (w *csvWriters) createPathRefusal(ctx context.Context, row migration.Row, typed map[string]any) (migration.Reason, error) {
	fields := textFields(row.Fields)
	if why := unnamedWhy(w.object, fields); why.Code != "" {
		return why, nil
	}
	return w.heldKeyRefusal(ctx, w.heldKeysOf(fields, typed, false), "row:"+row.ExternalID, nil)
}

// updateKeyRefusal is reconcile's skip reason for a changed unique key another
// live record holds, or the zero Reason.
func (w *csvWriters) updateKeyRefusal(
	ctx context.Context, id ids.UUID, changed map[string]string, typed map[string]any,
) (migration.Reason, error) {
	return w.heldKeyRefusal(ctx, w.heldKeysOf(changed, typed, true), "id:"+id.String(), &id)
}

// heldKeyRefusal walks the row's keys in the store's order. Each key is asked
// of the estate and then of the file's earlier rows. A row two keys would
// refuse then names the one the commit names.
func (w *csvWriters) heldKeyRefusal(ctx context.Context, keys []rowKey, holder string, except *ids.UUID) (migration.Reason, error) {
	w.earlier.claim(holder, keys)
	for _, key := range keys {
		if key.held == nil {
			continue
		}
		held, err := key.held(ctx, except)
		if err != nil {
			return migration.Reason{}, err
		}
		if held || w.earlier.heldByAnother(key.ledger, holder) {
			return key.why, nil
		}
	}
	return migration.Reason{}, nil
}

// heldKeysOf is the unique keys these fields claim, in the order the store
// checks them. On an update, fields are the changed ones.
//
// A contact's LinkedIn handle is no key. The lead store checks a lead's
// LinkedIn URL on create and not on update. An update still files it, because
// once it lands the store refuses the URL to a later create.
func (w *csvWriters) heldKeysOf(fields map[string]string, typed map[string]any, update bool) []rowKey {
	var keys []rowKey
	email := canonicalFor(fieldEmail, fields[fieldEmail])
	url, hasURL := typed[fieldLinkedIn].(string)
	hasURL = hasURL && fields[fieldLinkedIn] != ""
	switch w.object {
	case migration.ObjectContact:
		if email != "" {
			why := contactEmailClaimedWhy
			if update {
				why = duplicateEmailWhy
			}
			given := strings.TrimSpace(fields[fieldEmail])
			keys = append(keys, rowKey{"email:" + email, why, func(ctx context.Context, except *ids.UUID) (bool, error) {
				return w.contactEmailHeld(ctx, given, except)
			}})
		}
	case migration.ObjectCompany:
		if domain := canonicalFor(fieldDomain, fields[fieldDomain]); domain != "" {
			keys = append(keys, rowKey{"domain:" + domain, domainClaimedWhy, func(ctx context.Context, except *ids.UUID) (bool, error) {
				return w.companyDomainHeld(ctx, domain, except)
			}})
		}
		// The company key folds case, as company_linkedin_url_key does.
		if hasURL {
			keys = append(keys, rowKey{"linkedin:" + strings.ToLower(url), linkedInClaimedWhy, func(ctx context.Context, except *ids.UUID) (bool, error) {
				return w.companyLinkedInHeld(ctx, url, except)
			}})
		}
	case migration.ObjectLead:
		if email != "" {
			keys = append(keys, rowKey{"email:" + email, leadEmailClaimedWhy, func(ctx context.Context, except *ids.UUID) (bool, error) {
				return w.leadEmailHeld(ctx, email, except)
			}})
		}
		if hasURL {
			key := rowKey{ledger: "linkedin:" + url, why: linkedInClaimedWhy}
			if !update {
				key.held = func(ctx context.Context, _ *ids.UUID) (bool, error) { return w.leadLinkedInHeld(ctx, url) }
			}
			keys = append(keys, key)
		}
	}
	return keys
}
