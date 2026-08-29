// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ensureOrgWritable is ensureOrgReadable's write-side twin: same row-scope and
// existence hiding, plus the liveness the read deliberately does not require.
//
// The evidence of an archived company stays READABLE — that is the record of
// what was known when it was retired. Correcting it is a different act, and
// PATCH /organizations/{id} refuses it. Both evidence writers gate here so the
// refusal does not depend on whether the corrected field happens to have a
// canonical column behind it.
func ensureOrgWritable(ctx context.Context, tx pgx.Tx, id ids.OrganizationID) error {
	return auth.EnsureWritableLive(ctx, tx, "organization", id.UUID)
}

// canonicalOrgColumn maps a profile field onto the organization column that
// actually holds the value (ADR-0085 / A130). Where a mapping exists, THE
// COLUMN IS THE VALUE and the sidecar row is only its receipt — so a
// correction has to move both, or the header keeps showing what the user just
// corrected and the write is a lie about having been accepted (PO-AC-N-1).
//
// registered_address is deliberately absent: it is the registry address, a
// different fact from the operating address in the six address columns, and
// collapsing them was never intended. Everything else in the vocabulary has no
// column at all and lives only in the sidecar.
func canonicalOrgColumn(field string) (string, bool) {
	switch field {
	case fieldDisplayName:
		return fieldDisplayName, true
	case fieldLegalName:
		return fieldLegalName, true
	case fieldIndustry:
		return fieldIndustry, true
	default:
		return "", false
	}
}

// Audit-image keys the two evidence sidecars share. A correction's before image
// is the machine's whole claim, so both sidecars write the same shape and each
// key is spelled once. auditKeySource, auditKeySourceURL and companySourceHuman
// already exist in company.go and are reused rather than respelled here.
const (
	auditKeyValue           = "value"
	auditKeyEvidenceSnippet = "evidence_snippet"
	auditKeyConfidence      = "confidence"
	auditKeyVerifiedAt      = "verified_at"
	auditKeyVerifiedBy      = "verified_by"
)

// evidenceRow is one sidecar row's before-image, and it is ONE type because a
// profile field and a fact carry the same claim: a value, who stands behind it,
// what the machine read to propose it, and whether a human has since agreed.
//
// They were two identical structs with two identical auditImage methods. The
// duplication was not merely repetitive — it let the two audit trails drift, and
// an audit trail that records a correction differently depending on which
// sidecar it touched is one nobody can query.
type evidenceRow struct {
	ID              ids.UUID
	Value           string
	Source          string
	EvidenceSnippet *string
	SourceURL       *string
	Confidence      *float32
	VerifiedAt      *time.Time
	VerifiedBy      *ids.UUID
	// CapturedBy is WHO the row currently belongs to, and it is what the
	// enrichment upserts test (`captured_by NOT LIKE 'human:%'`) before they
	// overwrite a claim. A human verdict that moved `source` alone left this
	// naming the machine, so the next ordinary refresh reclaimed the row.
	CapturedBy string
}

// auditImage is the machine's whole claim, which is what a correction's BEFORE
// image has to be: the snippet, the source and the confidence survive the
// human's answer, so "what did it say before I fixed it" stays answerable
// (PO-AC-N-2).
func (r evidenceRow) auditImage() map[string]any {
	return map[string]any{
		auditKeyValue: r.Value, auditKeySource: r.Source,
		auditKeyEvidenceSnippet: r.EvidenceSnippet, auditKeySourceURL: r.SourceURL,
		auditKeyConfidence: r.Confidence,
		auditKeyVerifiedAt: r.VerifiedAt, auditKeyVerifiedBy: r.VerifiedBy,
		auditKeyCapturedBy: r.CapturedBy,
	}
}

// ProfileFieldWriteInput carries a correction. Value is nil for a confirmation,
// which is the same act without a value change (PO-AC-N-3).
type ProfileFieldWriteInput struct {
	Value     *string
	IfVersion *int64
}

