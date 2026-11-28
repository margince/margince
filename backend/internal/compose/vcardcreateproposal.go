// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A vCard near-match, made durable.
//
// The import creates nothing beside a near-match. The question outlives the
// upload as a staged proposal. Approve creates the contact, or with
// merge_into_candidate files the card on the contact it resembled. Reject
// leaves the card uncreated, and a decline is remembered.
// The proposal carries no target. Keeping the card separate is gated on the
// create grants. The merge also checks the decider's authority over the
// candidate at decision time.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// vcardCreateKind names the staged offer in the review queue.
const vcardCreateKind = approvals.KindVCardCreate

// vcardReviewWindow is how long an undecided card waits. The row holds the
// personal data of somebody nobody accepted into the product. The expiry
// deletes the card from the row.
const vcardReviewWindow = 30 * 24 * time.Hour

// vcardCreateProposal is the staged payload: the card as parsed, and the
// record it resembled when it was refused. FullName and Emails repeat the
// card's addressing in the canonical lowered form because they ARE the
// staged identity, and the approvals engine verifies an identity against
// the payload that carries it.
type vcardCreateProposal struct {
	Entry contacts.VCardEntry `json:"entry"`
	// Name is the name the accepted create writes, and the one the decider is
	// shown. FullName below is the folded identity and is never displayed.
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	// Emails is the card's addresses lowered, sorted and joined — ONE string,
	// because an identity field is a string by the engine's contract.
	Emails string `json:"emails"`
	// The rest of what an approval RELEASES, flattened for the card: the
	// decider is shown every field the create will write, or they are
	// approving more than they were asked. Empty fields are omitted so the
	// card carries facts, not blanks — Company is the one exception:
	// it also joins the identity below, and the engine's containment check
	// refuses an identity asserting a field the payload omits. A card
	// naming no company still displays as nothing; omitempty just cannot
	// be the reason it does, or that card can never be staged at all.
	Company string `json:"company"`
	Title   string `json:"title,omitempty"`
	Phones  string `json:"phones,omitempty"`
	URL     string `json:"url,omitempty"`
	Address string `json:"address,omitempty"`
	// CandidateContactID names the near-match the import saw, when the
	// importer could see it too. Keeping the card separate does not read it;
	// the merge does, and checks the decider against it.
	CandidateContactID *openapi_types.UUID `json:"candidate_contact_id,omitempty"`
	// MergeIntoCandidate is the decider's choice, set through the edit arm.
	// Staged false, and a row staged before the field existed reads false.
	MergeIntoCandidate bool `json:"merge_into_candidate"`
	// StagedBy is the importer's own user id. The engine's supersession and
	// decline memory carry no subject filter, and a card is addressed by facts
	// a stranger can guess. Folded into the identity and the hashed payload, it
	// keeps one member's staging from answering a colleague's same-named card.
	// Not shown to a decider.
	StagedBy string `json:"staged_by"`
}

// VCardCreateStager builds the contacts module's review port: one card's
// near-match becomes one durable proposal.
func VCardCreateStager(pool *pgxpool.Pool) func(ctx context.Context, entry contacts.VCardEntry, candidate *ids.ContactID) error {
	svc := NewApprovalsServiceWithEffects(pool)
	return func(ctx context.Context, entry contacts.VCardEntry, candidate *ids.ContactID) error {
		// Self-only, like the LinkedIn match: the importer is the proposal's
		// subject and the only contact who can read or decide it. subject is the
		// exact id the approval row carries as on_behalf_of.
		ctx, subject, err := withGhostOwnerAsSubject(ctx)
		if err != nil {
			return err
		}
		phones := make([]string, 0, len(entry.Phones))
		for _, phone := range entry.Phones {
			if v := strings.TrimSpace(phone.Value); v != "" {
				phones = append(phones, v)
			}
		}
		proposal := vcardCreateProposal{
			Entry: entry,
			Name:  contacts.VCardContactName(entry),
			// The dedupe lane's own folding, not a plain lowercase: a name it
			// treats as the same contact must hit the same decline memory, or
			// a re-spelt card walks past a refusal.
			FullName: contacts.NormalizeContactName(entry.FullName),
			Emails:   loweredCardEmails(entry),
			Company:  strings.TrimSpace(entry.Company),
			Title:    strings.TrimSpace(entry.Title),
			Phones:   strings.Join(phones, ", "),
			URL:      strings.TrimSpace(entry.URL),
			Address:  strings.TrimSpace(entry.Address),
			StagedBy: subject.String(),
		}
		if candidate != nil {
			id := openapi_types.UUID(candidate.UUID)
			proposal.CandidateContactID = &id
		}
		body, marshalErr := json.Marshal(proposal)
		if marshalErr != nil {
			return fmt.Errorf("compose: encoding the vCard create proposal: %w", marshalErr)
		}
		// Identity is the card's own addressing, not the whole payload. A
		// re-export that reorders phone numbers is then the same question, and
		// its decline is kept. staged_by says whose card it is. No field named
		// here carries `omitempty` above. The engine refuses an identity that
		// asserts a key the payload omits.
		identity, err := json.Marshal(map[string]any{
			fieldFullName:             proposal.FullName,
			"emails":                  proposal.Emails,
			string(recordTypeCompany): proposal.Company,
			"staged_by":               proposal.StagedBy,
		})
		if err != nil {
			return fmt.Errorf("compose: encoding the vCard proposal identity: %w", err)
		}
		digest := sha256.Sum256(body)
		window := vcardReviewWindow
		_, _, err = svc.StageUnlessDeclined(ctx, approvals.StageInput{
			Why:            approvals.HeadlineSaysWhy,
			Kind:           vcardCreateKind,
			ProposedChange: body,
			DiffHash:       hex.EncodeToString(digest[:]),
			Identity:       identity,
			Summary:        vcardCreateSummary(approvalSummaryCopyOver(ctx, pool), entry, candidate != nil),
			SystemText:     vcardSummaryMetadata(entry, candidate != nil),
			JoinPending:    true,
			TTL:            &window,
		})
		return err
	}
}

