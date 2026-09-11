// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The accepted cold-start read-back (features/07 §1): a human approval
// releases the staged fields onto the company the source URL names
// — resolve by domain, create when absent, fill only what no human has
// set, and keep every value's verbatim evidence queryable
// (company_profile_field, 0037). One transaction, one audit row,
// one company event; captured_by comes from the executing
// principal (agent:coldstart), source is site_read.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// coldStartSource is the provenance a company minted by an accepted
// cold-start proposal carries.
const coldStartSource = "coldstart"

// ColdStartFieldInput is one accepted, evidenced field.
type ColdStartFieldInput struct {
	Field           string
	Value           string
	EvidenceSnippet string
	SourceURL       string
	Confidence      float32
}

// ApplyColdStartProfileInput carries the whole accepted proposal.
type ApplyColdStartProfileInput struct {
	SourceURL string
	Fields    []ColdStartFieldInput
}

// columnLegalName is the company COLUMN, which is a different namespace
// from the field of the same spelling: columnBackedColdStartFields maps
// registered_address onto address_line1, so a field name and a column name
// agreeing here is a coincidence, not a rule. The two constants stay apart so
// renaming one cannot silently rename the other.
const (
	columnLegalName   = "legal_name"
	columnIndustry    = "industry"
	columnAddress     = "address"
	columnDescription = "description"
)

// columnBackedColdStartFields maps read-back fields onto company
// columns; everything else lives only in company_profile_field.
var columnBackedColdStartFields = map[string]string{
	fieldLegalName:       columnLegalName,
	"industry":           "industry",
	"registered_address": "address",
	fieldOfferSummary:    "description",
}

// ApplyColdStartProfile executes an ACCEPTED coldstart proposal. A
// column a human (or any earlier capture) already filled is left
// untouched — acceptance covers the staged diff, not an overwrite of
// standing values (features/07 §2: colliding writes need their own 🟡).
// The evidence row is upserted for EVERY accepted field, column-backed
// or not, so provenance stays queryable either way.
func (s *Store) ApplyColdStartProfile(ctx context.Context, in ApplyColdStartProfileInput) (ids.CompanyID, error) {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return ids.CompanyID{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.CompanyID{}, err
	}
	host, err := coldStartHost(in.SourceURL)
	if err != nil {
		return ids.CompanyID{}, err
	}
	if len(in.Fields) == 0 {
		return ids.CompanyID{}, errors.New("people: an accepted coldstart proposal carries no fields")
	}

	var companyID ids.CompanyID
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		companyID, err = applyColdStartTx(ctx, tx, in, host, by)
		if err != nil {
			return err
		}
		// A VAT number a read just extracted has not been checked, and this is
		// where most of them arrive — a rep correcting one afterwards is the
		// rarer path. Queued in the SAME transaction, so a rolled-back apply
		// leaves no job asking about a number the record does not hold.
		if statesAVatNumber(in.Fields) {
			return s.enqueueVatCheck(ctx, tx, companyID)
		}
		return nil
	})
	if err != nil {
		return ids.CompanyID{}, err
	}
	return companyID, nil
}

// applyColdStartTx resolves-or-creates the target company, fills the
// accepted fields (evidence for every one, columns only when empty), and
// runs the write shape — create vs update chosen by whether the company was
// just minted — all inside the caller's transaction.
func applyColdStartTx(ctx context.Context, tx pgx.Tx, in ApplyColdStartProfileInput, host, by string) (ids.CompanyID, error) {
	wsID := workspaceID(ctx)
	// Same pair of rules as ApplyDeepReadTx: only when a name is coming, and
	// then before the row lock the image read takes. Stated here rather than
	// left to the resolve step to take on the way past, so the ordering does
	// not depend on which branch that step happens to follow.
	if carriesCompanyName(in.Fields) {
		if err := lockCompanyNameWrites(ctx, tx); err != nil {
			return ids.CompanyID{}, err
		}
	}
	companyID, created, err := resolveOrCreateColdStartCompany(ctx, tx, host, by, in.Fields)
	if err != nil {
		return ids.CompanyID{}, err
	}
	if err := gateResolvedColdStartTarget(ctx, tx, companyID, created); err != nil {
		return ids.CompanyID{}, err
	}
	// The columns as they stand before the apply, including display_name: a
	// created company gets its name here, and that is a column change a
	// reader is entitled to see.
	//
	// A row this call just minted has no before-image at all. Reading one back
	// would return the values the create itself wrote, ChangedColumns
	// would then find nothing moved, and the create audit would omit the name
	// it inserted — the one change the row is entirely made of.
	var before map[string]any
	if created {
		before = emptyColdStartColumnImages()
	} else if before, err = readColdStartColumnImages(ctx, tx, companyID); err != nil {
		return ids.CompanyID{}, err
	}
	applied, err := applyEvidenceFields(ctx, tx, wsID, companyID, by, in.Fields)
	if err != nil {
		return ids.CompanyID{}, err
	}
	after, err := readColdStartColumnImages(ctx, tx, companyID)
	if err != nil {
		return ids.CompanyID{}, err
	}
	before, after = storekit.ChangedColumns(before, after)

	action := "update"
	if created {
		action = "create"
	}
	// before/after carry the RECORD's own column images and nothing else. The
	// operation's metadata rides audit_log.evidence, which is the column for
	// it: anything placed in the images is projected by field history as a
	// change to a field of that name (storekit.AuditWithEvidence).
	auditID, err := storekit.AuditWithEvidence(ctx, tx, action, "company", companyID.UUID, before, after, map[string]any{
		auditKeySource: companySourceSiteRead, auditKeySourceURL: in.SourceURL, auditKeyFields: applied,
	})
	if err != nil {
		return ids.CompanyID{}, fmt.Errorf("audit coldstart apply: %w", err)
	}
	payload := coldStartApplyPayload(created, in, host, by, applied)
	if err := storekit.EmitEvent(ctx, tx, auditID, companyID.UUID, payload); err != nil {
		return ids.CompanyID{}, fmt.Errorf("emit %s: %w", payload.EventType(), err)
	}
	return companyID, nil
}