// UpdateOrganizationProfileField corrects a profile field's value.
func (s *Store) UpdateOrganizationProfileField(
	ctx context.Context, orgID ids.OrganizationID, field string, in ProfileFieldWriteInput,
) (crmcontracts.CompanyProfileField, error) {
	if in.Value == nil {
		return crmcontracts.CompanyProfileField{}, fmt.Errorf(
			"a correction carries a value; use the confirm operation to agree without changing one: %w",
			apperrors.ErrConflict)
	}
	return s.writeProfileField(ctx, orgID, field, in)
}

// ConfirmOrganizationProfileField records that a human read the claim and
// agreed. No value moves; the field goes from extracted to confirmed.
func (s *Store) ConfirmOrganizationProfileField(
	ctx context.Context, orgID ids.OrganizationID, field string, in ProfileFieldWriteInput,
) (crmcontracts.CompanyProfileField, error) {
	in.Value = nil
	return s.writeProfileField(ctx, orgID, field, in)
}

// writeProfileField is the one path both verbs take, so the provenance flip,
// the canonical-column write, the audit image and the event cannot diverge
// between correcting and confirming.
//
// Held by: TestBothProfileFieldVerbsWriteThroughTheOnePath (backend/internal/modules/people/profilefieldonepath_test.go)
func (s *Store) writeProfileField(
	ctx context.Context, orgID ids.OrganizationID, field string, in ProfileFieldWriteInput,
) (crmcontracts.CompanyProfileField, error) {
	w := evidenceWrite[crmcontracts.CompanyProfileField]{
		table:      "organization_profile_field",
		archived:   storekit.NoArchiveColumn,
		changedKey: field,
		value:      in.Value,
		ifVersion:  in.IfVersion,
		readBefore: func(ctx context.Context, tx pgx.Tx) (evidenceRow, error) {
			return readProfileFieldRow(ctx, tx, orgID, field)
		},
		readAfter: func(ctx context.Context, tx pgx.Tx) (crmcontracts.CompanyProfileField, error) {
			return readProfileFieldWire(ctx, tx, orgID, field)
		},
	}
	// The half that makes the correction real: a field with a column on the
	// organization moves there too, or the header goes on showing the value the
	// human just corrected.
	if column, canonical := canonicalOrgColumn(field); canonical && in.Value != nil {
		w.canonical = func(ctx context.Context, tx pgx.Tx) error {
			return writeCanonicalOrgColumn(ctx, tx, orgID, column, *in.Value)
		}
	}
	return writeEvidence(ctx, s, orgID, w)
}

