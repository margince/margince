/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import {
  jsonResponse,
  type RouteMap,
  stubWithSession,
} from "../../../testing/story-utils";
import { CompanyAct } from "./company-act";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// A refused company save speaks in every view that can confirm.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type Proposal = components["schemas"]["OnboardingCompanyProposal"];
type ColdField = components["schemas"]["ColdStartField"];

const SITE_URL = "https://gradion.com";
const READ_ID = "018f3a1b-0000-7000-8000-0000000000e2";
const CONFIRM_PATH = `POST /company/site-reads/${READ_ID}/confirm`;

function grounded(field: ColdField["field"], value: string): ColdField {
  return {
    field,
    value,
    evidence_snippet: "seen on the site",
    source_kind: "url",
    source_url: SITE_URL,
    confidence: 0.95,
  };
}

// The three fields `confirmCompanySiteRead` 422s without. Filled, so nothing
// but the server's own refusal can hold Continue down.
const REQUIRED_TRIO: ColdField[] = [
  grounded("display_name", "Gradion"),
  grounded("offer_summary", "CRM software"),
  grounded("icp", "Mid-market B2B"),
];

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
  pages_read: 1,
  pages: [{ url: SITE_URL, status: "fetched", kind: "home" }],
  profile_fields: REQUIRED_TRIO,
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

function proposal(hash: string): Proposal {
  return {
    ready: true,
    fields: REQUIRED_TRIO.map((field) => ({
      field: field.field,
      value: field.value,
      confidence: field.confidence,
      evidence_snippet: field.evidence_snippet ?? "",
      source_url: field.source_url ?? SITE_URL,
    })),
    facts: [],
    open_questions: [],
    remaining_required_fields: [],
    draft_version: READ.draft_version,
    proposal_hash: hash,
  };
}

const REVIEW_STATE: ConversationState = {
  ...initialConversationState,
  act: "company",
  phase: "co.review",
  activeReadId: READ_ID,
  readCompleted: true,
  pendingQuestion: null,
};

function renderReview(routes: RouteMap): void {
  // Routes GET /me: an unrouted probe answers 501 and every grant fails closed.
  stubWithSession(
    {
      [`GET /company/site-reads/${READ_ID}`]: () => jsonResponse(READ),
      "GET /onboarding/company/proposal": () =>
        jsonResponse(proposal("proposal-1")),
      ...routes,
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
          state={REVIEW_STATE}
          dispatch={vi.fn()}
          profile={null}
          persist={vi.fn(async () => true)}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const DETAIL = "Company name must not be blank.";
const REFUSED: RouteMap = {
  [CONFIRM_PATH]: () =>
    jsonResponse(
      {
        title: "Validation failed",
        detail: DETAIL,
        code: "validation_error",
        status: 422,
      },
      422,
    ),
};

function refusalLine(): HTMLElement | null {
  return (
    screen
      .queryAllByRole("alert")
      .find((node) => node.textContent?.includes(DETAIL)) ?? null
  );
}

async function pressConfirm(): Promise<void> {
  const confirm = await screen.findByRole("button", {
    name: en["ob.deck.confirm"],
  });
  await waitFor(() => expect(confirm).toBeEnabled());
  fireEvent.click(confirm);
}

async function openWholeRecord(): Promise<void> {
  fireEvent.click(
    await screen.findByRole("button", { name: en["ob.deck.readWhole"] }),
  );
  await screen.findByRole("button", { name: en["ob.deck.backToOpen"] });
}

async function editCompanyName(): Promise<void> {
  fireEvent.click(
    await screen.findByRole("button", {
      name: en["ob.digest.editLine"].replace("{label}", "Company name"),
    }),
  );
  const control = await screen.findByRole("textbox");
  fireEvent.change(control, { target: { value: "Edited value" } });
  fireEvent.keyDown(control, { key: "Enter" });
}

async function expectRefusalLine(): Promise<void> {
  await waitFor(() => expect(refusalLine()).not.toBeNull());
  expect(screen.getAllByText(new RegExp(DETAIL))).toHaveLength(1);
}

it("speaks a refused save in the same foot as Confirm in the deck view", async () => {
  renderReview(REFUSED);
  expect(refusalLine()).toBeNull();

  await pressConfirm();

  await expectRefusalLine();
  const confirm = screen.getByRole("button", { name: en["ob.deck.confirm"] });
  const foot = confirm.closest(".ob-stage-acts");
  expect(foot).not.toBeNull();
  expect(refusalLine()?.closest(".ob-stage-acts")).toBe(foot);
});

it("keeps the refusal in the whole-record view", async () => {
  renderReview(REFUSED);
  await pressConfirm();
  await expectRefusalLine();

  fireEvent.click(
    await screen.findByRole("button", { name: en["ob.deck.readWhole"] }),
  );

  await screen.findByRole("button", { name: en["ob.deck.backToOpen"] });
  await expectRefusalLine();
});

it("keeps the refusal in the fact-board view", async () => {
  renderReview(REFUSED);
  await pressConfirm();
  await expectRefusalLine();

  fireEvent.click(
    await screen.findByRole("button", { name: en["ob.deck.readWhole"] }),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: en["ob.digest.pickFacts"] }),
  );

  await screen.findByRole("button", { name: en["ob.deck.backToRecord"] });
  await expectRefusalLine();
});

it("renders no refusal line before a save is refused", async () => {
  renderReview(REFUSED);
  await screen.findByRole("button", { name: en["ob.deck.confirm"] });

  expect(refusalLine()).toBeNull();
});

it("keeps one alert, on the same node, when the first edit adds Save", async () => {
  renderReview(REFUSED);
  await pressConfirm();
  await expectRefusalLine();
  await openWholeRecord();
  const before = refusalLine();

  await editCompanyName();

  await screen.findByRole("button", { name: en["ob.digest.saveChanges"] });
  await expectRefusalLine();
  expect(refusalLine()).toBe(before);
}, 30_000);

it("joins the foot of an already shown Save", async () => {
  renderReview(REFUSED);
  await openWholeRecord();
  await editCompanyName();
  const save = await screen.findByRole("button", {
    name: en["ob.digest.saveChanges"],
  });
  expect(refusalLine()).toBeNull();

  fireEvent.click(save);

  await expectRefusalLine();
  const foot = save.closest(".ob-stage-acts");
  expect(foot).not.toBeNull();
  expect(foot?.querySelectorAll('[role="alert"]')).toHaveLength(1);
}, 30_000);
