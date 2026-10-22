// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A LinkedIn match a human has to judge is an APPROVAL, not a queue of its own
// (founder decision, 2026-08-02).
//
// The first build gave the suggest tier its own list, its own confirm and
// reject endpoints and its own card. That was a second inbox: the product
// already has one place where a proposal waits for a contact, it already
// records who decided what and when, and a member who works through their
// morning approvals should not also have to remember a settings tab.
//
// So the tier stages here instead. The ghost row keeps only the OUTCOME
// (`unmatched` until decided, `confirmed` once the effect runs); the pending
// state lives in the approval, which ADR-0036 makes the authority object.
//
// Rejection is durable because the approval row persists. The matcher skips a
// ghost that already carries a decided proposal, so refusing "André is Andre"
// once means never being asked again — including after a re-import, which is
// the case that matters when somebody refreshes a five-thousand-row export.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// linkedInMatchKind is the staging kind. One per suggested match, not one per
// import: the decisions are independent, and a batch proposal would force a
// member to take thirty links to get the three they wanted.
const linkedInMatchKind = "linkedin_match"

// linkedInMatchProposal is what the inbox renders and the effect executes.
//
// It carries the ghost's own strings: the name, employer and position LinkedIn
// exported. A human judges the guess on those. It carries nothing else about
// the connection, because a ghost is a third party who never agreed to be in
// this CRM. Anyone who can decide a staged payload reads it. For the same
// reason it names no CRM company: the card resolves the company for its reader.
type linkedInMatchProposal struct {
	ConnectionID ids.UUID `json:"connection_id"`
	// OwnerUserID is the member whose network produced the pair, stamped at
	// staging from the ghost row. The apply binds on it: without it the write
	// gated the CONTACT and nothing tied the CONNECTION, so a payload naming
	// another member's connection applied to it.
	OwnerUserID ids.UUID `json:"owner_user_id"`
	ContactID   ids.UUID `json:"contact_id"`
	// ConnectionName and ConnectionCompany are the export's own spelling. The
	// folded forms the matcher compared on are deliberately absent: nobody can
	// decide "andreas muller · simio".
	ConnectionName     string `json:"connection_name"`
	ConnectionCompany  string `json:"connection_company,omitempty"`
	ConnectionPosition string `json:"connection_position,omitempty"`
	ContactName        string `json:"contact_name"`
	// ContactTitle and EmployerRelationshipID are the contact's title and
	// current primary employment when the card was staged. The apply refuses a
	// ticked box whose field has moved since.
	ContactTitle           string    `json:"contact_title,omitempty"`
	EmployerRelationshipID *ids.UUID `json:"employer_relationship_id,omitempty"`
	// EmployerCompanyID is the existing company the import placed the
	// connection at; absent means the card offers no employer.
	EmployerCompanyID *ids.UUID `json:"employer_company_id,omitempty"`
	// UseAsTitle and UseAsEmployer are the card's two boxes, and the only
	// members a human's edit may change. A card staged before they existed
	// carries neither and applies only the profile link.
	UseAsTitle    *bool `json:"use_as_title,omitempty"`
	UseAsEmployer *bool `json:"use_as_employer,omitempty"`
}

// fields is the proposal as the contacts store applies it.
func (p linkedInMatchProposal) fields() contacts.LinkedInMatchFields {
	return contacts.LinkedInMatchFields{
		Position: p.ConnectionPosition, ContactTitle: p.ContactTitle,
		EmployerCompanyID: p.EmployerCompanyID, EmployerRelationshipID: p.EmployerRelationshipID,
		UseAsTitle: p.UseAsTitle != nil && *p.UseAsTitle, UseAsEmployer: p.UseAsEmployer != nil && *p.UseAsEmployer,
	}
}

