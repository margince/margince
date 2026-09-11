// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The accepted enrichment of a KNOWN company (EP05 / scrapeCompany).
// Unlike the cold-start read-back it never resolves-by-domain or creates — it
// fills the company the proposal named, and only its empty fields, with the
// executing principal's provenance (agent:scrape). The per-field apply loop is
// shared with the read-back (applyEvidenceFields) so both write website
// evidence under the same site_read source vocabulary; the target differs.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ErrNoEnrichTarget is returned when a company that IS visible has no domain to
// read — the caller maps it to a 422, distinct from a 404 (not visible).
var ErrNoEnrichTarget = errors.New("people: company has no domain to enrich from")

// EnrichTargetURL returns the URL scrapeCompany should read for companyID: the
// company's primary domain as https://<domain>. Row-scoped — a company the caller
// cannot see is ErrNotFound (existence-hiding); a visible company with no domain
// is ErrNoEnrichTarget.
func (s *Store) EnrichTargetURL(ctx context.Context, companyID ids.CompanyID) (string, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return "", err
	}
	var domain string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "company", companyID.UUID); err != nil {
			return err
		}
		err := tx.QueryRow(ctx,
			`SELECT domain FROM company_domain
			 WHERE company_id = $1 AND archived_at IS NULL
			 ORDER BY is_primary DESC, created_at ASC
			 LIMIT 1`, companyID).Scan(&domain)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoEnrichTarget
		}
		return err
	})
	if err != nil {
		return "", err
	}
	return "https://" + domain, nil
}

// ApplyEnrichment executes an ACCEPTED enrichment proposal against the named
// company: fill only empty columns, upsert the evidence row for every field, never
// overwrite a human-set value. One transaction, one audit row, one
// company.updated event; captured_by is the executing principal
// (agent:scrape), source is site_read.
func (s *Store) ApplyEnrichment(ctx context.Context, companyID ids.CompanyID, in ApplyColdStartProfileInput) error {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if len(in.Fields) == 0 {
		return errors.New("people: an accepted enrichment carries no fields")
	}

	return s.tx(ctx, func(tx pgx.Tx) error {
		wsID := workspaceID(ctx)
		// The target is a KNOWN row — an enrichment never creates or resolves
		// by domain. Row-scope is re-checked here so a leaked company id buys
		// nothing (existence-hiding 404).
		//
		// LIVE, not merely visible: the proposal is staged and approved later,
		// so the company can be archived between the scrape and this apply.
		// EnsureWritableLive says why that is the write's own obligation.
		if err := auth.EnsureWritableLive(ctx, tx, "company", companyID.UUID); err != nil {
			return err
		}
		// The name lock before the row lock the image read takes, and only when a
		// name is coming — the ordering readColdStartColumnImages names as its
		// caller's obligation, and the one every other writer of an
		// company name follows. Taken the other way round, an enrichment
		// carrying a legal name deadlocks against a human rename.
		if carriesCompanyName(in.Fields) {
			if err := lockCompanyNameWrites(ctx, tx); err != nil {
				return err
			}
		}
		before, err := readColdStartColumnImages(ctx, tx, companyID)
		if err != nil {
			return err
		}
		applied, err := applyEvidenceFields(ctx, tx, wsID, companyID, by, in.Fields)
		if err != nil {
			return err
		}
		after, err := readColdStartColumnImages(ctx, tx, companyID)
		if err != nil {
			return err
		}
		before, after = storekit.ChangedColumns(before, after)
		// before/after carry the RECORD's own column images and nothing else.
		// The operation's metadata rides audit_log.evidence, which is the column
		// for it: anything placed in the images is projected by field history as
		// a change to a field of that name (storekit.AuditWithEvidence).
		auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "company", companyID.UUID, before, after, map[string]any{
			auditKeySource: companySourceSiteRead, auditKeySourceURL: in.SourceURL, auditKeyFields: applied,
		})
		if err != nil {
			return fmt.Errorf("audit enrichment apply: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, companyID.UUID, crmcontracts.PublicEventCompanyUpdated{
			ChangedFields: map[string]any{
				eventKeyDelta: applied, auditKeySource: companySourceSiteRead, auditKeySourceURL: in.SourceURL,
			},
		}); err != nil {
			return fmt.Errorf("emit company.updated: %w", err)
		}
		return nil
	})
}

// UnmarshalEnrichment decodes a staged enrichment proposal — the company id plus
// the shared field array. Shared with the compose effect so both sides agree
// on the JSON shape.
func UnmarshalEnrichment(raw json.RawMessage) (ids.CompanyID, string, []ColdStartFieldInput, error) {
	var proposal struct {
		CompanyID ids.CompanyID `json:"company_id"`
		SourceURL string        `json:"source_url"`
		Fields    []struct {
			Field           string  `json:"field"`
			Value           string  `json:"value"`
			EvidenceSnippet string  `json:"evidence_snippet"`
			SourceURL       string  `json:"source_url"`
			Confidence      float32 `json:"confidence"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(raw, &proposal); err != nil {
		return ids.CompanyID{}, "", nil, fmt.Errorf("people: enrichment proposal payload: %w", err)
	}
	fields := make([]ColdStartFieldInput, 0, len(proposal.Fields))
	for _, f := range proposal.Fields {
		fields = append(fields, ColdStartFieldInput{
			Field: f.Field, Value: f.Value, EvidenceSnippet: f.EvidenceSnippet,
			SourceURL: f.SourceURL, Confidence: f.Confidence,
		})
	}
	return proposal.CompanyID, proposal.SourceURL, fields, nil
}
