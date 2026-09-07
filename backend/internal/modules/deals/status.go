// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import "github.com/margince/margince/backend/internal/shared/kernel/values"

// DealStatus and StageSemantic are the deal lifecycle vocabulary — the
// Go spelling of the deal_status and stage semantic CHECKs (0006), kept
// in sync by the enumsync fitness gate. Domain logic branches on these
// constants, never on raw literals. The two sets share values by design
// (a stage's semantic derives the deal's status), but they are distinct
// vocabularies: a stage is never "open-ish", a deal never "a column".
type DealStatus string

const (
	DealOpen DealStatus = "open"
	DealWon  DealStatus = "won"
	DealLost DealStatus = "lost"
)

// One key keeps a stage refusal and the audit image of a stage write naming
// the same field, so the client reading the error and the auditor reading the
// trail see one name rather than two spellings of it.
const stageSemanticField = "semantic"

type StageSemantic string

const (
	SemanticOpen StageSemantic = "open"
	SemanticWon  StageSemantic = "won"
	SemanticLost StageSemantic = "lost"
)

// Terminal reports whether a stage closes the deal.
func (s StageSemantic) Terminal() bool { return s == SemanticWon || s == SemanticLost }

// ParseStageSemantic is the config seam's membership check (pipeline
// and stage editing take the semantic from the client).
func ParseStageSemantic(raw string) (StageSemantic, error) {
	switch s := StageSemantic(raw); s {
	case SemanticOpen, SemanticWon, SemanticLost:
		return s, nil
	}
	return "", &values.ParseError{
		Field: stageSemanticField, Code: "invalid_stage_semantic",
		Message: "semantic is one of open, won, lost",
	}
}

// Offer status needs no local vocabulary: the generated contract enum
// (crmcontracts.OfferStatus + its constants) is the source of truth the
// stores compare against.

// ProposalState is the offer line's approval vocabulary — the Go
// spelling of the offer_line_item.proposal_state CHECK (0059). A staged
// line is an AI-drafted proposal awaiting human acceptance (E03.21a):
// it never contributes to the server-computed offer totals, so a draft
// can never move a number the buyer sees. Not exposed on the contract —
// the accept transition is store-internal until a drafting surface ships.
type ProposalState string

const (
	ProposalStaged   ProposalState = "staged"
	ProposalAccepted ProposalState = "accepted"
)

// CriterionKind is the Go spelling of the stage_exit_criterion.kind CHECK,
// kept in sync by the enumsync fitness gate. It is not decoration: the kind
// decides which evidence sources may settle a criterion, so branching on a
// raw literal here is how a buyer milestone quietly accepts the seller's own
// mail as proof of itself.
//
// The buyer-milestone kinds are the ones a seller cannot assert on the
// buyer's behalf. BuyerConfirmed, EventHeld, DocumentSigned and TermsAccepted
// each name a thing the OTHER side did; RoleIdentified and Custom do not.
type CriterionKind string

// The criterion kinds, mirroring the stage_exit_criterion.kind CHECK.
const (
	CriterionBuyerConfirmed CriterionKind = "buyer_confirmed"
	CriterionEventHeld      CriterionKind = "event_held"
	CriterionDocumentSigned CriterionKind = "document_signed"
	CriterionRoleIdentified CriterionKind = "role_identified"
	CriterionTermsAccepted  CriterionKind = "terms_accepted"
	CriterionCustom         CriterionKind = "custom"
)

// criterionKindField keeps the refusal and the audit image of a criterion
// write naming one field, as stageSemanticField does for a stage.
const criterionKindField = "kind"

// BuyerMilestone reports whether this kind is settled by WHO SPOKE.
//
// The two that are can only be settled by buyer-authored evidence: a message
// our own side wrote saying the buyer confirmed something is our claim about
// them, not their confirmation.
//
// The other four are NOT weaker bars — they are different questions, and
// authorship is the wrong test for each:
//
//   - EventHeld and DocumentSigned are settled by a RECORDED FACT rather than
//     by anybody's word. A meeting either took place or it did not, and both
//     sides sign a contract. Testing them by authorship made event_held
//     unsettleable in practice, because capture stamps our own seat as the
//     sender of every meeting it syncs: the criterion could never be met by a
//     real record. MeetingWasHeld is what judges those, on the record's own
//     evidence.
//   - RoleIdentified is ours to observe — we learn who the economic buyer is
//     from our own notes as readily as from theirs.
//   - Custom carries no promise about who does the thing at all.
//
// A method rather than a list at the call site, so a kind added to the enum
// has to answer this question where it is declared.
func (k CriterionKind) BuyerMilestone() bool {
	switch k {
	case CriterionBuyerConfirmed, CriterionTermsAccepted:
		return true
	case CriterionEventHeld, CriterionDocumentSigned,
		CriterionRoleIdentified, CriterionCustom:
		return false
	}
	return false
}

// ParseCriterionKind is the config seam's membership check.
func ParseCriterionKind(raw string) (CriterionKind, error) {
	switch k := CriterionKind(raw); k {
	case CriterionBuyerConfirmed, CriterionEventHeld, CriterionDocumentSigned,
		CriterionRoleIdentified, CriterionTermsAccepted, CriterionCustom:
		return k, nil
	}
	return "", &values.ParseError{
		Field: criterionKindField, Code: "invalid_criterion_kind",
		Message: "kind is one of buyer_confirmed, event_held, document_signed, role_identified, terms_accepted, custom",
	}
}
