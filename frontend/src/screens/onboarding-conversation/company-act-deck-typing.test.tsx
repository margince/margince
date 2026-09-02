/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import type { CompanyFieldName } from "../onboarding";
import { installFetchStub, jsonResponse, type RouteMap } from "../story-utils";
import { CompanyAct } from "./company-act";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// The deck's own place in the queue must be a FIELD, not a position: `cards`
// carries only what is still outstanding, so a field leaves that array the
// moment its first character lands, and a cursor indexing into it would slide
// the next question in under the caret mid-word. This file pins the deck
// through exactly that moment, on the surface a reader actually types into.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type Proposal = components["schemas"]["OnboardingCompanyProposal"];
type ProposalField = components["schemas"]["OnboardingCompanyProposalField"];
type ColdField = components["schemas"]["ColdStartField"];

const SITE_URL = "https://gradion.com";
const READ_ID = "018f3a1b-0000-7000-8000-0000000000e3";

function grounded(field: ColdField["field"], value: string): ColdField {
  return {
    field,
    value,
    evidence_snippet: "seen on the site",
    source_kind: "url",
    source_url: SITE_URL,
    confidence: 0.9,
  };
}

// Every field the board asks about, filled and high-confidence, except the
// two the test cares about: `display_name` and `icp`, both required, and both
// left with no value in the draft. Every other required field (`offer_summary`)
// is filled, so exactly these two are the outstanding cards, and `display_name`
// leads the board's own field order (`reviewFields()` walks the legal-identity
// group first) — including on the very first commit, before the site-read's
// prefill has even landed, since it is blank there too. The deck's order-ref
// fixes a field's place in the queue the first time it appears, so a fixture
// that only reaches its final blank set AFTER a render would fix the queue on
// a different field than the one this test means to type into.
const FIELDS: readonly CompanyFieldName[] = [
  "legal_name",
  "registered_address",
  "register_vat",
  "legal_form",
  "register_court",
  "register_number",
  "industry",
  "history",
  "offer_summary",
  "value_proposition",
  "usp",
  "buying_center",
  "customer_pains",
  "desired_outcomes",
  "buying_intents",
  "common_objections",
  "sales_motion",
];

const PROFILE_FIELDS: readonly ColdField[] = FIELDS.map((field) =>
  grounded(field, `${field} on record`),
);

const READ: CompanySiteRead = {
  id: READ_ID,
  target_kind: "onboarding",
  organization_id: null,
  root_url: SITE_URL,
  status: "ready",
  status_code: null,
  status_detail: null,
  next_attempt_at: null,
  phase: null,
  pages_read: 1,
  pages: [{ url: SITE_URL, status: "fetched", kind: "home" }],
  profile_fields: PROFILE_FIELDS,
  facts: [],
  comparisons: [],
  people: [],
  legal_entities: [],
  warnings: [],
  draft_version: 1,
  proposal_hash: "proposal-1",
  created_at: "2026-07-22T08:00:00Z",
  updated_at: "2026-07-22T08:00:01Z",
};

const PROPOSAL: Proposal = {
  ready: true,
  fields: PROFILE_FIELDS.map(
    (field): ProposalField => ({
      field: field.field,
      value: field.value,
      confidence: field.confidence,
      evidence_snippet: field.evidence_snippet ?? "",
      source_url: field.source_url ?? SITE_URL,
    }),
  ),
  facts: [],
  open_questions: [],
  remaining_required_fields: [],
  draft_version: READ.draft_version,
  proposal_hash: READ.proposal_hash,
};

const REVIEW_STATE: ConversationState = {
  ...initialConversationState,
  act: "company",
  phase: "co.review",
  activeReadId: READ_ID,
  readCompleted: true,
  pendingQuestion: null,
};

function renderReview(): void {
  const routes: RouteMap = {
    [`GET /company/site-reads/${READ_ID}`]: () => jsonResponse(READ),
    "GET /onboarding/company/proposal": () => jsonResponse(PROPOSAL),
  };
  installFetchStub(routes);
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyAct
          state={REVIEW_STATE}
          dispatch={vi.fn()}
          profile={null}
          persist={vi.fn(async () => true)}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The one card the deck shows at a time — its label repeats in the digest
// beside it, so every query below is scoped to this container rather than to
// the label text alone.
function card(): HTMLElement {
  const found = document.querySelector<HTMLElement>(".rdeck-card");
  if (found === null) {
    throw new Error("the review deck rendered no .rdeck-card");
  }
  return found;
}

it("keeps the deck on the card being typed into, past the first character that drops it off the outstanding list", async () => {
  const user = userEvent.setup();
  renderReview();

  // `display_name` leads the board's own field order, so the deck opens on
  // it: a required question, and one of the two the reader has left to
  // answer. The tray's own "2 of 2 left" is the outstanding count settling —
  // the site read prefills the draft on its own query, separate from the
  // proposal that opens the deck.
  await screen.findByText("2 of 2 left");
  const question = within(card()).getByText("Company name");
  const counter = within(card()).getByText(/^1 of \d+$/).textContent;

  const control = within(card()).getByRole("textbox", {
    name: "Company name",
  });
  expect(control).toHaveValue("");

  // The first character is the one the old positional cursor never
  // survived: it is what drops `display_name` out of the outstanding list,
  // mid-keystroke.
  await user.type(control, "G");

  expect(within(card()).getByText("Company name")).toBe(question);
  expect(within(card()).getByText(counter as string)).toBeInTheDocument();
  expect(control).toHaveValue("G");

  // Past the first character, so the rest of the name lands too.
  await user.type(control, "radion");

  expect(within(card()).getByText("Company name")).toBe(question);
  expect(within(card()).getByText(counter as string)).toBeInTheDocument();
  expect(control).toHaveValue("Gradion");
});
