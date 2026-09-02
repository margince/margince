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
// The proposal deliberately carries NO target — the candidate travels in the
// payload for display only, and deciding is gated on the create grants
// alone, which is exactly what the approval spends. The decline memory is
// keyed on (kind, target, identity) and the identity carries the staging
// subject, so the memory is per-importer by design: a self-only kind's
// declines are one member's own answers, never another member's.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
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
	// The rest of what an approval RELEASES, flattened for the card: the
	// decider is shown every field the create will write, or they are
	// approving more than they were asked. Empty fields are omitted so the
	// card carries facts, not blanks — Organization is the one exception:
	// it also joins the identity below, and the engine's containment check
	// refuses an identity asserting a field the payload omits. A card
	// naming no company still displays as nothing; omitempty just cannot
	// be the reason it does, or that card can never be staged at all.
	Organization string `json:"organization"`
	Title        string `json:"title,omitempty"`
	Phones       string `json:"phones,omitempty"`
	URL          string `json:"url,omitempty"`
	Address      string `json:"address,omitempty"`
	// CandidatePersonID names the near-match the import saw, when the
	// importer could see it too. Informational for the decider; the create
	// itself does not read it.
	CandidatePersonID *openapi_types.UUID `json:"candidate_person_id,omitempty"`
	// StagedBy is the importer's own user id. vcard_create is self-only — one
	// member's own uploaded address book — but neither the identity-keyed
	// supersession nor the diff-hash join/decline-memory the engine runs for
	// EVERY kind carries a subject filter of its own; both trust a caller's
	// discriminator to already be collision-proof. A card is otherwise
	// addressed by facts a stranger can read off a business card or simply
	// guess (a name, a company), so without this a second member staging a
	// same-named — or even a same-named-and-same-company — card could
	// silently withdraw or answer a colleague's unrelated pending review.
	// Folding the subject in here, in BOTH the identity and (because it is a
	// plain field with no omitempty) the hashed payload, makes every one of
	// this kind's proposals inherently per-importer no matter which fields
	// the card itself states. Not offered to a decider (no DISPLAY_FIELDS
	// entry) — a self-only proposal's importer already knows their own id.
	StagedBy string `json:"staged_by"`
}

// vcardCreateStager builds the people module's review port: one card's
// near-match becomes one durable proposal.
func vcardCreateStager(pool *pgxpool.Pool) func(ctx context.Context, entry people.VCardEntry, candidate *ids.PersonID) error {
	svc := approvalsServiceWithEffects(pool)
	return func(ctx context.Context, entry people.VCardEntry, candidate *ids.PersonID) error {
		// Self-only, like the LinkedIn match one seam over: the card is one
		// member's own uploaded address book, so the importer is recorded as
		// the proposal's subject and is the only person who can read or
		// decide it. Without the stamp a self-only row is decidable by
		// nobody, so the two halves land together. subject is the exact id
		// the approval row itself will carry as on_behalf_of.
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
			// The dedupe lane's own folding, not a plain lowercase: a name it
			// treats as the same person must hit the same decline memory, or
			// a re-spelt card walks past a refusal.
			FullName:     people.NormalizePersonName(entry.FullName),
			Emails:       loweredCardEmails(entry),
			Organization: strings.TrimSpace(entry.Organization),
			Title:        strings.TrimSpace(entry.Title),
			Phones:       strings.Join(phones, ", "),
			URL:          strings.TrimSpace(entry.URL),
			Address:      strings.TrimSpace(entry.Address),
			StagedBy:     subject.String(),
		}
		if candidate != nil {
			id := openapi_types.UUID(candidate.UUID)
			proposal.CandidatePersonID = &id
		}
		body, marshalErr := json.Marshal(proposal)
		if marshalErr != nil {
			return fmt.Errorf("compose: encoding the vCard create proposal: %w", marshalErr)
		}
		// Identity is the card's own addressing — who this card claims to be —
		// not the whole payload: a re-export that reorders phone numbers or
		// tweaks a title is still the same question, and a decline keyed on
		// the full payload would be forgotten the first time a field moved.
		// staged_by joins it for a different reason than the card fields do:
		// full_name, emails and organization say WHICH card; staged_by says
		// WHOSE — and without it, two members' cards naming the same person
		// (or, for a bare name with no other addressing at all, simply
		// sharing one) would be one question in the engine's eyes, answerable
		// and withdrawable by either member's stager.
		//
		// Every field named here must carry no `omitempty` on the struct
		// above: the engine refuses an identity asserting a key the payload's
		// JSON omits, and a card whose value is legitimately empty (no email,
		// no company) is not exempt from being asked about.
		identity, err := json.Marshal(map[string]any{
			fieldFullName:                  proposal.FullName,
			"emails":                       proposal.Emails,
			string(recordTypeOrganization): proposal.Organization,
			"staged_by":                    proposal.StagedBy,
		})
		if err != nil {
			return fmt.Errorf("compose: encoding the vCard proposal identity: %w", err)
		}
		digest := sha256.Sum256(body)
		_, _, err = svc.StageUnlessDeclined(ctx, approvals.StageInput{
			Kind:           vcardCreateKind,
			ProposedChange: body,
			DiffHash:       hex.EncodeToString(digest[:]),
			Identity:       identity,
			Summary:        vcardCreateSummary(entry.FullName),
			JoinPending:    true,
		})
		return err
	}
}

