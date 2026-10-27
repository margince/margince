// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The lead identity probes (ADR-0008 keeps leads out of contact matching, so
// they carry exact keys of their own: a live email and a LinkedIn profile
// URL). Two write shapes create leads — the direct create and the
// evidence-backed capture — and they answer a hit with deliberately
// different policies: the direct create refuses with the incumbent's id,
// capture stages a merge proposal for a human. What they must NOT hold
// separately is the question itself, which is why the SQL lives here rather
// than once per module: the modules cannot import each other, and a second
// copy of "is this identity already claimed" is how the two drifted apart
// before (a LinkedIn URL was probed by neither).
//
// This is the same seam EmailSuppressed already occupies — a tx-carrying
// predicate that both contacts and capture call.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// LeadSourceKey names the source record a probe must not report against
// itself. Re-capturing one message is a replay, not a collision, and without
// this the second delivery of the same mail would stage a merge proposal
// pairing a lead with itself.
type LeadSourceKey struct {
	SourceSystem string
	SourceID     string
}

// LiveLeadClause is what makes a lead a live identity. Promotion,
// disqualification and a lead merge all archive the lead they retire. So this
// one column is the whole lifecycle test, as in uq_lead_email_dedupe.
func LiveLeadClause(alias string) string {
	return alias + ".archived_at IS NULL"
}

// LeadEmailMatchClause is an exact address match against a lead. The address
// is folded the way lead_email_norm stores it and capture stamps a participant.
func LeadEmailMatchClause(alias, address string) string {
	return alias + ".email = lower(btrim(" + address + "))"
}

// LeadEmailHeldAtClause asks that address was already the lead's when the
// activity under alias activity happened, placed by occurredday.StartSQL. The
// lead's first change of email after that moment must start from its current address.
//
// Mail from before the lead was created is its history only for a lead an
// import run created. Even then no other lead, live or retired, may hold or
// have held the address. Erasure nulls an address without recording it, so
// the import rule bounds what the history cannot see.
func LeadEmailHeldAtClause(alias, address, activity string) string {
	folded := "lower(btrim(" + address + "))"
	at := occurredday.StartSQL(activity)
	return `(` + at + ` >= ` + alias + `.created_at OR (` + leadImportedClause(alias) + ` AND NOT (
		EXISTS (SELECT 1 FROM lead other_holder
		         WHERE other_holder.id <> ` + alias + `.id AND other_holder.email = ` + folded + `)
		OR EXISTS (SELECT 1 FROM audit_log given_up
		         WHERE given_up.entity_type = 'lead' AND given_up.entity_id <> ` + alias + `.id
		           AND given_up.before ? 'email' AND lower(given_up.before->>'email') = ` + folded + `))))
	   AND NOT EXISTS (SELECT 1 FROM (
		SELECT email_change.before->>'email' AS held FROM audit_log email_change
		 WHERE email_change.entity_type = 'lead' AND email_change.entity_id = ` + alias + `.id
		   AND email_change.before ? 'email' AND email_change.after ? 'email'
		   AND email_change.occurred_at > ` + at + `
		 ORDER BY email_change.occurred_at, email_change.id LIMIT 1) first_change
		 WHERE lower(first_change.held) IS DISTINCT FROM ` + alias + `.email)`
}

// leadImportedClause asks whether an import run created the lead: its create
// audit row carries the run as its batch, which only the import job sets.
func leadImportedClause(alias string) string {
	return `EXISTS (SELECT 1 FROM audit_log created JOIN import_run run ON run.id = created.batch_id
		 WHERE created.entity_type = 'lead' AND created.entity_id = ` + alias + `.id AND created.action = 'create')`
}

// LiveLeadByEmail reports the live lead holding this address, if any.
func LiveLeadByEmail(ctx context.Context, tx pgx.Tx, email string, exclude *LeadSourceKey) (ids.LeadID, bool, error) {
	if email == "" {
		return ids.LeadID{}, false, nil
	}
	return liveLeadBy(ctx, tx, LeadEmailMatchClause("lead", "$1"), email, exclude, "email")
}