// writeCanonicalOrgColumn moves the value the sidecar was only describing.
//
// A correction here IS an organization edit, so it owes the record everything
// UpdateOrganization owes it — the same four obligations, or a correction
// becomes a back door that reaches the same columns with fewer rules:
//
//   - LIVE ONLY. PATCH /organizations/{id} refuses an archived record; without
//     the same filter, correcting its display_name through the receipt would
//     rewrite a record the ordinary path says is gone.
//   - THE VERSION MOVES, so another editor holding the organization's If-Match
//     is told the row changed under them. trg_organization_updated does that
//     for any UPDATE on the row, which is why nothing here sets it.
//   - name_source = 'human', FOR A DISPLAY-NAME CORRECTION ONLY. A human naming
//     the company is the top of the provenance lattice (ADR-0072/A118), and
//     left unstamped the next enrichment run overwrites the correction as
//     though no one had made it. The column describes display_name's
//     provenance and nothing else, so a legal-name correction must not stamp
//     it: that would claim a human authored a display name nobody touched, and
//     PromoteOrgNameTx (which promotes only while it still reads 'domain')
//     would refuse that company its real name forever. The rule is not local
//     to this writer: UpdateOrganization stamps 'human' only when display_name
//     actually moved, and the cold-start create declines the value outright,
//     reserving it in its own comment for a human's naming.
//   - THE RENAME RECHECK RUNS. A new name can collide with an existing company,
//     and the duplicate queue only learns about it if this asks. Either name
//     can collide, so either name pays for it.
func writeCanonicalOrgColumn(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, column, value string,
) error {
	// Two questions, and they are not one: which corrections write a NAME (both
	// of them, and each owes the lock and the re-check), and which correction
	// authors the name whose provenance name_source records (display_name
	// alone). One flag cannot answer both without stamping a provenance onto a
	// display name nobody touched.
	nameWrite := column == fieldDisplayName || column == fieldLegalName
	authoredDisplayName := column == fieldDisplayName
	if nameWrite {
		// Workspace-wide, and taken before the row write for the ordering rule
		// on lockOrgNameWrites. Only a rename pays for it.
		if err := lockOrgNameWrites(ctx, tx); err != nil {
			return err
		}
	}
	// The column name comes from canonicalOrgColumn's closed switch, never from
	// caller input, so the interpolation cannot carry anything a request chose.
	// name_source rides the same statement rather than a second UPDATE: one
	// write, so the value and its provenance cannot land apart.
	nameSource := ""
	if authoredDisplayName {
		// Only when the name MOVES. Every right-hand side in an UPDATE sees the
		// old row, so this compares what is stored against what is being
		// written — no read, still one statement.
		//
		// Re-sending the same name is not authoring it, and 'human' is a door
		// that does not reopen: once the lattice reads human-authored no
		// automated source may correct the name again. A write that merely
		// echoed the name it read — an agent round-tripping a record, a form
		// resaving an untouched field — froze a provisional domain-derived name
		// for good, with nothing in the record saying a person never chose it.
		nameSource = `, name_source = CASE WHEN display_name IS DISTINCT FROM $2
		                                   THEN 'human' ELSE name_source END`
	}
	tag, err := tx.Exec(ctx,
		`UPDATE organization SET `+column+` = $2`+nameSource+`
		  WHERE id = $1 AND archived_at IS NULL`, orgID, value)
	if err != nil {
		return fmt.Errorf("write canonical organization column: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	if !nameWrite {
		return nil
	}
	editor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return recheckOrgNameForDuplicates(ctx, tx, orgID, editor)
}

func readProfileFieldRow(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, field string,
) (evidenceRow, error) {
	var r evidenceRow
	err := tx.QueryRow(ctx, `
		SELECT id, value, source, evidence_snippet, source_url, confidence, verified_at, verified_by, captured_by
		  FROM organization_profile_field
		 WHERE organization_id = $1 AND field = $2`,
		orgID, field,
	).Scan(&r.ID, &r.Value, &r.Source, &r.EvidenceSnippet, &r.SourceURL, &r.Confidence,
		&r.VerifiedAt, &r.VerifiedBy, &r.CapturedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperrors.ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("read organization profile field: %w", err)
	}
	return r, nil
}

func readProfileFieldWire(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, field string,
) (crmcontracts.CompanyProfileField, error) {
	var (
		pf           crmcontracts.CompanyProfileField
		rowID        ids.UUID
		fieldV, srcV string
	)
	err := tx.QueryRow(ctx, `
		SELECT id, field, value, source, captured_by, evidence_snippet, source_url, confidence,
		       retrieved_at, verified_at, verified_by, updated_at
		  FROM organization_profile_field
		 WHERE organization_id = $1 AND field = $2`,
		orgID, field,
	).Scan(&rowID, &fieldV, &pf.Value, &srcV, &pf.CapturedBy, &pf.EvidenceSnippet, &pf.SourceUrl,
		&pf.Confidence, &pf.RetrievedAt, &pf.VerifiedAt, &pf.VerifiedBy, &pf.UpdatedAt)
	if err != nil {
		return pf, fmt.Errorf("re-read organization profile field: %w", err)
	}
	// The row's own identity, on the write path as on the read path: a client
	// that just corrected a value holds the record a later sentence will cite,
	// and a response without it forces a second list read to find the id.
	id := openapi_types.UUID(rowID)
	pf.Id = &id
	pf.Field = crmcontracts.CompanyProfileFieldField(fieldV)
	pf.Source = crmcontracts.CompanyProfileFieldSource(srcV)
	return pf, nil
}
