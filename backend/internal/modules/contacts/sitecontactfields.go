// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Auto-filling a site contact onto a contact the workspace ALREADY has
// (ADR-0072/A118 phase 4B).
//
// A deep read of a company's team page publishes contacts. A stranger among them
// stages as a lead and stays staged (ADR-0008, NEVER-8) — that boundary does not
// move. But when the published contact is unmistakably someone the workspace
// already records at that company, staging a lead offers a human a duplicate of
// a record they already have, and the role the site prints next to their name
// goes unused.
//
// "Unmistakably" is deliberately narrow, and it is the whole safety argument:
//
//   - an exact live email match among that company's own employees, or
//   - exactly ONE employee of that company whose name matches confidently.
//
// Zero matches, or more than one, means the site contact is not identifiable and
// the lead stages exactly as before. The scope is the company's employees
// rather than the workspace, because the site is claiming this contact works
// THERE: filling a title from company X's site onto a contact the CRM records at
// company Y is a conflict a human should see, not one a sweep should settle.
//
// Everything written is fill-only-empty and evidence-backed, so a human's answer
// is structurally untouchable and a re-read applies nothing twice.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// siteFieldSource is the DM-CONV-11 channel for site-read contact fields.
const siteFieldSource = "site_read"

// siteConfidentNameMatch is how close a published name must be to an employee's
// before the two are treated as the same contact. Above the dedupe REVIEW
// threshold on purpose: 0.72 is where a human is asked to compare two records,
// and this path asks nobody.
const siteConfidentNameMatch = 0.92

// SiteContactFields is one published contact as the site printed them, and the
// page that printed it.
type SiteContactFields struct {
	Name            string
	Role            string
	PublishedEmail  string
	LinkedinURL     string
	EvidenceSnippet string
	SourceURL       string
}

// ApplySiteContactFields fills a matched employee's empty fields from what the
// company's own site publishes about them. It reports whether a contact was
// matched at all — false means the caller stages the lead, which is the
// unchanged path for every stranger and every ambiguous name.
func (s *Store) ApplySiteContactFields(ctx context.Context, companyID ids.CompanyID, in SiteContactFields) (bool, error) {
	var matched bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		matched, err = s.applySiteContactFieldsTx(ctx, tx, companyID, in)
		return err
	})
	if err != nil {
		return false, err
	}
	return matched, nil
}

func (s *Store) applySiteContactFieldsTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, in SiteContactFields) (bool, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return false, errors.New("contacts: a site contact needs the name the page published")
	}
	// The company is a KNOWN row and this is a read of it: row-scope is
	// re-checked so a leaked company id buys nothing (existence-hiding 404).
	if err := auth.EnsureVisible(ctx, tx, entityCompany, companyID.UUID); err != nil {
		return false, err
	}

	contactID, ok, err := matchSiteContact(ctx, tx, companyID, in)
	if err != nil || !ok {
		return false, err
	}
	// The contact is resolved from the company's employment edges, so the
	// probe above says nothing about it: the company gate is a gate on a DIFFERENT
	// table. Probe the record this function is about to write, the way every
	// sibling fill does (ApplyDiscoveredFields, SaveResearchClaims,
	// ApplyEnrichment, ApplyDeepReadTx).
	//
	// Live, not the plain spelling: EnsureWritable returns nil the moment the
	// rendered scope clause is empty, which is exactly what today's two callers
	// — both PrincipalSystem — produce. The plain probe would therefore be a
	// no-op on every live call site and this would read as gated while gating
	// nothing. The Live spelling always runs the existence and archived_at
	// query, so at minimum a matched contact that has since been archived stops
	// being written.
	//
	// SKIP rather than refuse. matchSiteContact's contract is already
	// "not identifiable here → stage a lead instead", and a match the caller
	// may not write is exactly that case: refusing would abort a whole
	// company's site confirmation over one out-of-scope employee, while
	// skipping leaves the lead to stage and the rest of the page to land.
	if err := auth.EnsureWritableLive(ctx, tx, entityContact, contactID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return false, fmt.Errorf("contacts: probing write authority over the site contact: %w", err)
	}

	sourceRef := siteFieldSource + ":" + in.SourceURL
	applied, previous, values, err := fillSiteContactFields(ctx, tx, contactID, sourceRef, by, siteFieldSource, in)
	if err != nil {
		return false, err
	}
	if len(applied) == 0 {
		// Matched, but the site said nothing this contact was missing. Reported
		// as matched all the same: the lead must not stage, because the contact
		// is not a stranger and a duplicate is not an improvement.
		return true, nil
	}

	// The images carry the fields the page filled and what each held before,
	// which for this writer is nothing: every fill here is guarded — the
	// evidence rows by ON CONFLICT DO NOTHING, the title column by IS NULL — so
	// a field that landed had no prior value. WHICH page said so is context
	// about the mutation and rides evidence, because a source folded into the
	// after-image projects as a change to a field of that name.
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionUpdate, entityContact, contactID.UUID,
		previous, values,
		map[string]any{auditKeySource: siteFieldSource, auditKeySourceRef: sourceRef})
	if err != nil {
		return false, fmt.Errorf("contacts: auditing the site contact fill: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, contactID.UUID, crmcontracts.PublicEventContactUpdated{
		ChangedFields: map[string]any{auditKeyFields: applied, auditKeySource: siteFieldSource},
	}); err != nil {
		return false, fmt.Errorf("contacts: emitting contact.updated for the site fill: %w", err)
	}
	return true, nil
}

