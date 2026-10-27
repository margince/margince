// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

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
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

type CreateLeadInput struct {
	FullName *string
	Email    *string
	// Phone is E.164 by the time it reaches the column, normalised through the
	// same values.ParsePhone a contact's number goes through. One number, like
	// the one email beside it: a lead is a single enquiry until it is promoted.
	Phone               *string
	Title               *string
	CompanyName         *string
	CandidateCompanyKey *string
	// Industry is a code by the time it reaches the column (leadIndustryWrite).
	Industry     *string
	LinkedInURL  *string
	Status       string
	OwnerID      *ids.UserID
	ProjectID    *ids.ProjectID
	SourceSystem *string
	SourceID     *string
	// Source is how the row reached Margince; it never scores.
	Source string
	// Origin is where the lead came from: a lead_origin key. OriginHint is
	// the raw wording only an importer sets; it resolves to a key, or stays in
	// OriginDetail with Origin left nil (admitLeadOrigin).
	Origin       *string
	OriginDetail *string
	OriginHint   string
	// Author is who wrote it in the system it came from; zero when unknown.
	Author storekit.SourceAuthorInput
	// CustomFields carries the request body's cf_* keys. A key no active catalog
	// column takes is refused (storekit.CheckCustomValues).
	CustomFields map[string]any
	// RequestedContactID is the deprecated contact_id request field. It is
	// retained only to return the contact-held refusal older clients understand.
	RequestedContactID *ids.ContactID
}

// CreateLead inserts into the segregated lead table — never contact, never
// relationship (ADR-0008: the anti-pollution guarantee is structural).
// Idempotent on (source_system, source_id): a re-import returns the
// existing row instead of erroring, so bulk sourcing can re-run.
func (s *Store) CreateLead(ctx context.Context, in CreateLeadInput) (crmcontracts.Lead, bool, error) {
	if err := auth.Require(ctx, "lead", principal.ActionCreate); err != nil {
		return crmcontracts.Lead{}, false, err
	}
	in, by, err := s.readyLeadCreate(ctx, in)
	if err != nil {
		return crmcontracts.Lead{}, false, err
	}
	active, err := s.activeColumns(ctx, "lead")
	if err != nil {
		return crmcontracts.Lead{}, false, err
	}

	var out crmcontracts.Lead
	created := true
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, created, err = createLeadInTx(ctx, tx, in, by, active)
		if err != nil || !created {
			return err
		}
		return s.recordLeadNearMatch(ctx, tx, out, by)
	})
	return out, created, s.TellContactOwnerOfLead(ctx, err, in, arrivalOrigin(ctx, in.Source))
}

// arrivalOrigin names the door a lead came through, for the contact owner's
// task: an agent, or the source the request declared.
func arrivalOrigin(ctx context.Context, source string) string {
	if actor, ok := principal.Actor(ctx); ok && actor.Type == principal.PrincipalAgent {
		return "an agent"
	}
	if source == "" {
		return "the API"
	}
	return source
}

// CreateLeadTx is CreateLead inside a transaction the caller opened, whose own
// write must land with this lead or not at all. Same gates, same order. The
// bool is false when the idempotency replay found the lead already landed.
//
// Custom fields ride on the columns the caller read before it opened the
// transaction (ActiveCreateColumns); with none, they are refused, never dropped.
func (s *Store) CreateLeadTx(ctx context.Context, tx pgx.Tx, in CreateLeadInput, cols CustomColumns) (crmcontracts.Lead, bool, error) {
	if err := auth.Require(ctx, "lead", principal.ActionCreate); err != nil {
		return crmcontracts.Lead{}, false, err
	}
	if err := refuseUncataloguedCustomFields(in.CustomFields, cols); err != nil {
		return crmcontracts.Lead{}, false, err
	}
	in, by, err := s.readyLeadCreate(ctx, in)
	if err != nil {
		return crmcontracts.Lead{}, false, err
	}
	out, created, err := createLeadInTx(ctx, tx, in, by, cols.cols)
	if err != nil || !created {
		return out, created, err
	}
	return out, created, s.recordLeadNearMatch(ctx, tx, out, by)
}