// vcardCreateSummary keeps the qualifier — the whole reason the card is in
// the queue — ahead of the card's own name, and bounds that name: the name
// is the uploader's text, and a long one placed first would push "resembles
// an existing contact" past the summary's truncation.
func vcardCreateSummary(fullName string) string {
	name := strings.TrimSpace(fullName)
	if runes := []rune(name); len(runes) > 80 {
		name = string(runes[:80]) + "…"
	}
	return "This card resembles an existing contact. Create " + name + " anyway?"
}

// loweredCardEmails is the card's addresses in the canonical form the
// identity is remembered against — lowered, deduplicated, sorted, and
// joined on a byte no address can carry — so the same set is one spelling
// and two different sets can never collide on a delimiter inside a value.
func loweredCardEmails(entry people.VCardEntry) string {
	seen := map[string]bool{}
	emails := make([]string, 0, len(entry.Emails))
	for _, email := range entry.Emails {
		v := strings.ToLower(strings.TrimSpace(email.Value))
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
// dropped the entry would otherwise create a person with no name.
func vcardCreatePrecheck() approvals.ReleasePrecheck {
	return func(_ context.Context, staged, edited json.RawMessage) error {
		payload := staged
		if len(edited) > 0 {
			payload = edited
		}
		var proposal vcardCreateProposal
		if err := json.Unmarshal(payload, &proposal); err != nil {
			return errors.New("this card's proposal could not be read; reject it and re-import the file")
		}
		if strings.TrimSpace(proposal.Entry.FullName) == "" {
			return errors.New("the card names nobody: a person needs a name before they can be created")
		}
		// The same question the create asks, asked while the row is still
		// re-decidable. Without it a card carrying a number the writer refuses
		// is approved, the create fails after the decision, and the decider
		// learns about it from the did-not-run lane instead of from the
		// decision they were making.
		//
		// PARSING only, and deliberately. The create also refuses an address
		// another person already claims, and that refusal cannot be brought
		// forward honestly: a claim can be taken between this check and the
		// approval, so asking here would not prevent the post-commit failure —
		// it would only make it rarer while reading like a guarantee. A parse
		// failure is a property of the card itself and cannot change under it.
		if err := people.ValidateVCardContacts(proposal.Entry); err != nil {
			return fmt.Errorf("this card carries a contact detail that cannot be stored: %w", err)
		}
		return nil
	}
}

// vcardCreateAcceptEffect executes the approved answer: the human said this
// is somebody else, so the person is created with the card's employer edge,
// under the decider's own authority. There is no reject effect — rejecting a
// create leaves the world exactly as it was.
//
// The create rides RedeemAndApply, so the approval's redemption and the
// person it releases commit in ONE transaction: a redelivered decision finds
// the approval consumed and creates nobody a second time, and a create that
// fails rolls the redemption back with it rather than stranding an approved
// row whose person never appeared.
func vcardCreateAcceptEffect(svc *approvals.Service, store *people.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var proposal vcardCreateProposal
		if err := json.Unmarshal(proposedChange, &proposal); err != nil {
			return fmt.Errorf("compose: decoding the vCard create proposal: %w", err)
		}
		return svc.RedeemAndApply(ctx, approvalID, vcardCreateKind, diffHash, func(tx pgx.Tx) error {
			_, err := store.CreateFromVCardReviewTx(ctx, tx, proposal.Entry)
			return err
		})
	}
}