// matchSiteContact resolves the published contact to at most ONE employee of the
// company: exact live email first, then a confident name match that must be
// unique. Ambiguity is not a tie to break — it is the answer "not identifiable",
// and it stages a lead like any stranger.
func matchSiteContact(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, in SiteContactFields) (ids.ContactID, bool, error) {
	if email := strings.ToLower(strings.TrimSpace(in.PublishedEmail)); email != "" {
		var id ids.ContactID
		err := tx.QueryRow(ctx, `
			SELECT p.id
			  FROM contact p
			  JOIN contact_email pe ON pe.contact_id = p.id AND pe.email = $2 AND pe.archived_at IS NULL
			  JOIN relationship r ON r.contact_id = p.id AND r.company_id = $1
			   AND r.kind = 'employment' AND r.archived_at IS NULL
			 WHERE p.archived_at IS NULL AND p.merged_into_id IS NULL
			 LIMIT 1`, companyID, email).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return ids.ContactID{}, false, fmt.Errorf("contacts: matching a site contact by email: %w", err)
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT p.id, p.full_name
		  FROM contact p
		  JOIN relationship r ON r.contact_id = p.id AND r.company_id = $1
		   AND r.kind = 'employment' AND r.archived_at IS NULL
		 WHERE p.archived_at IS NULL AND p.merged_into_id IS NULL`, companyID)
	if err != nil {
		return ids.ContactID{}, false, fmt.Errorf("contacts: reading the company's employees: %w", err)
	}
	defer rows.Close()
	var match ids.ContactID
	found := 0
	for rows.Next() {
		var id ids.ContactID
		var fullName string
		if err := rows.Scan(&id, &fullName); err != nil {
			return ids.ContactID{}, false, err
		}
		if nameSimilarity(in.Name, fullName) >= siteConfidentNameMatch {
			match, found = id, found+1
		}
	}
	if err := rows.Err(); err != nil {
		return ids.ContactID{}, false, err
	}
	if found != 1 {
		return ids.ContactID{}, false, nil
	}
	return match, true, nil
}

// fillSiteContactFields writes what the page published into the fields this
// contact has not answered yet, and returns what actually landed.
//
// The published EMAIL is deliberately never written. It is a matching key here,
// not a fill: adding an address to an existing contact changes who that record
// is reachable as, and the site is not authority for that.
// It answers the fields it wrote, in the order it wrote them, and the audit
// images those fields carry — an explicit null per field before, the written
// value after. Field history projects per field from those, so neither can be a
// list of names.
// fillSiteContactFields fills the fields a published page states, and only where
// the record is empty.
//
// Fill-only-empty here while the signature and card paths replace by recency,
// and the difference is the SOURCE rather than an inconsistency: a page is
// somebody else's description of this contact, published who-knows-when and
// carrying no date of its own, so it has no standing to overwrite an answer
// already on the record. A signature and a card are the contact saying so
// themselves, on a date (observedcontact.go).
func fillSiteContactFields(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, sourceRef, by, source string, in SiteContactFields) ([]string, map[string]any, map[string]any, error) {
	var applied []string
	previous, values := map[string]any{}, map[string]any{}
	write := func(field, value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		// A machine fill: what a page published claims a field nobody has
		// answered and never replaces one.
		landed, err := writeContactProfileField(ctx, tx, contactID, contactProfileFieldRow{
			Field: field, Value: value, EvidenceSnippet: in.EvidenceSnippet, SourceRef: sourceRef,
			Source: source, CapturedBy: by,
		}, claimUnanswered)
		if err != nil {
			return err
		}
		if !landed {
			return nil
		}
		if err := storekit.StampFields(ctx, tx, entityContact, contactID.UUID, sourceRef, by,
			[]storekit.FieldStamp{{Field: field}}); err != nil {
			return err
		}
		applied = append(applied, field)
		previous[field], values[field] = nil, value
		return nil
	}

	if err := write("role", in.Role); err != nil {
		return nil, nil, nil, err
	}
	if err := write("linkedin", in.LinkedinURL); err != nil {
		return nil, nil, nil, err
	}
	// The title column carries the role for display. Fill-only-empty: the NULL
	// predicate is the CAS, so an occupied title stands whoever set it — and
	// archived_at, so this half refuses exactly when the evidence row above
	// does. Without it an erasure committing between the two statements leaves
	// the evidence refused and the erased contact's title written back.
	if role := strings.TrimSpace(in.Role); role != "" {
		tag, err := tx.Exec(ctx, `
			UPDATE contact SET title = $2 WHERE id = $1 AND title IS NULL AND archived_at IS NULL`, contactID, role)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("contacts: site contact title fill: %w", err)
		}
		if tag.RowsAffected() > 0 {
			applied = append(applied, "title")
			previous["title"], values["title"] = nil, role
		}
	}
	return applied, previous, values, nil
}
