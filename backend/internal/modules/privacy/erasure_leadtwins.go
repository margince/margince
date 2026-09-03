// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The lead half of an Art. 17 erasure: the segregated lead rows a subject was
// captured as before anybody promoted them, and the per-lead records that hang
// off those rows. Its own file beside erasure_attachments.go and
// erasure_consent.go, because the package splits an erasure by the kind of
// thing being destroyed and a lead twin is a kind of its own.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The communication arms are here rather than in deleteConsentCapabilities for
// the reason the four scrubs above them are: this is an ANONYMIZE, so the lead
// row survives and no ON DELETE CASCADE fires. A decision written while the
// subject was still a lead carries subject_kind='lead', which a person-keyed
// statement cannot see — it would keep a live address and a lead id that still
// points at the erased person through promoted_person_id.
//
// anonymizeLeadTwins wipes the subject's segregated lead rows and everything
// keyed to them, answering with the twins it touched. One CTE on purpose:
// the UPDATE runs first and feeds the touched ids to every DELETE, so the
// email match still sees the pre-anonymize addresses; split into separate
// statements, the second would match nothing.
//
// The dependents are not incidental. Field provenance says WHO captured
// WHICH field from WHERE. The correction ledger holds human verdicts about
// the twin. The score history embeds the ids of activities the subject took
// part in, inside JSON no field-level scrub reaches. A manual scoring signal
// carries a colleague's name and their written judgement about them. This is
// an ANONYMIZE, so the lead row survives and nothing cascades — each has to
// be named here or it outlives the erasure (ADR-0105).
func anonymizeLeadTwins(ctx context.Context, tx pgx.Tx, personID ids.PersonID, emails []string) ([]ids.UUID, error) {
	leadCustom, err := subjectCustomColumns(ctx, tx, "lead")
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		WITH wiped AS (
		  UPDATE lead SET full_name = 'Anonymized Lead', email = NULL, title = NULL,
		    company_name = NULL, candidate_org_key = NULL, raw = NULL,
		    archived_at = coalesce(archived_at, now())%s
		  WHERE promoted_person_id = $1
		     OR id IN (SELECT converted_from_lead_id FROM person WHERE id = $1 AND converted_from_lead_id IS NOT NULL)
		     OR (email IS NOT NULL AND lower(email) = ANY($2))
		  RETURNING id
		), pruned AS (
		  DELETE FROM field_provenance
		  WHERE object_type = 'lead' AND object_id IN (SELECT id FROM wiped)
		), verdicts AS (
		  DELETE FROM ai_feedback
		  WHERE subject_type = 'lead' AND subject_id IN (SELECT id FROM wiped)
		), scores AS (
		  DELETE FROM lead_score_history
		  WHERE lead_id IN (SELECT id FROM wiped)
		), manual AS (
		  DELETE FROM lead_manual_signal
		  WHERE lead_id IN (SELECT id FROM wiped)
		), leadbases AS (
		  DELETE FROM communication_basis
		  WHERE lead_id IN (SELECT id FROM wiped)
		), leadsuppressions AS (
		  DELETE FROM communication_suppression
		  WHERE lead_id IN (SELECT id FROM wiped)
		), leaddecisions AS (
		  UPDATE communication_decision
		     SET recipient_address = 'erased+' || id || '@example.invalid',
		         subject_id = NULL, subject_kind = NULL
		   WHERE subject_kind = 'lead' AND subject_id IN (SELECT id FROM wiped)
		)
		SELECT id FROM wiped`, nullColumnAssignments(leadCustom)),
		personID, lowercased(emails))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