// LiveLeadByLinkedInURL reports the live lead holding this profile URL. The
// caller normalizes first (contacts.NormalizeLinkedInURL) — a profile URL has
// no SQL-side normalization to fall back on, so an unnormalized argument
// finds nothing rather than matching loosely.
//
// The earliest row wins when a workspace already carries duplicates from
// before this probe existed: idx_lead_linkedin is deliberately NOT unique
// (merging what is already there is a human decision), so the probe has to
// pick the canonical original deterministically rather than assume one row.
func LiveLeadByLinkedInURL(ctx context.Context, tx pgx.Tx, normalizedURL string, exclude *LeadSourceKey) (ids.LeadID, bool, error) {
	if normalizedURL == "" {
		return ids.LeadID{}, false, nil
	}
	return liveLeadBy(ctx, tx, `linkedin_url = $1`, normalizedURL, exclude, "linkedin_url")
}

// liveLeadBy is the one query shape behind both probes. The predicate is a
// fixed literal chosen by the exported wrappers, never caller text; the value
// is always a bind parameter.
func liveLeadBy(ctx context.Context, tx pgx.Tx, predicate, value string, exclude *LeadSourceKey, lane string) (ids.LeadID, bool, error) {
	var excludeSystem, excludeID *string
	if exclude != nil {
		excludeSystem, excludeID = &exclude.SourceSystem, &exclude.SourceID
	}
	var id ids.LeadID
	err := tx.QueryRow(ctx, `
		SELECT id FROM lead
		 WHERE `+predicate+` AND `+LiveLeadClause("lead")+`
		   AND ($2::text IS NULL
		        OR source_system IS DISTINCT FROM $2
		        OR source_id IS DISTINCT FROM $3)
		 ORDER BY created_at, id
		 LIMIT 1`, value, excludeSystem, excludeID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.LeadID{}, false, nil
	}
	if err != nil {
		return ids.LeadID{}, false, fmt.Errorf("probe lead %s dedupe: %w", lane, err)
	}
	return id, true, nil
}

// IdentityKeys are the exact keys that name one human on either side: live
// addresses, and LinkedIn profile URLs in the one normalised spelling
// (contacts.NormalizeContactLinkedInURL). A value in any other spelling matches
// nothing, so a handle stored as typed never counts as identity.
type IdentityKeys struct {
	Emails       []string
	LinkedInURLs []string
}

// IdentityHolders are the live leads and live contacts holding some of the
// keys asked about, each list oldest first and without repeats.
type IdentityHolders struct {
	Leads    []ids.LeadID
	Contacts []ids.ContactID
	held     map[heldKey]bool
}

// heldKey is one key a holder of one kind was found under.
type heldKey struct {
	holder, lane, key string
}

const (
	holderLead    = "lead"
	holderContact = "contact"
	laneEmail     = "email"
	laneLinkedIn  = "linkedin"
)

// LeadHoldsEmail reports whether a live lead holds this address.
func (h IdentityHolders) LeadHoldsEmail(email string) bool {
	return h.held[heldKey{holderLead, laneEmail, foldEmail(email)}]
}

// ContactHoldsEmail reports whether a live contact holds this address.
func (h IdentityHolders) ContactHoldsEmail(email string) bool {
	return h.held[heldKey{holderContact, laneEmail, foldEmail(email)}]
}

// LeadHoldsLinkedIn reports whether a live lead holds this normalised profile URL.
func (h IdentityHolders) LeadHoldsLinkedIn(url string) bool {
	return h.held[heldKey{holderLead, laneLinkedIn, url}]
}

// ContactHoldsLinkedIn reports whether a live contact holds this normalised profile URL.
func (h IdentityHolders) ContactHoldsLinkedIn(url string) bool {
	return h.held[heldKey{holderContact, laneLinkedIn, url}]
}

