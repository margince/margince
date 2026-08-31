// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// Asking a colleague for an introduction.
//
// The People tab can already say that Sofia is the warmest way in to Philipp.
// This is the sentence that asks her, and it lives here because the facts it
// rests on — who can reach this contact, how warm, how recently — are this
// package's own reads under the caller's own scope. A separate package would
// have had to copy the route read, and a copied scope clause is one that stops
// agreeing with its original.
//
// A message to a COLLEAGUE, not to a counterparty. Every other draft in this
// tree writes outward, in the register a customer is owed; this asks somebody
// on our own side for a favour. Different register, different length, different
// ask — so it carries its own phrasing rather than bending the outward table
// into a shape it was not written for. The intro-path signal reached the same
// conclusion for the same reason.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/proposeroles"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// IntroRequest names the introduction being asked for.
type IntroRequest struct {
	// PersonID is the contact to be introduced TO.
	PersonID ids.PersonID
	// ViaUserID is the colleague being asked.
	ViaUserID ids.UserID
	// DealID is the deal it is for, or the zero value for the account as a
	// whole — which is the honest shape when a rep is opening a conversation
	// rather than moving a transaction.
	DealID *ids.DealID
}

// introFacts is what one draft may say, and nothing else.
//
// Assembled before any model call and passed whole, so the deterministic floor
// and the model write from exactly the same facts. A floor that could state
// less than the model would make "which wrote this" a question about content
// rather than about phrasing.
type introFacts struct {
	colleague string
	contact   string
	title     string
	account   string
	deal      string
	// band is how warm the colleague's relationship is, in the vocabulary the
	// page already shows. Carried rather than recomputed: a second banding here
	// would let the draft claim a closeness the map does not draw.
	band string
	// lastAt is when they last spoke, or nil when nothing is recorded.
	lastAt *time.Time
	lang   textlang.Lang
}

// IntroRequestDraft writes the message asking a colleague for an introduction.
//
// It reads, and writes nothing at all — no draft row, no activity, no audit
// entry beyond the model call's own. What comes back is for the reader to send
// under their own name, from their own mail client.
func (s *Service) IntroRequestDraft(
	ctx context.Context, lane Completer, orgID ids.OrganizationID, req IntroRequest,
) (crmcontracts.AccountEmailDraft, error) {
	// Human-only: this spends the workspace's model budget on prose a person
	// will send under their own name.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.AccountEmailDraft{}, err
	}
	// The custom-field catalog BEFORE the transaction opens. It takes a
	// connection of its own, and a second connection inside somebody else's
	// transaction commits separately and can deadlock undetectably against a
	// lock that transaction holds. Coverage reads it the same way.
	active, err := s.people.ActiveOrganizationColumns(ctx)
	if err != nil {
		return crmcontracts.AccountEmailDraft{}, err
	}
	var facts introFacts
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		facts, err = s.introFactsFor(ctx, tx, orgID, active, req)
		return err
	})
	if err != nil {
		return crmcontracts.AccountEmailDraft{}, err
	}
	return writeIntroRequest(ctx, lane, facts), nil
}

// introFactsFor assembles the draft's material under the caller's own scope.
func (s *Service) introFactsFor(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
	active people.CustomColumns, req IntroRequest,
) (introFacts, error) {
	// The account first, and its refusal is the whole read's: a caller who
	// cannot open the company has no business drafting about its people.
	org, err := s.people.GetOrganizationTx(ctx, tx, orgID, storekit.LiveOnly, active)
	if err != nil {
		return introFacts{}, err
	}
	identity, err := contactIdentity(ctx, tx, orgID, []ids.PersonID{req.PersonID})
	if err != nil {
		return introFacts{}, err
	}
	who, known := identity[req.PersonID]
	if !known {
		// Not on this account, or not one this caller may see. The two answer
		// the same, because telling them apart would confirm the contact exists.
		return introFacts{}, apperrors.ErrNotFound
	}
	route, err := s.introRoute(ctx, tx, req)
	if err != nil {
		return introFacts{}, err
	}
	facts := introFacts{
		colleague: route.DisplayName,
		contact:   who.fullName,
		title:     titleOf(who),
		account:   org.DisplayName,
		band:      string(route.StrengthBucket),
		lastAt:    route.LastInteractionAt,
	}
	if req.DealID != nil {
		name, err := s.introDealName(ctx, tx, orgID, *req.DealID)
		if err != nil {
			return introFacts{}, err
		}
		facts.deal = name
	}
	// The CORRESPONDENCE decides the language, not the names. Record names are
	// not prose — "Brandt GmbH" and "Retrofit 2026" detect as nothing at all,
	// so a German account was being asked in English. What the contact actually
	// wrote is the signal every other draft here uses, and this read already
	// has it: the same authorship-bound messages the role reading quotes.
	said, err := ownWords(ctx, tx, orgID, []ids.PersonID{req.PersonID}, s.now().UTC())
	if err != nil {
		return introFacts{}, err
	}
	facts.lang = textlang.Detect(correspondenceOf(said[req.PersonID.UUID]))
	return facts, nil
}

