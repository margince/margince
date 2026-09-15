/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { jsonResponse, stubWithSession } from "../story-utils";
import { CompanyAct } from "./company-act";
import type {
  ConversationQuestion,
  ConversationState,
} from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// What a legal-entity candidate card tells a reader who has to pick between
// two names that read almost alike. The card has one slot for a number: the
// registry identity when the notice printed one, and the tax identity when it
// printed only that — a candidate is never a bare name merely because its
// notice carried a VAT ID instead of a register entry. company-act.test.tsx
// pins where the decision renders; this file pins what each card says.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type CompanySiteRead = components["schemas"]["CompanySiteRead"];

const SITE_URL = "https://gradion.com";
const LEGAL_NOTICE = "https://gradion.com/legal-notice";
const READ_ID = "018f3a1b-0000-7000-8000-0000000000e4";

// Registered with both identities, so the card has to choose one.
const REGISTERED = {
  name: "Gradion GmbH",
  registered_address: "Friedrichstraße 68, 10117 Berlin",
  register_number: "HRB 201934 B",
  vat_number: "DE318447291",
  source_url: LEGAL_NOTICE,
};

// A notice that printed a tax identity and nothing else.
const VAT_ONLY = {
  name: "Gradion Holding GmbH",
  registered_address: "Kaiserstraße 12, 60311 Frankfurt am Main",
  vat_number: "DE299110482",
  source_url: LEGAL_NOTICE,
};

const READ: CompanySiteRead = {
  id: READ_ID,
  target_kind: "onboarding",
  company_id: null,
  root_url: SITE_URL,
  status: "ready",
  status_code: null,
  status_detail: null,
  next_attempt_at: null,
  phase: null,
  pages_read: 2,
  pages: [{ url: LEGAL_NOTICE, status: "fetched", kind: "impressum" }],
  profile_fields: [],
  facts: [],
  comparisons: [],
  contacts: [],
  legal_entities: [REGISTERED, VAT_ONLY],
  warnings: [],
  draft_version: 1,
  proposal_hash: "proposal-1",
  created_at: "2026-07-22T08:00:00Z",
  updated_at: "2026-07-22T08:00:01Z",
};

const ENTITY_QUESTION: ConversationQuestion = {
  id: "clarify:legal_name:1",
  i18nKey: "ob.conv.clarify.question",
  params: {
    question:
      "The legal notice names more than one legal entity. Which one is your company?",
  },
  dismissLabelKey: "ob.conv.clarify.dismiss",
  options: [
    { value: REGISTERED.name, label: REGISTERED.name },
    { value: VAT_ONLY.name, label: VAT_ONLY.name },
  ],
};

const CLARIFY_STATE: ConversationState = {
  ...initialConversationState,
  act: "company",
  phase: "co.clarify",
  activeReadId: READ_ID,
  readCompleted: true,
  pendingQuestion: ENTITY_QUESTION,
};

function renderClarify(): void {
  // The session probe is routed because this act mounts capability-aware
  // chrome and story-utils refuses to guess a session. The grants are empty on
  // purpose: this is onboarding, before any of them are held.
  stubWithSession(
    {
      [`GET /company/site-reads/${READ_ID}`]: () => jsonResponse(READ),
      "GET /onboarding/company/proposal": () =>
        jsonResponse({ title: "not ready", code: "not_found" }, 404),
    },
    {},
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyAct
          state={CLARIFY_STATE}
          dispatch={vi.fn()}
          profile={null}
          persist={vi.fn(async () => true)}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The candidate's own card, found through the name it leads with, so every
// fact asserted below is proven to sit under THAT name rather than anywhere on
// the surface.
function cardOf(name: string): HTMLElement {
  const card = screen.getByText(name).closest("label");
  if (card === null) {
    throw new Error(`no candidate card leads with "${name}"`);
  }
  return card;
}

it("shows a registered candidate's register number, not its VAT ID", async () => {
  renderClarify();

  await screen.findByText(REGISTERED.register_number);
  const card = cardOf(REGISTERED.name);
  expect(within(card).getByText(REGISTERED.register_number)).toBeVisible();
  expect(within(card).getByText(REGISTERED.registered_address)).toBeVisible();
  expect(within(card).queryByText(REGISTERED.vat_number)).toBeNull();
});

it("falls back to the VAT ID for a candidate whose notice printed no register number", async () => {
  renderClarify();

  await screen.findByText(VAT_ONLY.vat_number);
  const card = cardOf(VAT_ONLY.name);
  expect(within(card).getByText(VAT_ONLY.vat_number)).toBeVisible();
  expect(within(card).getByText(VAT_ONLY.registered_address)).toBeVisible();
});
