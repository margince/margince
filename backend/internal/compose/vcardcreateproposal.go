// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A vCard near-match, made durable.
//
// The import refuses to create beside a near-match — creating is how a file
// of forty cards quietly doubles a contact list — and used to answer only in
// the upload response: a transient needs_review line the reader had to act on
// before closing the tab, with the card's data gone the moment they did. The
// question now outlives the upload as a staged proposal: approve means "this
// is somebody else, create them", reject means the card stays uncreated, and
// a decline is remembered so re-importing the same file does not re-ask.
//
// The candidate person is the proposal's TARGET when the importer could see
// it, which is what lets the card name the record it resembles. A candidate
// outside the importer's scope stages with no target — the same
// existence-hiding answer the import response gives.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// vcardCreateKind names the staged offer in the review queue.
const vcardCreateKind = "vcard_create"

// vcardCreateProposal is the staged payload: the card as parsed, and the
// record it resembled when it was refused. FullName and Emails repeat the
// card's addressing in the canonical lowered form because they ARE the
// staged identity, and the approvals engine verifies an identity against
// the payload that carries it.
type vcardCreateProposal struct {
	Entry    people.VCardEntry `json:"entry"`
	FullName string            `json:"full_name"`
	// Emails is the card's addresses lowered, sorted and joined — ONE string,
	// because an identity field is a string by the engine's contract.
	Emails string `json:"emails"`
	// CandidatePersonID names the near-match the import saw, when the
	// importer could see it too. Informational for the decider; the create
	// itself does not read it.
	CandidatePersonID *openapi_types.UUID `json:"candidate_person_id,omitempty"`
}

// vcardCreateStager builds the people module's review port: one card's
// near-match becomes one durable proposal.
func vcardCreateStager(pool *pgxpool.Pool) func(ctx context.Context, entry people.VCardEntry, candidate *ids.PersonID) error {
	svc := approvalsServiceWithEffects(pool)
	return func(ctx context.Context, entry people.VCardEntry, candidate *ids.PersonID) error {
		proposal := vcardCreateProposal{
			Entry:    entry,
			FullName: strings.ToLower(strings.TrimSpace(entry.FullName)),
			Emails:   loweredCardEmails(entry),
		}
		if candidate != nil {
			id := openapi_types.UUID(candidate.UUID)
			proposal.CandidatePersonID = &id
		}
		body, err := json.Marshal(proposal)
		if err != nil {
			return fmt.Errorf("compose: encoding the vCard create proposal: %w", err)
		}
		// Identity is the card's own addressing — who this card claims to be —
		// not the whole payload: a re-export that reorders phone numbers or
		// tweaks a title is still the same question, and a decline keyed on
		// the full payload would be forgotten the first time a field moved.
		identity, err := json.Marshal(map[string]any{
			fieldFullName: proposal.FullName,
			"emails":      proposal.Emails,
		})
		if err != nil {
			return fmt.Errorf("compose: encoding the vCard proposal identity: %w", err)
		}
		digest := sha256.Sum256(body)
		in := approvals.StageInput{
			Kind:           vcardCreateKind,
			ProposedChange: body,
			DiffHash:       hex.EncodeToString(digest[:]),
			Identity:       identity,
			Summary:        "Create " + strings.TrimSpace(entry.FullName) + " anyway? The card resembles an existing contact.",
			JoinPending:    true,
		}
		if candidate != nil {
			in.TargetType = flipObjectPerson
			in.TargetID = candidate.UUID
		}
		_, _, err = svc.StageUnlessDeclined(ctx, in)
		return err
	}
}

// loweredCardEmails is the card's addresses in the canonical form the
// identity is remembered against — lowered, sorted, comma-joined — so the
// same card asked twice is ONE question however it is re-exported.
func loweredCardEmails(entry people.VCardEntry) string {
	emails := make([]string, 0, len(entry.Emails))
	for _, email := range entry.Emails {
		if v := strings.ToLower(strings.TrimSpace(email.Value)); v != "" {
			emails = append(emails, v)
		}
	}
	sort.Strings(emails)
	return strings.Join(emails, ",")
}

// vcardCreateAcceptEffect executes the approved answer: the human said this
// is somebody else, so the person is created with the card's employer edge,
// under the decider's own authority. There is no reject effect — rejecting a
// create leaves the world exactly as it was.
func vcardCreateAcceptEffect(store *people.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, _ ids.ApprovalID, proposedChange json.RawMessage, _ string) error {
		var proposal vcardCreateProposal
		if err := json.Unmarshal(proposedChange, &proposal); err != nil {
			return fmt.Errorf("compose: decoding the vCard create proposal: %w", err)
		}
		_, err := store.CreateFromVCardReview(ctx, proposal.Entry)
		return err
	}
}