// vcardCreateSummary keeps the qualifier — the whole reason the card is in
// the queue — ahead of the card's own name, and bounds that name: the name
// is the uploader's text, and a long one placed first would push "resembles
// an existing contact" past the summary's truncation. Every language's
// sentence keeps that order.
func vcardCreateSummary(said approvalSummaryCopy, entry contacts.VCardEntry, resemblesContact bool) string {
	name := textcut.RunesMarked(strings.TrimSpace(entry.FullName), 80, "…")
	if entry.ModelRead && !resemblesContact {
		return fmt.Sprintf(said.vcardReadFromPhoto, name)
	}
	return fmt.Sprintf(said.vcardResemblesContact, name)
}

// loweredCardEmails is the card's addresses in the canonical form the
// identity is remembered against — lowered, deduplicated, sorted, and
// joined on a byte no address can carry — so the same set is one spelling
// and two different sets can never collide on a delimiter inside a value.
func loweredCardEmails(entry contacts.VCardEntry) string {
	seen := map[string]bool{}
	emails := make([]string, 0, len(entry.Emails))
	for _, email := range entry.Emails {
		v := correspondence.Fold(email.Value)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		emails = append(emails, v)
	}
	sort.Strings(emails)
	return strings.Join(emails, "\n")
}

// vcardCreatePrecheck refuses, while the proposal is still pending and
// re-decidable, what the create would refuse after the decision committed —
// the modify-then-approve arm can rewrite the payload, and an edit that
// dropped the entry would otherwise create a contact with no name.
func vcardCreatePrecheck(store *contacts.Store) approvals.ReleasePrecheck {
	return func(ctx context.Context, staged, edited json.RawMessage) error {
		payload := staged
		if len(edited) > 0 {
			payload = edited
		}
		var proposal vcardCreateProposal
		if err := json.Unmarshal(payload, &proposal); err != nil {
			return errors.New("this card's proposal could not be read; reject it and re-import the file")
		}
		if strings.TrimSpace(proposal.Entry.FullName) == "" {
			return errors.New("the card names nobody: a contact needs a name before they can be created")
		}
		// The create's own parse, asked while the row is still re-decidable, so
		// a number the writer refuses fails the decision, not the effect after
		// it. Parsing only. Another contact can claim an address between this
		// check and the approval, so asking here would promise too much.
		if err := contacts.ValidateVCardContacts(proposal.Entry); err != nil {
			return fmt.Errorf("this card carries a contact detail that cannot be stored: %w", err)
		}
		if !proposal.MergeIntoCandidate {
			return nil
		}
		if proposal.CandidateContactID == nil {
			return errors.New("this card resembles no contact it could be merged into; keep it separate or reject it")
		}
		return store.RequireVCardMergeTarget(ctx, ids.From[ids.ContactKind](ids.UUID(*proposal.CandidateContactID)))
	}
}

// vcardCreateAcceptEffect executes the approved answer under the decider's own
// authority. Either the contact is created with the card's employer edge, or
// the card is filed on the contact it resembled. Rejecting has no effect.
//
// The write rides RedeemAndApply, so the redemption and what it releases commit
// in one transaction. A redelivered decision finds the approval consumed, and a
// failed write rolls the redemption back with it.
func vcardCreateAcceptEffect(svc *approvals.Service, store *contacts.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var proposal vcardCreateProposal
		if err := json.Unmarshal(proposedChange, &proposal); err != nil {
			return fmt.Errorf("compose: decoding the vCard create proposal: %w", err)
		}
		// The approval owns this transaction, so a colleague holding the card's
		// address is asked to share it only after the transaction rolled back.
		return store.AskHolderToShare(ctx, svc.RedeemAndApply(ctx, approvalID, vcardCreateKind, diffHash, func(tx pgx.Tx) error {
			if proposal.MergeIntoCandidate {
				if proposal.CandidateContactID == nil {
					return apperrors.ErrNotFound
				}
				return store.MergeVCardIntoTx(ctx, tx, ids.From[ids.ContactKind](ids.UUID(*proposal.CandidateContactID)), proposal.Entry)
			}
			_, err := store.CreateFromVCardReviewTx(ctx, tx, proposal.Entry)
			return err
		}))
	}
}

func vcardSummaryMetadata(entry contacts.VCardEntry, resembles bool) *systemtext.Message {
	code := "approval.vcardResemblesContact"
	if entry.ModelRead && !resembles {
		code = "approval.vcardReadFromPhoto"
	}
	return &systemtext.Message{Version: 1, Subject: systemtext.Sentence(code, systemtext.Text(textcut.RunesMarked(strings.TrimSpace(entry.FullName), 80, "…")))}
}