// withGhostOwnerAsSubject records the acting member as the subject the
// proposals are staged for. A context with no human actor cannot stage: a
// self-only proposal nobody is recorded for is one nobody can ever decide.
//
// Returns the stamped subject alongside the context so a caller that needs
// it (to fold into an identity, say) reads back the exact id just recorded
// rather than asking principal.Actor a second time for an answer this
// function already computed and cannot itself have gotten wrong.
func withGhostOwnerAsSubject(ctx context.Context) (context.Context, ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return nil, ids.Nil, apperrors.ErrPermissionDenied
	}
	actor.OnBehalfOf = actor.UserID
	return principal.WithActor(ctx, actor), actor.OnBehalfOf, nil
}

// linkedInMatchStager is the seam contacts.Handlers calls after an import. It
// holds the approvals service so the transport does not have to, and builds it
// ONCE: the registration list is a dozen effects over a dozen stores, and
// rebuilding it per upload produces the same service every time.
func linkedInMatchStager(pool *pgxpool.Pool) func(context.Context) error {
	svc, store := approvalsServiceWithEffects(pool), contacts.NewStore(InstallationDB(pool))
	return func(ctx context.Context) error {
		_, err := StageLinkedInMatches(ctx, pool, svc, store)
		return err
	}
}

// StageLinkedInMatches proposes every undecided name-and-employer match this
// member's network produced.
//
// It runs under the ghost owner's own authority — the caller establishes that,
// as every other pass over these rows does — so a contact outside their row
// scope never becomes a proposal they can see.
func StageLinkedInMatches(ctx context.Context, pool *pgxpool.Pool, svc *approvals.Service, store *contacts.Store) (int, error) {
	pending, err := store.PendingLinkedInMatches(ctx)
	if err != nil {
		return 0, err
	}
	return stagePendingLinkedInMatches(ctx, pool, svc, store, pending)
}

// StageLinkedInMatchesForContact is the same pass narrowed to the matches about
// ONE contact.
//
// The rule both entry points keep is that a pass proposes over the SAME scope it
// matched. Matching against a single arrival can only have raised questions
// about that arrival, so this is the complete answer for that caller as well as
// the bounded one: proposing the member's entire outstanding set instead would
// run once per contact event and only ever rejoin rows that already exist.
func StageLinkedInMatchesForContact(ctx context.Context, pool *pgxpool.Pool, svc *approvals.Service, store *contacts.Store, contact ids.UUID) (int, error) {
	pending, err := store.PendingLinkedInMatchesForContact(ctx, contact)
	if err != nil {
		return 0, err
	}
	return stagePendingLinkedInMatches(ctx, pool, svc, store, pending)
}

// stagePendingLinkedInMatches turns the candidates a match produced into
// proposals — the one place both scopes pass through.
func stagePendingLinkedInMatches(
	ctx context.Context, pool *pgxpool.Pool, svc *approvals.Service, store *contacts.Store,
	pending []contacts.PendingLinkedInMatch,
) (int, error) {
	// Staged ON BEHALF OF the member whose network produced it, so the audit
	// trail records whose export raised the question. It grants nothing and
	// withholds nothing: who may decide is the inbox's ordinary rule — the
	// grants the effect needs, and visibility of the CONTACT the proposal is
	// about. ADR-0078/A123 settles that deliberately: who-knows-whom is
	// workspace-shared metadata, guarded by "you only see edges for a contact
	// you can see at all", which is exactly what that rule already applies.
	ctx, _, err := withGhostOwnerAsSubject(ctx)
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}
	said := approvalSummaryCopyOver(ctx, pool)
	staged := 0
	for _, m := range pending {
		proposed, err := stageOneLinkedInMatch(ctx, svc, said, m)
		if err != nil {
			return staged, err
		}
		if proposed {
			staged++
			continue
		}
		// Not staged means one thing here and the engine is precise about it:
		// StageUnlessDeclined refuses only when a prior offer for this identity
		// was REJECTED. So this is where a refusal made BEFORE the decline
		// effect shipped becomes observable, and the connection is marked
		// terminal — a repair pass, not the path a rejection takes now.
		//
		// A rejection made today lands in linkedInMatchDeclineEffect, inside the
		// decision's own transaction, so the ghost goes terminal at the moment
		// the human says no rather than on the next hourly sweep. This call
		// stays because it is what heals the rows refused before that existed,
		// and because it costs one predicate that matches nothing once they are.
		//
		// The cost of not doing it is what makes it worth a write: the sweep
		// enumerates (unmatched, suggested), so a refused row was matched,
		// read and staged on every hourly pass for ever, always to no effect.
		if err := store.RecordLinkedInMatchRefused(ctx, m.ConnectionID); err != nil {
			return staged, err
		}
	}
	return staged, nil
}