// coldStartApplyPayload builds the company-side event an accepted
// cold-start profile emits — company.created (the union struct,
// display_name/primary_domain from the accepted legal_name field and the
// resolved host) when the apply minted a fresh company, or an
// company.updated changed_fields note when it filled an existing
// one — the ONE place that maps the applied field delta onto the
// published schema. The two shapes are different published events, not
// variants of one, so the return type is the shared events.Payload seam.
//
//nolint:ireturn // dispatches to PublicEventCompanyCreated vs Updated by the created condition; tested directly via the interface in person_company_payload_test.go
func coldStartApplyPayload(created bool, in ApplyColdStartProfileInput, host, by string, applied map[string]any) events.Payload {
	if created {
		displayName := fieldValue(in.Fields, fieldLegalName)
		if displayName == "" {
			// The company row is stored with the domain-derived name when no
			// legal_name was accepted (resolveOrCreateColdStartCompany's fallback),
			// so company.created must publish the SAME derived value —
			// never the raw host or an empty display_name the record does not
			// actually carry.
			displayName = DisplayNameFromDomain(host)
			if displayName == "" {
				displayName = host
			}
		}
		primaryDomain := host
		source := companySourceSiteRead
		capturedBy := by
		return crmcontracts.PublicEventCompanyCreated{
			DisplayName:   &displayName,
			PrimaryDomain: &primaryDomain,
			Source:        &source,
			CapturedBy:    &capturedBy,
		}
	}
	return crmcontracts.PublicEventCompanyUpdated{
		ChangedFields: map[string]any{
			eventKeyDelta: applied, auditKeySource: companySourceSiteRead, auditKeySourceURL: in.SourceURL,
		},
	}
}

// resolveOrCreateColdStartCompany finds the company the source domain
// names, or creates it (with its primary domain) when absent. It reports
// whether it created the company so the caller selects the create/update audit
// action and event.
func resolveOrCreateColdStartCompany(ctx context.Context, tx pgx.Tx, host, by string, fields []ColdStartFieldInput) (ids.CompanyID, bool, error) {
	// Name-source authority (ADR-0072/A118, PO-F-2a). Without a scraped legal
	// name the company is named from the domain's registrable label ("Docusign",
	// not "eu.docusign.net") and marked provisional ('domain'), overwritable by
	// a richer source. A site-stated legal name accepted here is dossier-sourced
	// ('dossier') — read from the site and human-confirmed, but NOT a raw human
	// edit ('human' is reserved for UpdateCompany) — so a later human edit
	// still wins while a weaker signature/domain source cannot clobber it.
	displayName := DisplayNameFromDomain(host)
	if displayName == "" {
		displayName = host
	}
	nameSource := nameSourceDomain
	legal := fieldValue(fields, fieldLegalName)
	if legal != "" {
		displayName = legal
		nameSource = nameSourceDossier
	}

	// PO-F-2 rather than a bare domain lookup: the site's own legal name is the
	// strongest signal this path has, and a company already captured from a
	// different domain collides on exactly that name and on nothing else.
	match, err := DedupeCompanyForCreate(ctx, tx, CompanyCandidate{
		DisplayName: displayName,
		LegalName:   legal,
		Domains:     []string{host},
	})
	if err != nil {
		return ids.CompanyID{}, false, err
	}
	if match.Decision == DecisionExactCollision {
		return match.CompanyID, false, nil
	}

	companyID, err := createCompany(ctx, tx, match, CompanySpec{
		DisplayName: displayName,
		NameSource:  nameSource,
		Domains:     []CompanyDomainInput{{Domain: host, IsPrimary: true}},
		Source:      coldStartSource,
		CapturedBy:  by,
	})
	if err != nil {
		return ids.CompanyID{}, false, err
	}
	if err := match.recordIfReview(ctx, tx, companyID, coldStartSource, by); err != nil {
		return ids.CompanyID{}, false, err
	}
	return companyID, true, nil
}