// readyLeadCreate runs what a create settles BEFORE any transaction opens —
// the captured-by resolution and the input normalization — and answers the
// normalized input beside the attribution the write shape stamps. Both entry
// points call it, so neither can drift from the other's validation.
func (s *Store) readyLeadCreate(ctx context.Context, in CreateLeadInput) (CreateLeadInput, string, error) {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return CreateLeadInput{}, "", err
	}
	normalized, err := normalizedCreateLeadInput(in)
	if err != nil {
		return CreateLeadInput{}, "", err
	}
	// The owner is exactly what a HUMAN caller named — deliberately not the
	// creator default every other manual create applies. A lead is
	// the funnel's queue entity: it arrives unassigned unless somebody names
	// an owner, routing (leadrouting.go) is what assigns it, and the claim
	// verb plus the Unassigned list dial exist for the ownerless state. The
	// UI's create form still defaults its owner picker to the creating rep,
	// so a rep's own lead stays theirs by an explicit choice, not a fallback.
	//
	// An AGENT's create keeps the on-behalf-of fallback: EnsureWritable
	// refuses ownerless rows and the claim verb is human-only, so a NULL
	// owner would strand the very lead the agent just filed — it could never
	// update, qualify or disqualify it again. The queue is a human choice.
	if p, ok := principal.Actor(ctx); ok && p.Type == principal.PrincipalAgent && normalized.OwnerID == nil {
		normalized.OwnerID = storekit.OwnerOrActor(ctx)
	}
	return normalized, by, nil
}