func stageOneLinkedInMatch(ctx context.Context, svc *approvals.Service, said approvalSummaryCopy, m contacts.PendingLinkedInMatch) (bool, error) {
	canonical, hash, err := diffhash.Object(linkedInMatchPayload(m))
	if err != nil {
		return false, err
	}
	// The identity is the CONNECTION, not the diff: a later export that changes
	// the employer string should supersede the stale proposal for the same
	// connection rather than compete with it in the inbox. JoinPending makes
	// the re-import path idempotent, which matters because a member refreshing
	// a five-thousand-row export re-runs this over every row.
	identity, err := json.Marshal(map[string]string{"connection_id": m.ConnectionID.String()})
	if err != nil {
		return false, err
	}
	// StageUnlessDeclined, not Stage. A refusal is durable, and the engine
	// already owns that memory: it takes the identity lock BEFORE reading the
	// declined set, which closes the gap a hand-rolled "read the decided ids,
	// then stage" leaves open — a human rejecting between the two would have
	// the same question re-asked immediately.
	_, proposed, err := svc.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind:           linkedInMatchKind,
		ProposedChange: canonical,
		DiffHash:       hash,
		TargetType:     string(recordTypeContact),
		TargetID:       m.ContactID,
		Identity:       identity,
		JoinPending:    true,
		Summary: fmt.Sprintf(said.linkedInLooksLike,
			m.ConnectionName, employerOrPlaceholder(said, m.ConnectionCompany), m.ContactName),
	})
	if errors.Is(err, approvals.ErrNoDecider) {
		return false, nil
	}
	return proposed, err
}

// linkedInMatchPayload is the staged change, spelled as linkedInMatchProposal
// reads it. Every field the card shows is in it, so a later export with a new
// position supersedes the pending card through JoinPending.
//
//craft:ignore naked-any diffhash.Object canonicalizes a JSON object, and its members hold strings and booleans alike
func linkedInMatchPayload(m contacts.PendingLinkedInMatch) map[string]any {
	payload := map[string]any{
		"connection_id": m.ConnectionID.String(), "owner_user_id": m.OwnerUserID.String(),
		companionContactField: m.ContactID.String(),
		"connection_name":     m.ConnectionName, "connection_company": m.ConnectionCompany,
		"contact_name": m.ContactName,
		"use_as_title": m.Fields.UseAsTitle, "use_as_employer": m.Fields.UseAsEmployer,
	}
	optional := map[string]string{"connection_position": m.Fields.Position, "contact_title": m.Fields.ContactTitle}
	if m.Fields.EmployerCompanyID != nil {
		optional["employer_company_id"] = m.Fields.EmployerCompanyID.String()
	}
	if m.Fields.EmployerRelationshipID != nil {
		optional["employer_relationship_id"] = m.Fields.EmployerRelationshipID.String()
	}
	for key, value := range optional {
		if value != "" {
			payload[key] = value
		}
	}
	return payload
}

func employerOrPlaceholder(said approvalSummaryCopy, employer string) string {
	if employer == "" {
		return said.unnamedEmployer
	}
	return employer
}

