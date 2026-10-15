// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The held-key refusals. An address, a domain or a LinkedIn URL names one
// live record across the estate. The store refuses a write that would give it
// to a second one, and the commit skips the row. The preview asks first, so it
// does not promise a landing the commit skips.

import (
	"context"
	"encoding/json"
	"fmt"
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
	// held asks the estate whether a live record outside except holds the
	// key. It is nil where the store files the key without asking.
	held func(ctx context.Context, except []ids.UUID) (bool, error)
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
// live record holds, or the zero Reason. current is the stored record, whose
// replaced keys the update frees.
func (w *csvWriters) updateKeyRefusal(
	ctx context.Context, id ids.UUID, current []byte, changed map[string]string, typed map[string]any,
) (migration.Reason, error) {
	freed, err := w.replacedKeysOf(current, changed)
	if err != nil {
		return migration.Reason{}, err
	}
	w.earlier.release(id, freed)
	return w.heldKeyRefusal(ctx, w.heldKeysOf(changed, typed, true), "id:"+id.String(), []ids.UUID{id})
}

// heldKeyRefusal walks the row's keys in the store's order. Each key is asked
// of the estate and then of the file's earlier rows. A row two keys would
// refuse then names the one the commit names.
func (w *csvWriters) heldKeyRefusal(ctx context.Context, keys []rowKey, holder string, except []ids.UUID) (migration.Reason, error) {
	w.earlier.claim(holder, keys)
	for _, key := range keys {
		if key.held == nil {
			continue
		}
		held, err := key.held(ctx, w.earlier.leaveOut(key.ledger, except))
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
			keys = append(keys, rowKey{emailKey(email), why, func(ctx context.Context, except []ids.UUID) (bool, error) {
				return w.contactEmailHeld(ctx, given, except)
			}})
		}
	case migration.ObjectCompany:
		if domain := canonicalFor(fieldDomain, fields[fieldDomain]); domain != "" {
			keys = append(keys, rowKey{"domain:" + domain, domainClaimedWhy, func(ctx context.Context, except []ids.UUID) (bool, error) {
				return w.companyDomainHeld(ctx, domain, except)
			}})
		}
		if hasURL {
			keys = append(keys, rowKey{w.linkedInKey(url), linkedInClaimedWhy, func(ctx context.Context, except []ids.UUID) (bool, error) {
				return w.companyLinkedInHeld(ctx, url, except)
			}})
		}
	case migration.ObjectLead:
		if email != "" {
			keys = append(keys, rowKey{emailKey(email), leadEmailClaimedWhy, func(ctx context.Context, except []ids.UUID) (bool, error) {
				return w.leadEmailHeld(ctx, email, except)
			}})
		}
		if hasURL {
			key := rowKey{ledger: w.linkedInKey(url), why: linkedInClaimedWhy}
			if !update {
				key.held = func(ctx context.Context, except []ids.UUID) (bool, error) {
					return w.leadLinkedInHeld(ctx, url, except)
				}
			}
			keys = append(keys, key)
		}
	}
	return keys
}

// replacedKeysOf is the stored keys an update replaces rather than adds to.
// It frees them once it lands. They are a company's LinkedIn URL, and a
// lead's address and LinkedIn URL. Addresses and domains merge, so they free none.
func (w *csvWriters) replacedKeysOf(current []byte, changed map[string]string) ([]string, error) {
	if w.object != migration.ObjectCompany && w.object != migration.ObjectLead {
		return nil, nil
	}
	var stored struct {
		Email       string `json:"email"`
		LinkedInURL string `json:"linkedin_url"`
	}
	if err := json.Unmarshal(current, &stored); err != nil {
		return nil, fmt.Errorf("import: reading the stored keys an update replaces: %w", err)
	}
	var freed []string
	if changed[fieldLinkedIn] != "" && stored.LinkedInURL != "" {
		freed = append(freed, w.linkedInKey(stored.LinkedInURL))
	}
	if w.object == migration.ObjectLead && changed[fieldEmail] != "" && stored.Email != "" {
		freed = append(freed, emailKey(canonicalFor(fieldEmail, stored.Email)))
	}
	return freed, nil
}

// emailKey and linkedInKey spell a key as the store compares it. A company's
// LinkedIn key folds case, as company_linkedin_url_key does; a lead's does not.
func emailKey(email string) string { return "email:" + email }

func (w *csvWriters) linkedInKey(url string) string {
	if w.object == migration.ObjectCompany {
		url = strings.ToLower(url)
	}
	return "linkedin:" + url
}