// introRoute is the colleague's recorded relationship with the contact.
//
// A route is REQUIRED, not decorative. Asking somebody with no relationship to
// trade on is a favour they cannot do, and a draft claiming one would describe
// a closeness the account's own page does not show — which is exactly the
// sentence a reader would forward and be embarrassed by.
func (s *Service) introRoute(
	ctx context.Context, tx pgx.Tx, req IntroRequest,
) (crmcontracts.Organization360Route, error) {
	may, err := mayReadRoutes(ctx)
	if err != nil {
		return crmcontracts.Organization360Route{}, err
	}
	if !may {
		// Without the activity grant a caller cannot learn who can reach this
		// contact, and a draft naming a colleague would disclose exactly that.
		return crmcontracts.Organization360Route{}, apperrors.ErrPermissionDenied
	}
	routes, err := contactRoutes(ctx, tx, []ids.UUID{req.PersonID.UUID}, s.now().UTC())
	if err != nil {
		return crmcontracts.Organization360Route{}, err
	}
	for _, route := range routes[req.PersonID.UUID].Top {
		if ids.UUID(route.UserId) == req.ViaUserID.UUID {
			return route, nil
		}
	}
	return crmcontracts.Organization360Route{}, apperrors.ErrNotFound
}

// introDealName reads the deal the introduction is for, refusing one this
// caller cannot see or that is no longer open.
func (s *Service) introDealName(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, dealID ids.DealID,
) (string, error) {
	// The refusal STANDS. A caller who named a deal and cannot read it is not
	// asking for an account-wide introduction: silently dropping the deal
	// returns 200 with a draft about the wrong subject, which the rep then
	// sends to a colleague believing it says what they asked for. An ABSENT
	// deal_id is the account-wide case, and the contract says so.
	open, err := s.visibleOpenDeals(ctx, tx, orgID)
	if err != nil {
		return "", err
	}
	for _, candidate := range open {
		if ids.UUID(candidate.DealId) == dealID.UUID {
			return candidate.Name, nil
		}
	}
	return "", apperrors.ErrNotFound
}

// wireIntroRequest turns the draft into the shape the composer reads.
func wireIntroRequest(
	draft introDraft, by crmcontracts.WrittenBy, facts introFacts,
) crmcontracts.AccountEmailDraft {
	aiWritten := by == crmcontracts.Model
	out := crmcontracts.AccountEmailDraft{
		Subject: draft.subject,
		Body:    draft.body,
		// No `to`. The reader sends this from their own mail client, and
		// putting a colleague's address on a payload nothing sends would
		// disclose it for no purpose the draft has.
		GeneratedBy: by,
		AiGenerated: &aiWritten,
		Reasoning:   introReasons(facts),
	}
	if aiWritten {
		disclosure := introDisclosure(facts.lang)
		out.AiDisclosure = &disclosure
	}
	return out
}

// introReasons names what the draft was written FROM, in the reader's own
// words rather than the model's.
//
// Deterministic on both paths, because they are facts about the route rather
// than claims the model made: a reader checking why the draft says what it says
// is owed the same answer whichever writer produced the prose.
func introReasons(facts introFacts) []crmcontracts.AccountDraftReason {
	out := []crmcontracts.AccountDraftReason{{
		Kind:  crmcontracts.AccountDraftReasonKindRelationship,
		Label: fmt.Sprintf("%s knows %s (%s)", facts.colleague, facts.contact, facts.band),
	}}
	if facts.deal != "" {
		out = append(out, crmcontracts.AccountDraftReason{
			Kind:  crmcontracts.AccountDraftReasonKindDeal,
			Label: facts.deal,
		})
	}
	return out
}

// firstName is the greeting's name, which for a colleague is the given one.
//
// A colleague is addressed the way a colleague is addressed. "Dear Ms Meier"
// to somebody two desks away reads as a form letter, which is the one thing an
// ask for a favour must not.
func firstName(full string) string {
	fields := strings.Fields(full)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// introDisclosure is the AI-authorship line, in the draft's own language.
func introDisclosure(lang textlang.Lang) string {
	switch lang {
	case textlang.German:
		return "Diese Nachricht wurde mit KI-Unterstützung verfasst."
	case textlang.Vietnamese:
		return "Tin nhắn này được soạn với sự hỗ trợ của AI."
	default:
		return "This message was drafted with AI assistance."
	}
}

// correspondenceOf folds a contact's own messages into the text a language
// detector reads.
//
// Subjects and bodies together, newest first, which is what the person-side
// draft folds for the same question. A detector fed record names instead
// answers Unknown for almost every account, and the ask goes out in the wrong
// language to a colleague who reads the right one every day.
func correspondenceOf(said []proposeroles.Message) string {
	var text strings.Builder
	for _, message := range said {
		text.WriteString(message.Subject + "\n" + message.Body + "\n\n")
	}
	return text.String()
}