// applyEvidenceFields fills the column-backed fields (only when empty) and
// upserts the evidence row for EVERY field, returning what was applied. Shared
// by the cold-start read-back and per-company enrichment so both write provenance
// identically; the caller supplies the executing principal (by) and owns the
// audit source. A re-accept refreshes an agent-captured row and never touches
// one a human has since claimed.
func applyEvidenceFields(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, companyID ids.CompanyID, by string, fields []ColdStartFieldInput) (map[string]any, error) {
	return applyEvidenceFieldsWithOverwrite(ctx, tx, wsID, companyID, companySourceSiteRead, by, fields, nil)
}

func applyEvidenceFieldsWithOverwrite(
	ctx context.Context,
	tx pgx.Tx,
	wsID ids.WorkspaceID,
	companyID ids.CompanyID,
	source string,
	by string,
	fields []ColdStartFieldInput,
	overwrite map[string]bool,
) (map[string]any, error) {
	// Only an apply that carries a NAME takes the name lock. The key is
	// workspace-wide, so taking it for a batch of industry or address facts
	// would serialize every company write in the installation behind an
	// apply that cannot rename anything — and enrichment and deep-read arrive
	// in batches.
	//
	// When it IS taken it must come before the loop, not at the re-check that
	// needs it: the loop writes legal_name and so takes this company's row
	// lock, and a path holding both in the other order deadlocks against a
	// human rename. Whether a name is coming is knowable here, which is what
	// makes the early take possible.
	if carriesCompanyName(fields) {
		if err := lockCompanyNameWrites(ctx, tx); err != nil {
			return nil, err
		}
	}
	applied := map[string]any{}
	for _, f := range fields {
		if column, backed := columnBackedColdStartFields[f.Field]; backed {
			filled, err := writeCompanyColumn(ctx, tx, companyID, column, f.Value, overwrite[f.Field])
			if err != nil {
				return nil, err
			}
			if filled {
				applied[f.Field] = f.Value
				// The shared field-provenance layer (B-E02.12) records the
				// filled COLUMN's origin; the profile-field evidence row
				// below keeps the full snippet either way.
				confidence := f.Confidence
				stamp := storekit.FieldStamp{Field: column, Confidence: &confidence}
				if f.SourceURL != "" {
					// A missing source link reads as NULL, never as ''.
					stamp.EvidenceRef = &f.SourceURL
				}
				if err := storekit.StampFields(ctx, tx, "company", companyID.UUID, source, by, []storekit.FieldStamp{stamp}); err != nil {
					return nil, err
				}
			}
		} else {
			applied[f.Field] = f.Value
		}
		// The evidence row lands for every accepted field; a re-accept
		// refreshes an agent-captured row and never touches one a human has
		// since claimed.
		if _, err := tx.Exec(ctx, upsertCompanyProfileField,
			companyID, f.Field, f.Value, f.EvidenceSnippet, f.SourceURL, f.Confidence, source, by, overwrite[f.Field]); err != nil {
			return nil, fmt.Errorf("upsert profile field %s: %w", f.Field, err)
		}
	}
	// A filled legal name is new identity information about a record that
	// already exists — the axis PO-F-2 had nothing to compare when the row was
	// created, and the axis on which a company captured twice under two
	// marketing names finally collides.
	if _, named := applied[fieldLegalName]; named {
		if err := recheckCompanyNameForDuplicates(ctx, tx, companyID, by); err != nil {
			return nil, err
		}
	}
	return applied, nil
}

func carriesCompanyName(fields []ColdStartFieldInput) bool {
	for _, f := range fields {
		if f.Field == fieldLegalName {
			return true
		}
	}
	return false
}

func fieldValue(fields []ColdStartFieldInput, name string) string {
	for _, f := range fields {
		if f.Field == name {
			return f.Value
		}
	}
	return ""
}

func coldStartHost(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("people: coldstart source url %q has no host", rawURL)
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www."), nil
}

// UnmarshalColdStartFields decodes the staged proposal's field array —
// shared with the compose effect so both sides agree on the JSON shape.
func UnmarshalColdStartFields(raw json.RawMessage) (string, []ColdStartFieldInput, error) {
	var proposal struct {
		SourceURL string `json:"source_url"`
		Fields    []struct {
			Field           string  `json:"field"`
			Value           string  `json:"value"`
			EvidenceSnippet string  `json:"evidence_snippet"`
			SourceURL       string  `json:"source_url"`
			Confidence      float32 `json:"confidence"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(raw, &proposal); err != nil {
		return "", nil, fmt.Errorf("people: coldstart proposal payload: %w", err)
	}
	fields := make([]ColdStartFieldInput, 0, len(proposal.Fields))
	for _, f := range proposal.Fields {
		fields = append(fields, ColdStartFieldInput{
			Field: f.Field, Value: f.Value, EvidenceSnippet: f.EvidenceSnippet,
			SourceURL: f.SourceURL, Confidence: f.Confidence,
		})
	}
	return proposal.SourceURL, fields, nil
}
