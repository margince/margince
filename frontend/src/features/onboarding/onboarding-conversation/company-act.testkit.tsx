import type { components } from "../../../api/schema";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

export type CompanySiteRead = components["schemas"]["CompanySiteRead"];

export type Proposal = components["schemas"]["OnboardingCompanyProposal"];

export type ProposalField =
  components["schemas"]["OnboardingCompanyProposalField"];

export type ColdField = components["schemas"]["ColdStartField"];

export const REVIEW_READ_ID = "018f3a1b-0000-7000-8000-0000000000e1";

// ProposalField's `field` is a bare string (the proposal endpoint names any
// field), but ColdStartField's is a closed literal union. A fixture for both
// shapes starts from the narrow one and widens, never the other way.
export type FieldFixture = Readonly<{
  field: ColdField["field"];
  value: string;
  confidence: number;
}>;

export function proposedField(
  field: ColdField["field"],
  value: string,
  confidence: number,
): FieldFixture {
  return { field, value, confidence };
}

export function toProposalField(fixture: FieldFixture): ProposalField {
  return {
    field: fixture.field,
    value: fixture.value,
    confidence: fixture.confidence,
    evidence_snippet: "seen on the site",
    source_url: "https://gradion.com",
  };
}

export function toColdField(fixture: FieldFixture): ColdField {
  return {
    field: fixture.field,
    value: fixture.value,
    evidence_snippet: "seen on the site",
    source_kind: "url",
    source_url: "https://gradion.com",
    confidence: fixture.confidence,
  };
}

export const REVIEW_READ: CompanySiteRead = {
  id: REVIEW_READ_ID,
  target_kind: "onboarding",
  company_id: null,
  root_url: "https://gradion.com",
  status: "ready",
  status_code: null,
  status_detail: null,
  next_attempt_at: null,
  phase: null,
  pages_read: 3,
  pages: [{ url: "https://gradion.com", status: "fetched", kind: "home" }],
  profile_fields: [],
  facts: [],
  comparisons: [],
  contacts: [],
  legal_entities: [],
  warnings: [],
  draft_version: 1,
  proposal_hash: "proposal-1",
  created_at: "2026-07-22T08:00:00Z",
  updated_at: "2026-07-22T08:00:01Z",
};

export function reviewProposal(
  fields: readonly FieldFixture[],
  openQuestions: Proposal["open_questions"] = [],
): Proposal {
  return {
    ready: true,
    fields: fields.map(toProposalField),
    facts: [],
    open_questions: openQuestions,
    remaining_required_fields: [],
    draft_version: REVIEW_READ.draft_version,
    proposal_hash: REVIEW_READ.proposal_hash,
  };
}

export const REVIEW_STATE: ConversationState = {
  ...initialConversationState,
  act: "company",
  phase: "co.review",
  activeReadId: REVIEW_READ_ID,
  readCompleted: true,
  pendingQuestion: null,
};