// createLeadInTx is CreateLead's transactional body, shared by the
// store-opened and caller-opened entry points. It answers the lead and
// whether this call is what created it.
func createLeadInTx(ctx context.Context, tx pgx.Tx, in CreateLeadInput, by string,
	active []fieldcatalog.Column,
) (crmcontracts.Lead, bool, error) {
	replay, err := replayedLead(ctx, tx, in, active)
	if err != nil {
		return crmcontracts.Lead{}, false, err
	}
	if replay != nil {
		return *replay, false, nil
	}
	if err := admitLeadCreate(ctx, tx, in); err != nil {
		return crmcontracts.Lead{}, false, err
	}
	in, err = admitLeadOrigin(ctx, tx, in)
	if err != nil {
		return crmcontracts.Lead{}, false, err
	}
	id, err := insertLeadRow(ctx, tx, in, active, by)
	if err != nil {
		return crmcontracts.Lead{}, false, err
	}

	auditID, err := storekit.Audit(ctx, tx, "create", "lead", id.UUID, nil,
		map[string]any{
			leadEmailColumn: in.Email, leadPhoneColumn: in.Phone, leadCompanyColumn: in.CompanyName,
			leadOriginField: in.Origin, leadOriginDetailField: in.OriginDetail,
		})
	if err != nil {
		return crmcontracts.Lead{}, false, fmt.Errorf("audit lead create: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventLeadCreated{SourceSystem: in.SourceSystem}); err != nil {
		return crmcontracts.Lead{}, false, fmt.Errorf("emit lead.created: %w", err)
	}
	out, err := readLead(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Lead{}, false, fmt.Errorf("read created lead: %w", err)
	}
	return out, true, nil
}

// insertLeadRow writes the lead row itself and answers with its id.
func insertLeadRow(ctx context.Context, tx pgx.Tx, in CreateLeadInput, active []fieldcatalog.Column, by string) (ids.LeadID, error) {
	if in.Email != nil {
		if err := storekit.RefuseErasedEmail(ctx, tx, leadEmailColumn, *in.Email); err != nil {
			return ids.LeadID{}, err
		}
	}
	fit, err := initialLeadScore(ctx, tx, in)
	if err != nil {
		return ids.LeadID{}, err
	}
	id := ids.New[ids.LeadKind]()
	if err := storekit.RefuseUnknownSeat(ctx, tx, in.Author); err != nil {
		return ids.LeadID{}, err
	}
	fixed := []any{
		id, in.FullName, in.Email, in.Title, in.CompanyName, in.CandidateCompanyKey,
		in.LinkedInURL, in.Status, fit.Score, in.OwnerID, in.ProjectID, in.SourceSystem, in.SourceID, in.Source, by,
		in.Phone, in.Origin, in.OriginDetail, in.Industry,
	}
	holders := strings.Split(storekit.Placeholders(fixed), ",")
	// The email is the third bind: lower() holds the dedupe key's spelling at the column, whatever door wrote it.
	holders[2] = "lower(" + holders[2] + ")"
	authorCols, authorHolders, base := storekit.AuthorInsertFragments(in.Author, fixed)
	cfCols, cfHolders, args, err := storekit.InsertFragments(active, in.CustomFields, base)
	if err != nil {
		return ids.LeadID{}, err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO lead (id, full_name, email, title, company_name, candidate_company_key,
		                   linkedin_url, status, score, owner_id, project_id, source_system, source_id, source, captured_by,
		                   phone, origin, origin_detail, `+leadIndustryField+authorCols+cfCols+`)
		 VALUES (`+strings.Join(holders, ", ")+authorHolders+cfHolders+`)`,
		args...)
	if err != nil {
		// Race behind the pre-checks: the constraint name tells an
		// email dedupe hit from a concurrent same-source import — the
		// latter is a plain conflict, not a "duplicate email" (the
		// email may not even be set). No re-read here: the failed
		// INSERT aborted the transaction.
		if mapped, ok := leadUniqueViolation(err, in.Email); ok {
			return ids.LeadID{}, mapped
		}
		return ids.LeadID{}, fmt.Errorf("insert lead: %w", err)
	}
	// The first point in the series, written with the score it explains
	// (ADR-0105 §1). A lead created and never recomputed still opens.
	if err := appendLeadScoreHistory(ctx, tx, id, fit.Score, fit, nil); err != nil {
		return ids.LeadID{}, err
	}
	return id, nil
}

// normalizedCreateLeadInput is CreateLead's parse-don't-validate step:
// status defaults and is membership-checked, and the two identity keys —
// email and LinkedIn URL — normalize ONCE here, so the dedupe probes,
// the insert and the audit image all see one spelling (the SQL lower()
// stays as defense in depth).
func normalizedCreateLeadInput(in CreateLeadInput) (CreateLeadInput, error) {
	if in.Status == "" {
		in.Status = string(LeadStatusNew)
	}
	if _, err := parseWritableLeadStatus(in.Status); err != nil {
		return CreateLeadInput{}, err
	}
	if in.Email != nil {
		parsed, err := values.ParseEmail(*in.Email)
		if err != nil {
			return CreateLeadInput{}, err
		}
		normalized := parsed.String()
		in.Email = &normalized
	}
	if in.Phone != nil {
		parsed, err := values.ParsePhone(*in.Phone)
		if err != nil {
			return CreateLeadInput{}, err
		}
		normalized := parsed.String()
		in.Phone = &normalized
	}
	if in.LinkedInURL != nil {
		normalized, err := NormalizeContactLinkedInURL(*in.LinkedInURL)
		if err != nil {
			return CreateLeadInput{}, err
		}
		in.LinkedInURL = &normalized
	}
	industry, err := leadIndustryWrite(in.Industry)
	if err != nil {
		return CreateLeadInput{}, err
	}
	in.Industry = industry
	return in, nil
}

// replayedLead resolves the (source_system, source_id) idempotency key:
// a re-import returns the existing row. The replay path returns a
// record, so it carries the read's row scope: re-importing someone
// else's source key must not hand over their lead — out of scope
// answers the same 409 the unique-index race does.
func replayedLead(ctx context.Context, tx pgx.Tx, in CreateLeadInput, active []fieldcatalog.Column) (*crmcontracts.Lead, error) {
	if in.SourceSystem == nil || in.SourceID == nil {
		return nil, nil
	}
	var existing ids.LeadID
	err := tx.QueryRow(ctx,
		`SELECT id FROM lead
			  WHERE source_system = $1 AND source_id = $2`,
		*in.SourceSystem, *in.SourceID).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("probe source-key idempotency: %w", err)
	}
	visible, err := auth.VisibleTo(ctx, tx, "lead", existing.UUID)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, apperrors.ErrConflict
	}
	out, err := readLead(ctx, tx, existing, storekit.IncludeArchived, active)
	if err != nil {
		return nil, fmt.Errorf("read replayed lead: %w", err)
	}
	return &out, nil
}

// FindLeadByLinkedInURL is the E12.11 exact-match dedupe probe: the
// earliest-captured live lead holding this profile URL (the canonical
// original when duplicates slipped in), or nil when the workspace has none.
// The lookup normalizes its input the way CreateLead stores it, so the
// comparison is exact by construction. Returning a record makes this a
// read: the caller's row scope applies, and an out-of-scope match reads
// as no match — the capture path then warns on what the caller could see,
// never on hidden rows (idx_lead_linkedin is a lookup index, not UNIQUE:
// merging duplicates is a human decision, so the probe warns, it does not
// refuse).
func (s *Store) FindLeadByLinkedInURL(ctx context.Context, rawURL string) (*crmcontracts.Lead, error) {
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return nil, err
	}
	normalized, err := NormalizeContactLinkedInURL(rawURL)
	if err != nil {
		return nil, err
	}

	args := []any{normalized}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := scopeOrAllRows(ctx, "lead", "", arg)
	if err != nil {
		return nil, err
	}

	var out *crmcontracts.Lead
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		ctx, err = withLeadTaskIdentities(ctx, tx)
		if err != nil {
			return err
		}
		// A dedupe probe for the capture path — its result is not rendered
		// with custom fields, so no catalog columns are carried (nil active).
		policy, err := loadLeadSLAPolicy(ctx, tx, s.bootConfig())
		if err != nil {
			return err
		}
		columns, err := leadColumns(ctx, arg)
		if err != nil {
			return err
		}
		l, err := scanLead(tx.QueryRow(ctx,
			`SELECT `+columns+` FROM lead
			 WHERE linkedin_url = $1 AND archived_at IS NULL AND `+scope+`
			 ORDER BY created_at ASC, id ASC LIMIT 1`, args...), nil, policy)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("probe linkedin dedupe: %w", err)
		}
		out = &l
		return nil
	})
	return out, err
}

func (s *Store) GetLead(ctx context.Context, id ids.LeadID, archived storekit.ArchivedFilter) (crmcontracts.Lead, error) {
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return crmcontracts.Lead{}, err
	}
	active, err := s.activeColumns(ctx, "lead")
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	var out crmcontracts.Lead
	err = s.tx(ctx, func(tx pgx.Tx) (err error) {
		if err := auth.EnsureVisible(ctx, tx, "lead", id.UUID); err != nil {
			return err
		}
		out, err = readLeadOffered(ctx, tx, id, archived, active)
		return err
	})
	return out, err
}

// leadUniqueViolation maps a lead write's unique-index violation to the
// contract error: the email dedupe index answers 409 duplicate-email; any
// other unique index a plain conflict. The bool is false when err is not a
// unique violation at all, so the caller keeps its own wrapping.
func leadUniqueViolation(err error, email *string) (error, bool) {
	name, ok := storekit.UniqueViolation(err)
	if !ok {
		return nil, false
	}
	if name == "uq_lead_email_dedupe" {
		return &DuplicateLeadError{Email: deref(email)}, true
	}
	return apperrors.ErrConflict, true
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func admitLeadCreate(ctx context.Context, tx pgx.Tx, in CreateLeadInput) error {
	if err := refuseRequestedContact(ctx, tx, in.RequestedContactID, leadKeys(in.Email, in.LinkedInURL)); err != nil {
		return err
	}
	if err := lockLeadLinkedInIdentity(ctx, tx, in.LinkedInURL); err != nil {
		return err
	}
	if err := ensureLeadEmailUnclaimed(ctx, tx, in.Email); err != nil {
		return err
	}
	if err := ensureLeadLinkedInUnclaimed(ctx, tx, in.LinkedInURL); err != nil {
		return err
	}
	if err := refuseContactHolders(ctx, tx, leadKeys(in.Email, in.LinkedInURL), nil); err != nil {
		return err
	}

	if in.ProjectID != nil {
		if err := auth.EnsureReferenceTarget(ctx, tx, "project_id", "project", in.ProjectID.UUID); err != nil {
			return err
		}
	}
	if in.OwnerID != nil {
		if err := auth.EnsureNewRecordOwner(ctx, tx, in.OwnerID.UUID); err != nil {
			return err
		}
	}
	return nil
}