// LockIdentityHolders is the one question every door asks before it writes a
// lead or a contact: who already holds these keys. It takes the identity locks first, in
// the order a lead create takes them: the LinkedIn write identity, then the
// subject locks of the addresses. A caller that locks an existing contact row
// does so before this call, never after.
func LockIdentityHolders(ctx context.Context, tx pgx.Tx, keys IdentityKeys) (IdentityHolders, error) {
	urls := slices.Clone(keys.LinkedInURLs)
	slices.Sort(urls)
	for _, url := range slices.Compact(urls) {
		if url == "" {
			continue
		}
		if err := LockWriteIdentity(ctx, tx, "lead_linkedin", url); err != nil {
			return IdentityHolders{}, err
		}
	}
	if err := LockSubjectKeys(ctx, tx, nil, keys.Emails); err != nil {
		return IdentityHolders{}, err
	}
	return IdentityHoldersOf(ctx, tx, keys)
}

// identityHoldersSQL matches each key against both sides. A lead holds an
// address or a profile URL on its own row. A contact holds them on its live
// contact_email rows and its LinkedIn contact_social row.
const identityHoldersSQL = `
	WITH emails AS (SELECT DISTINCT lower(btrim(e)) AS key FROM unnest($1::text[]) e WHERE btrim(e) <> ''),
	     urls AS (SELECT DISTINCT u AS key FROM unnest($2::text[]) u WHERE u <> '')
	SELECT 'lead', 'email', l.id, l.email, l.created_at FROM lead l
	 WHERE l.email IN (SELECT key FROM emails) AND l.archived_at IS NULL
	UNION ALL
	SELECT 'lead', 'linkedin', l.id, l.linkedin_url, l.created_at FROM lead l
	 WHERE l.linkedin_url IN (SELECT key FROM urls) AND l.archived_at IS NULL
	UNION ALL
	SELECT 'contact', 'email', c.id, ce.email, c.created_at FROM contact c
	  JOIN contact_email ce ON ce.contact_id = c.id AND ce.archived_at IS NULL
	 WHERE ce.email IN (SELECT key FROM emails) AND c.archived_at IS NULL
	UNION ALL
	SELECT 'contact', 'linkedin', c.id, cs.handle, c.created_at FROM contact c
	  JOIN contact_social cs ON cs.contact_id = c.id AND cs.platform = 'linkedin'
	 WHERE cs.handle IN (SELECT key FROM urls) AND c.archived_at IS NULL
	ORDER BY 5, 3`

// IdentityHoldersOf is the same question without the locks, for a writer that
// already holds them and for a read that writes nothing.
func IdentityHoldersOf(ctx context.Context, tx pgx.Tx, keys IdentityKeys) (IdentityHolders, error) {
	out := IdentityHolders{Leads: []ids.LeadID{}, Contacts: []ids.ContactID{}, held: map[heldKey]bool{}}
	if len(keys.Emails) == 0 && len(keys.LinkedInURLs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, identityHoldersSQL, keys.Emails, keys.LinkedInURLs)
	if err != nil {
		return IdentityHolders{}, fmt.Errorf("storekit: reading who holds these identity keys: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var found heldKey
		var id ids.UUID
		var createdAt time.Time
		if err := rows.Scan(&found.holder, &found.lane, &id, &found.key, &createdAt); err != nil {
			return IdentityHolders{}, fmt.Errorf("storekit: reading who holds these identity keys: %w", err)
		}
		out.add(found, id)
	}
	if err := rows.Err(); err != nil {
		return IdentityHolders{}, fmt.Errorf("storekit: reading who holds these identity keys: %w", err)
	}
	return out, nil
}

func (h *IdentityHolders) add(found heldKey, id ids.UUID) {
	h.held[found] = true
	if found.holder == holderLead {
		if lead := ids.From[ids.LeadKind](id); !slices.Contains(h.Leads, lead) {
			h.Leads = append(h.Leads, lead)
		}
		return
	}
	if contact := ids.From[ids.ContactKind](id); !slices.Contains(h.Contacts, contact) {
		h.Contacts = append(h.Contacts, contact)
	}
}

// foldEmail is the spelling contact_email and lead_email_norm store.
func foldEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }
