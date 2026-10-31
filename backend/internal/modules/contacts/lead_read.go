// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The lead read spine: the one column list every lead read shares, the
// single-row read, and the scanner. Split out of lead.go so each file
// stays one concept under the 500-LOC cap.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// liveOnlyClause narrows a read to unarchived rows, spelled once.
const liveOnlyClause = ` AND archived_at IS NULL`

// The trailing subqueries derive the working-surface facts (ADR-0118/A169)
// from activity_link rather than storing them on the lead: the last touch,
// how many tasks are still open against it, and the strongest engagement
// signal. Every lead read is FROM the unaliased table, which is what lets
// them name lead.id. A restricted activity (A165: held under a statutory
// retention obligation) is in none of them, the same as in every other read.
// leadSourceLabelSQL is the source in the words the catalog gives it, which is
// what the Source column prints. The stored key is an identifier; a reader
// sorting the column means the label beside it.
const leadSourceLabelSQL = `(SELECT s.label FROM lead_source s WHERE s.key = lead.source)`

// leadNextTaskDueSQL is when the next open task a reader may open falls due —
// the deadline the Next task column prints, taking the audience for the reason
// the pair beside it does.
func leadTaskWorkSQL(ctx context.Context, arg func(any) int) (string, error) {
	identities, err := leadTaskIdentitiesSQL(ctx, arg)
	if err != nil {
		return "", err
	}
	return " AND " + auth.LiveIntakeTaskClause("a") + " AND " + storekit.CurrentTaskClause("a", slaNow(ctx), identities, arg), nil
}

func leadNextTaskDueSQL(work string) string {
	return `(SELECT ` + storekit.TaskDueSQL("a") + ` FROM activity_link l JOIN activity a ON a.id = l.activity_id
	   WHERE l.lead_id = lead.id AND a.archived_at IS NULL AND a.restricted_at IS NULL AND a.kind = 'task' AND NOT a.is_done ` + work + `
	     ` + auth.AudienceWorkspaceOnly("a") + `
	   ORDER BY ` + storekit.TaskDueSQL("a") + ` NULLS LAST, a.created_at, a.id LIMIT 1)`
}

// leadLastActivitySQL is the last-touch clock. Like last_activity_of_contact, it
// leaves out system remediation, called-off meetings and what has not happened
// yet: none of it is the lead engaging.
//
// The row that PRINTS it and the expression that ORDERS BY it read this, so a
// reader sees the instant the list was arranged by rather than a second reading
// of the same question.
func leadLastActivitySQL() string {
	return `(SELECT max(a.occurred_at) ` + leadTouchesSQL() + `)`
}

// leadLastActivityPrecisionSQL is how to read leadLastActivitySQL's instant,
// from the activity it names.
func leadLastActivityPrecisionSQL() string {
	return `(SELECT ` + occurredday.LatestPrecisionSQL("a", "") + ` ` + leadTouchesSQL() + `)`
}

// leadTouchesSQL is the activities the last-touch clock reads.
func leadTouchesSQL() string {
	return `FROM activity_link l JOIN activity a ON a.id = l.activity_id
	   WHERE l.lead_id = lead.id AND a.archived_at IS NULL AND a.restricted_at IS NULL
	     AND ` + relstrength.LastActivityCountsSQL("a", "now()") + auth.OriginIsEngagement("a")
}

func leadColumns(ctx context.Context, arg func(any) int) (string, error) {
	work, err := leadTaskWorkSQL(ctx, arg)
	if err != nil {
		return "", err
	}
	return `id, full_name, email, phone, title, company_name, candidate_company_key,
	linkedin_url, status, score, score_override_reason, score_computed, owner_id, project_id, source_system, source_id,
	promoted_contact_id, promoted_at, merged_into_id, from_contact_id, source, captured_by,
	source_author_id, source_author_name,
	` + sourceAuthorSeatNameSQL("lead") + `,
	version, created_at, updated_at, archived_at,
	routed_at, first_response_at,
	` + leadSourceLabelSQL + `,
	origin, origin_detail, ` + leadOriginLabelSQL + `,
	disqualify_reason_id, disqualify_note,
	(SELECT r.label FROM lead_disqualify_reason r WHERE r.id = lead.disqualify_reason_id),
	status_set_by, qualified_deal_id,
	` + leadQualificationEvidenceSQL() + `,
	` + leadLastActivitySQL() + `, ` + leadLastActivityPrecisionSQL() + `,
	(SELECT count(*) FROM activity_link l JOIN activity a ON a.id = l.activity_id
	   WHERE l.lead_id = lead.id AND a.archived_at IS NULL AND a.restricted_at IS NULL AND a.kind = 'task' AND NOT a.is_done ` + work + `),
	-- The next open task, as the pair it is: its title and its deadline
	-- describe ONE task, so they select one row. The subject is content and
	-- the due date is a marker, but splitting them by that distinction would
	-- let the two subqueries land on different tasks — a limited task skipped
	-- by one and taken by the other — and the card would show one task's title
	-- beside another's deadline, which is worse than showing neither.
	--
	-- So both take the audience. A limited task is not the "next task" for a
	-- lead card every seat reads; the card names the next one its reader may
	-- actually open. The count and the max beside them are aggregate markers
	-- over all tasks and owe nothing.
	(SELECT a.subject FROM activity_link l JOIN activity a ON a.id = l.activity_id
	   WHERE l.lead_id = lead.id AND a.archived_at IS NULL AND a.restricted_at IS NULL AND a.kind = 'task' AND NOT a.is_done ` + work + `
	     ` + auth.AudienceWorkspaceOnly("a") + `
	   ORDER BY ` + storekit.TaskDueSQL("a") + ` NULLS LAST, a.created_at, a.id LIMIT 1),
	` + leadNextTaskDueSQL(work) + `,
	(SELECT factor.value->>'factor'
	   FROM LATERAL (
	     SELECT factors FROM lead_score_history
	      WHERE lead_id = lead.id
	      ORDER BY computed_at DESC, id DESC LIMIT 1
	   ) history
	   CROSS JOIN LATERAL jsonb_array_elements(
	     CASE WHEN jsonb_typeof(history.factors) = 'array' THEN history.factors ELSE '[]'::jsonb END
	   ) WITH ORDINALITY factor(value, position)
	  WHERE jsonb_typeof(factor.value->'factor') = 'string'
	    AND jsonb_typeof(factor.value->'points') = 'number'
	  ORDER BY abs(CASE WHEN jsonb_typeof(factor.value->'points') = 'number'
	                    THEN (factor.value->>'points')::numeric END) DESC,
	           factor.position
	  LIMIT 1),
	legal_hold, ` + leadHasInboundSQL + `, company_id, ` + leadIndustryField + `, ` + leadCompanyFactsSQL(ctx) + `,
	partner_company_id, partner_accepted_at, partner_accepted_by`, nil
}

// readLead resolves one lead row; active names the custom-field columns
// to carry alongside the core ones — nil for internal decision reads whose
// result never reaches the wire.
func readLead(ctx context.Context, tx pgx.Tx, id ids.LeadID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Lead, error) {
	var err error
	ctx, err = withLeadTaskIdentities(ctx, tx)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	args := []any{id}
	arg := func(value any) int { args = append(args, value); return len(args) }
	columns, err := leadColumns(ctx, arg)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	q := `SELECT ` + columns + storekit.SelectSuffix(active) + ` FROM lead WHERE id = $1`
	if archived == storekit.LiveOnly {
		q += liveOnlyClause
	}
	policy, err := loadLeadSLAPolicy(ctx, tx, config.Bound())
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	l, err := scanLead(tx.QueryRow(ctx, q, args...), active, policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.Lead{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := stampLeadWritable(ctx, tx, &l); err != nil {
		return crmcontracts.Lead{}, err
	}
	one := []crmcontracts.Lead{l}
	if err := withholdUnreadableLinks(ctx, tx, one); err != nil {
		return crmcontracts.Lead{}, err
	}
	return one[0], nil
}

// withholdUnreadableLinks blanks each record id on a lead its reader cannot
// open. Those are the contact it was worked from, its linked company and the
// partner that referred it.
func withholdUnreadableLinks(ctx context.Context, tx pgx.Tx, leads []crmcontracts.Lead) error {
	if err := withholdUnreadableSourceContacts(ctx, tx, leads); err != nil {
		return err
	}
	if err := withholdUnreadablePartners(ctx, tx, leads); err != nil {
		return err
	}
	return withholdUnreadableCompanies(ctx, tx, leads)
}

// withholdUnreadableSourceContacts blanks from_contact_id on every lead whose
// contact the caller cannot open, so reading a lead never hands over the id of
// a contact outside the reader's sight. One probe for the whole page.
func withholdUnreadableSourceContacts(ctx context.Context, tx pgx.Tx, leads []crmcontracts.Lead) error {
	var contactIDs []ids.UUID
	for _, l := range leads {
		if l.FromContactId != nil {
			contactIDs = append(contactIDs, ids.UUID(*l.FromContactId))
		}
	}
	if len(contactIDs) == 0 {
		return nil
	}
	visible, err := auth.VisibleSubset(ctx, tx, "contact", contactIDs)
	if err != nil {
		return err
	}
	for i := range leads {
		if leads[i].FromContactId != nil && !visible[ids.UUID(*leads[i].FromContactId)] {
			leads[i].FromContactId = nil
		}
	}
	return nil
}

// scanLead scans core + active custom columns, plus whatever trailing
// expressions the caller selected — the list page appends the sort's
// __cursor_key there, exactly as scanContact does.
func scanLead(row pgx.Row, active []fieldcatalog.Column, policy leadSLAPolicy, extra ...any) (crmcontracts.Lead, error) {
	var l crmcontracts.Lead
	var id ids.UUID
	var ownerID, projectID, promotedContact, mergedInto, fromContact, disqualifyReason, qualifiedDeal, companyID, partnerCompany *ids.UUID
	var statusSetBy *string
	var evidence []byte
	var email *string
	var status string
	var version int64
	var openTasks int
	var lastActivityPrecision *string
	var hasInbound bool
	var authorName, authorSeatName *string
	var authorID *ids.UUID
	var companyFacts []byte

	dests := []any{
		&id, &l.FullName, &email, &l.Phone, &l.Title, &l.CompanyName, &l.CandidateCompanyKey,
		&l.LinkedinUrl, &status, &l.Score, &l.ScoreOverrideReason, &l.ScoreComputed, &ownerID, &projectID, &l.SourceSystem, &l.SourceId,
		&promotedContact, &l.PromotedAt, &mergedInto, &fromContact, &l.Source, &l.CapturedBy,
		&authorID, &authorName, &authorSeatName,
		&version, &l.CreatedAt, &l.UpdatedAt, &l.ArchivedAt,
		&l.RoutedAt, &l.FirstResponseAt, &l.SourceLabel, &l.Origin, &l.OriginDetail, &l.OriginLabel, &disqualifyReason, &l.DisqualifyNote, &l.DisqualifyReason,
		&statusSetBy, &qualifiedDeal, &evidence,
		&l.LastActivityAt, &lastActivityPrecision, &openTasks,
		&l.NextTaskSubject, &l.NextTaskDueAt, &l.ScoreReason, &l.LegalHold, &hasInbound, &companyID,
		&l.Industry, &companyFacts, &partnerCompany, &l.PartnerAcceptedAt, &l.PartnerAcceptedBy,
	}
	cf := storekit.ScanDests(active)
	if err := row.Scan(append(append(dests, cf...), extra...)...); err != nil {
		return l, err
	}
	if values := storekit.ExtractValues(active, cf); len(values) > 0 {
		l.AdditionalProperties = values
	}

	l.Id = openapi_types.UUID(id)
	l.OwnerId = uuidPtr(ownerID)
	l.ProjectId = uuidPtr(projectID)
	l.PromotedContactId = uuidPtr(promotedContact)
	// What separates a merged-away lead from a disqualified one. Both are
	// archived and neither carries a promoted_contact_id, so without this the
	// page can only see that the lead ended — and it said "Disqualified",
	// which claims a human judged the lead not worth pursuing.
	l.MergedIntoId = uuidPtr(mergedInto)
	l.FromContactId = uuidPtr(fromContact)
	l.CompanyId = uuidPtr(companyID)
	l.PartnerCompanyId = uuidPtr(partnerCompany)
	if err := scanLeadCompanyFacts(&l, companyFacts); err != nil {
		return l, err
	}
	l.DisqualifyReasonId = uuidPtr(disqualifyReason)
	l.QualifiedDealId = uuidPtr(qualifiedDeal)
	if statusSetBy != nil {
		setBy := crmcontracts.LeadStatusSetBy(*statusSetBy)
		l.StatusSetBy = &setBy
	}
	if len(evidence) > 0 {
		var ev crmcontracts.LeadQualificationEvidence
		if err := json.Unmarshal(evidence, &ev); err != nil {
			return l, fmt.Errorf("decode qualification evidence for lead %s: %w", l.Id, err)
		}
		l.QualificationEvidence = &ev
	}
	if email != nil {
		e := openapi_types.Email(*email)
		l.Email = &e
	}
	l.Status = crmcontracts.LeadStatus(status)
	l.Version = &version
	l.OpenTaskCount = &openTasks
	if lastActivityPrecision != nil {
		l.LastActivityPrecision = new(crmcontracts.OccurredPrecision(*lastActivityPrecision))
	}
	if !hasInbound {
		policy.enabled = false
	}
	l.SlaDeadlineAt, l.SlaState = leadSLAFields(policy, l.RoutedAt, l.CreatedAt, l.FirstResponseAt, l.ArchivedAt)
	l.Author = sourceAuthorOf(authorID, authorSeatName, authorName, l.SourceSystem)
	l.ImportedFrom = provenance.ImportedFrom(l.SourceSystem)
	return l, nil
}
