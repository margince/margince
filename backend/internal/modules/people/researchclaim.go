// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What a human accepted from a research run (ADR-0096 D4).
//
// This is the ONLY write in the whole research surface. A run stages and
// touches nothing; the record changes here, once somebody has read the claims
// and chosen which of them are true.
//
// It lands in person_profile_field, the same table the signature-enrichment
// pass writes: a fact about who this person IS, with the evidence it was read
// from. That table's shape is what makes the guarantee enforceable rather than
// promised — evidence_snippet and source_ref are both NOT NULL, so a claim
// that lost its quote or its source cannot be stored at all.
//
// captured_by names the HUMAN who accepted it, not the provider that proposed
// it. The provider's URL rides source_ref, so the chain from record to document
// stays intact; but the decision was a person's, and the record says so.

import (
	"context"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// webSourceURL admits only a document somebody can actually open. A host is
// required as well as a scheme: bare "http:" parses cleanly and points nowhere.
func webSourceURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Host == "" {
		return false
	}
	return parsed.Scheme == schemeHTTPS || parsed.Scheme == schemeHTTP
}

// The two schemes a citation can honestly carry.
const (
	schemeHTTPS = "https"
	schemeHTTP  = "http"
)

// researchSource marks these rows as a human's acceptance of a provider's
// claim, distinguishing them from the signature pass that shares the table.
const researchSource = "person_research"

// ResearchClaimInput is one accepted claim.
type ResearchClaimInput struct {
	// Field is which profile field this claim fills. The table constrains the
	// set, so a claim naming anything else is refused by the database rather
	// than silently stored under a name no reader looks for.
	Field string
	Value string
	// Quote and SourceURL are the evidence. Both are required: a fact a reader
	// cannot trace to a document is what the review step exists to stop.
	Quote     string
	SourceURL string
}

// SaveResearchClaims writes the accepted claims and returns how many landed.
//
// The whole set commits in ONE transaction. A partial save would leave the
// reader with no way to tell which half of their decision took effect, and
// re-running the drawer would then re-offer claims they had already accepted.
func (s *Store) SaveResearchClaims(ctx context.Context, personID ids.PersonID, claims []ResearchClaimInput) (int, error) {
	// Malformed input is named before authority is asked about: a caller who
	// sent a claim with no evidence made a mistake the server can describe.
	for i, claim := range claims {
		if claim.Value == "" || claim.Quote == "" || claim.SourceURL == "" {
			return 0, httperr.Validation(fmt.Sprintf("claims[%d]", i), "required",
				"a saved research claim carries its value, the words it was read from, and the document it came from — one that lost any of the three cannot be checked, and is refused rather than stored")
		}
		// The scheme is checked HERE, not only where the run produced it. A
		// client is not obliged to send back what the run returned, so a read
		// path that refuses javascript: and a write path that accepts it is the
		// asymmetry that lands untrusted input in the record and waits for a
		// renderer to turn it into a sink.
		if !webSourceURL(claim.SourceURL) {
			return 0, httperr.Validation(fmt.Sprintf("claims[%d].source_url", i), "invalid",
				"a source is a document a reader can open — give an http or https URL with a host")
		}
	}
	// After validation, so the messages above still point at the caller's own
	// indices, and before authority, which is about the person rather than the
	// batch.
	claims = lastClaimPerField(claims)
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return 0, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return 0, err
	}

	saved := 0
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// Reset inside the closure: WithWorkspaceTx may run it again, and a
		// counter that survived the retry would tell the caller more claims
		// landed than exist — and put that inflated number in the audit row.
		saved = 0
		if err := auth.EnsureWritableLive(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		for _, claim := range claims {
			// The one write in this table that REPLACES: somebody read the
			// claim, its quote and the document behind it, and chose it.
			// updated_at and version are the trigger's
			// (trg_person_profile_field_updated), so nothing here names them.
			landed, err := writePersonProfileField(ctx, tx, personID, personProfileFieldRow{
				Field: claim.Field, Value: claim.Value, EvidenceSnippet: claim.Quote,
				SourceRef: claim.SourceURL, Source: researchSource, CapturedBy: by,
			}, replaceOnAcceptance)
			if err != nil {
				return fmt.Errorf("save the research claim %q: %w", claim.Field, err)
			}
			if !landed {
				// An acceptance REPLACES, so the only way a row does not land
				// is the writer refusing it for the subject: an erasure
				// committed between EnsureWritableLive above and the row lock
				// the writer takes, which at READ COMMITTED is a fresh
				// snapshot. The
				// whole acceptance fails rather than counting a claim that is
				// not there — the count rides the audit row and the outbox
				// event, so a saved++ here would record a mutation that never
				// happened, on the one table whose purpose is traceability.
				return apperrors.ErrNotFound
			}
			saved++
		}
		// The claims land in person_profile_field, never on the person row, so
		// there is no column image to carry — and quoting the values would put a
		// second copy of the subject's data in a table the erasure treats as
		// evidence. The image names WHICH fields a person accepted, a closed
		// vocabulary the table constrains: a row whose before and after are both
		// empty records that something happened and cannot say what. How many
		// landed is context about the write and rides evidence.
		auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "person", personID.UUID,
			map[string]any{"research_claims_accepted": acceptedClaimFields(claims)},
			map[string]any{"research_claims_saved": saved})
		if err != nil {
			return fmt.Errorf("audit the saved research: %w", err)
		}
		// person.updated is the open envelope its own doc names for exactly
		// this: a signature-enrichment fill is the same shape as a research
		// fill, and both are a change-set rather than a fixed field list.
		return storekit.EmitEvent(ctx, tx, auditID, personID.UUID,
			crmcontracts.PublicEventPersonUpdated{
				ChangedFields: map[string]any{
					"profile_fields": map[string]any{"saved": saved, auditKeySource: researchSource},
				},
			})
	})
	if err != nil {
		return 0, err
	}
	return saved, nil
}

// lastClaimPerField keeps one claim per field, the last the caller sent, in the
// order the fields first appeared.
//
// Two claims for one field in a batch are two answers to one question, and
// applying both did not merely pick one: the acceptance REPLACES, so the second
// claim's value reached the evidence row, while the empty-social-slot fill the
// first claim triggered is ADDITIVE and kept the first claim's URL. The record
// then showed one LinkedIn address in its research section and a different one
// on the rail, with nothing in the audit trail saying which the human meant.
//
// Last wins because the batch is one form submission read top to bottom, and
// the caller's final word about a field is the one they left there.
func lastClaimPerField(claims []ResearchClaimInput) []ResearchClaimInput {
	out := make([]ResearchClaimInput, 0, len(claims))
	at := make(map[string]int, len(claims))
	for _, claim := range claims {
		if i, seen := at[claim.Field]; seen {
			out[i] = claim
			continue
		}
		at[claim.Field] = len(out)
		out = append(out, claim)
	}
	return out
}

// acceptedClaimFields is the audit image's list of what the human accepted —
// the field names, which the table's own constraint closes, never the values
// they carry.
func acceptedClaimFields(claims []ResearchClaimInput) []string {
	out := make([]string, 0, len(claims))
	for _, claim := range claims {
		out = append(out, claim.Field)
	}
	return out
}