// linkedInMatchDeclineEffect marks the ghost row terminal when a member says no.
//
// Rejecting used to change nothing on the connection. The approvals record said
// declined and the domain record still said suggested, and the two disagreed
// until a sweep happened to notice — up to an hour, and only because
// StageUnlessDeclined refused to re-propose. The dead branches were the tell:
// matchRankOrder carried a `rejected` slot and the pending read carried
// `<> 'rejected'`, and no writer in the tree could produce a row for either.
//
// The DECISION's transaction, handed in: the rejection and the mark commit
// together, so a failed mark takes the rejection with it and the member can
// answer again. Rejected-but-still-suggested is the one outcome this card
// cannot produce — the same shape heldDeclineEffect takes, and for the same
// reason.
func linkedInMatchDeclineEffect(store *contacts.Store) approvals.DeclinedEffect {
	return func(ctx context.Context, tx pgx.Tx, _ ids.ApprovalID, proposedChange json.RawMessage) error {
		var p linkedInMatchProposal
		if err := json.Unmarshal(proposedChange, &p); err != nil {
			return fmt.Errorf("compose: unreadable LinkedIn match proposal: %w", err)
		}
		// The OWNER and the CONTACT the proposal named, not the deciding actor
		// and not whoever the row points at now: a refusal answers one
		// suggestion, and the store binds on the pair for the same reason the
		// apply does.
		return contacts.RecordLinkedInMatchRefusedTx(ctx, tx, p.ConnectionID, p.OwnerUserID, p.ContactID)
	}
}

// linkedInMatchPrecheck refuses a ticked box that no longer fits while the card
// is still pending. That is a title or employer set while it waited, a new
// export, or a decider who may not make the employer write. Refused after the
// decision, the card would stay approved and its apply would fail every retry.
func linkedInMatchPrecheck(store *contacts.Store) approvals.ReleasePrecheck {
	return func(ctx context.Context, staged, edited json.RawMessage) error {
		payload := staged
		if len(edited) > 0 {
			payload = edited
		}
		p, err := readLinkedInMatchProposal(payload)
		if err != nil {
			return err
		}
		return store.CheckLinkedInMatchFields(ctx, p.apply())
	}
}

func readLinkedInMatchProposal(raw json.RawMessage) (linkedInMatchProposal, error) {
	var p linkedInMatchProposal
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("compose: unreadable LinkedIn match proposal: %w", err)
	}
	return p, nil
}

// apply is the accepted match as the contacts store writes it.
func (p linkedInMatchProposal) apply() contacts.LinkedInMatchApply {
	return contacts.LinkedInMatchApply{
		ConnectionID: p.ConnectionID, OwnerUserID: p.OwnerUserID, ContactID: p.ContactID, Fields: p.fields(),
	}
}

// linkedInMatchAcceptEffect links the connection to the contact and puts the
// LinkedIn address on the record. That is the write the automatic exact-name
// path performs, released by a human. It also writes the boxes they ticked.
func linkedInMatchAcceptEffect(svc *approvals.Service, store *contacts.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		p, err := readLinkedInMatchProposal(proposedChange)
		if err != nil {
			return err
		}
		if _, ok := principal.Actor(ctx); !ok {
			return errors.New("compose: LinkedIn match effect without a deciding principal")
		}
		apply := p.apply()
		// Read before the transaction opens: the catalog read takes its own,
		// and a second pool acquire inside the write is a deadlock under load.
		if apply.Fields.UseAsTitle {
			columns, err := store.ActiveContactUpdateColumns(ctx)
			if err != nil {
				return err
			}
			apply.Columns = columns
		}
		// ONE TRANSACTION, which is what RedeemAndApply is for. Redeem-then-apply
		// was two: the redemption committed, and a failure in the apply left the
		// approval consumed and the connection never linked — unrecoverable
		// through the API, because the row is decided and re-deciding answers
		// 409. Redeem's own doc says callers should use this and have no window
		// at all, and every other accept effect in compose already does.
		//
		// The single-use redemption is still the idempotency claim: whoever
		// consumes the approval executes, anyone else finds it consumed. What
		// changes is that a consumed approval now implies the write landed.
		//
		// Executed as the DECIDER, not as a machine: a member approving a match
		// is making the claim themselves, and the write must be gated by their
		// grants and recorded against them.
		return svc.RedeemAndApply(ctx, approvalID, linkedInMatchKind, diffHash, func(tx pgx.Tx) error {
			return store.ApplyLinkedInMatchTx(ctx, tx, apply)
		})
	}
}
