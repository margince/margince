/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../../i18n";
import {
  jsonResponse,
  type RouteMap,
  stubWithSession,
} from "../../../testing/story-utils";
import { CompanyAct } from "./company-act";
import {
  type CompanySiteRead,
  type FieldFixture,
  type Proposal,
  proposedField,
  REVIEW_READ,
  REVIEW_READ_ID,
  REVIEW_STATE,
  reviewProposal,
  toColdField,
} from "./company-act.testkit";
import type {
  ConversationQuestion,
  ConversationState,
} from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// The rail's to-do list during co.review must name what the review board
// counts as outstanding, no more and no fewer. Two surfaces read one draft,
// so they must share one idea of "needs attention".

// A live decision owns the whole work surface (DecisionScene): the rail
// beside it is a narrator, never a second copy of the same choice. This is
// the one invariant this suite guards — every way a "question" thread entry
// can reach the rail is swept here, not just the machine's current
// pendingQuestion.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows a live legal-entity decision on the surface, never as a QuestionCard in the rail", () => {
  const live = entityQuestion("clarify:legal_name:3");
  renderCompanyAct({
    ...initialConversationState,
    act: "company",
    phase: "co.clarify",
    activeReadId: null,
    readCompleted: true,
    pendingQuestion: live,
    thread: [
      { kind: "question", id: "question:clarify:legal_name:3", question: live },
    ],
    seq: 1,
  });

  // The scene renders no heading of its own — the room's h1 IS the
  // question — and one radio per candidate.
  expect(
    screen.getByRole("heading", { level: 1, name: /legal entity/ }),
  ).toBeInTheDocument();
  expect(
    screen.getAllByRole("radio", { name: "Gradion Holding GmbH" }),
  ).toHaveLength(1);
});

it("keeps a superseded, never-answered re-ask out of the rail once a fresh one takes over", () => {
  // The server re-issues a clarify with a new id across a background poll
  // (a new draft version); the machine appends the new question without
  // retiring the old thread entry, which is exactly the entry that must
  // never render as a second, disabled copy of the same candidate list.
  const stale = entityQuestion("clarify:legal_name:2");
  const live = entityQuestion("clarify:legal_name:3");
  renderCompanyAct({
    ...initialConversationState,
    act: "company",
    phase: "co.clarify",
    activeReadId: null,
    readCompleted: true,
    pendingQuestion: live,
    thread: [
      {
        kind: "question",
        id: "question:clarify:legal_name:2",
        question: stale,
      },
      {
        kind: "question",
        id: "question:clarify:legal_name:3",
        question: live,
      },
    ],
    seq: 2,
  });

  expect(screen.getAllByRole("radio", { name: "Gradion GmbH" })).toHaveLength(
    1,
  );
  // The stale re-ask's own candidate list must not survive as a second copy,
  // answered or not — its answer can never be recorded, so an inert copy
  // would be a dead end that looks exactly like the live one.
  expect(
    screen.queryAllByRole("button", { name: "Gradion Holding GmbH" }),
  ).toHaveLength(0);
});

it("carries no chat-style composer during manual entry — typed fields are the interview's own", () => {
  renderCompanyAct({
    ...initialConversationState,
    act: "company",
    phase: "co.manual",
    activeReadId: null,
    readCompleted: false,
  });

  // The manual form's own fields are real textboxes, asked one at a time —
  // never a free-text message composed and sent.
  expect(screen.queryAllByRole("textbox").length).toBeGreaterThan(0);
  expect(screen.queryByRole("button", { name: /^send\b/i })).toBeNull();
});

function entityQuestion(id: string): ConversationQuestion {
  return {
    id,
    i18nKey: "ob.conv.clarify.question",
    params: {
      question:
        "The legal notice names more than one legal entity. Which one is your company?",
    },
    dismissLabelKey: "ob.conv.clarify.dismiss",
    options: [
      { value: "Gradion GmbH", label: "Gradion GmbH" },
      { value: "Gradion Holding GmbH", label: "Gradion Holding GmbH" },
    ],
  };
}

// Every stub in this file routes the session probe, because this act mounts
// capability-aware chrome and story-utils refuses to guess a session. An
// unrouted GET /me answers 501 and every grant fails closed. The surface then
// draws a branch no assertion here is about, late enough under load to change
// the verdict. The grants are empty because this is onboarding, before any of
// them are held.

function renderCompanyAct(state: ConversationState) {
  stubWithSession({}, {});
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyAct
          state={state}
          dispatch={vi.fn()}
          profile={null}
          persist={vi.fn(async () => true)}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// One field left high-confidence or human-typed in every group (so each
// section has a settled row too), the rest spread across the states the
// rail must fold into its two buckets: no value (required or optional) and
// a weak-confidence value worth a second look.
const REVIEW_FIELDS: readonly FieldFixture[] = [
  proposedField("display_name", "Gradion", 0.95),
  proposedField("industry", "B2B software", 0.6),
  proposedField("history", "Founded 2019", 0.9),
  // legal_name, registered_address, register_vat: left blank on purpose.
  // legal_form and register_court are grounded so the identity section keeps
  // exactly NAV_NAMED_LIMIT outstanding rows: past that the nav stops naming
  // them one by one and falls back to an overflow count, and this case is
  // about the two surfaces agreeing field for field.
  proposedField("legal_form", "GmbH", 0.9),
  proposedField("register_court", "Amtsgericht Charlottenburg", 0.9),
  // register_number: left blank on purpose, like the trio above it.
  proposedField("value_proposition", "Faster onboarding", 0.9),
  proposedField("usp", "AI-native from day one", 0.9),
  // offer_summary: left blank on purpose (also required).
  proposedField("customer_pains", "Manual onboarding takes weeks", 0.9),
  proposedField("buying_center", "Ops and RevOps leads", 0.4),
  // icp: left blank on purpose (also required); desired_outcomes too.
  proposedField("buying_intents", "Evaluating CRM replacements", 0.9),
  proposedField("common_objections", "Migration risk", 0.9),
  proposedField("sales_motion", "Sales-assisted", 0.9),
];

// The read's own profile_fields is what actually prefills the draft
// (`useCompanyRead`'s `handleSnapshot` → `prefill`); the proposal endpoint
// only supplies confidence and evidence for a value the draft already
// carries. A row only reads as filled if BOTH agree, so every scenario below
// builds the read's snapshot from the exact same field list as the proposal.
function reviewRoutes(
  fields: readonly FieldFixture[],
  proposal: Proposal,
): RouteMap {
  const read: CompanySiteRead = {
    ...REVIEW_READ,
    profile_fields: fields.map(toColdField),
  };
  return {
    [`GET /company/site-reads/${REVIEW_READ_ID}`]: () => jsonResponse(read),
    "GET /onboarding/company/proposal": () => jsonResponse(proposal),
  };
}

// The dossier's entity cards and the clarify's candidate list ask the same
// question of the same candidates, so a pick has to settle the same way on
// both: the chosen name wins over a name typed earlier, because that name
// standing above this candidate's address and registration number would put
// two companies on one card. The bug this guards was the dossier keeping the
// typed name while silently taking the rest of the block from the candidate.
describe("the dossier's legal-entity picker", () => {
  const GRADION_LTD = {
    name: "Gradion Co., Ltd.",
    registered_address: "Level 12, Bitexco Tower, Ho Chi Minh City",
    register_number: "0318 447 291",
    evidence_snippet: "Gradion Co., Ltd. · 0318 447 291",
    source_url: "https://gradion.com/legal-notice",
  };

  it("settles the chosen name over one the human typed earlier", async () => {
    // A read that reached the AI budget ceiling keeps the evidence it already
    // collected but never becomes confirmable, so no review scene takes the
    // surface — the dossier stays, with its "edit fields directly" escape
    // hatch and the entity cards behind it.
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () =>
          jsonResponse({
            ...REVIEW_READ,
            status: "deferred",
            status_code: "budget_deferred",
            legal_entities: [
              GRADION_LTD,
              {
                name: "Gradion Holding GmbH",
                source_url: "https://gradion.com/legal-notice",
              },
            ],
          }),
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
            state={REVIEW_STATE}
            dispatch={vi.fn()}
            profile={null}
            persist={vi.fn(async () => true)}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    fireEvent.click(
      await screen.findByRole("button", { name: "Edit fields directly" }),
    );
    const legalName = screen.getByLabelText(/Registered legal name/);
    fireEvent.change(legalName, { target: { value: "Gradion, roughly" } });

    const card = screen.getByRole("radio", { name: /Gradion Co\., Ltd\./ });
    fireEvent.click(card);

    expect(legalName).toHaveValue("Gradion Co., Ltd.");
    // The picker marks a plate chosen by comparing it to legal_name, so a pick
    // that left the typed name standing also denied the very click it had just
    // honoured everywhere else.
    expect(card).toBeChecked();
    expect(screen.getByLabelText(/Registered address/)).toHaveValue(
      GRADION_LTD.registered_address,
    );
  });
});

// Arrival is not an action the reader took: landing on co.review must show
// the scene from its own top, not wherever a leftover crawl narration last
// pointed. The bug this guards was CompanyActArtifact's highlight effect
// pulsing and scrolling to whatever field the LAST thread entry named, even
// when that entry predates the review scene entirely — a stale finding from
// the read phase, not anything that happened while the review was on screen.
describe("arriving at the review scene", () => {
  it("leaves the board unscrolled and unfocused when a field-naming entry is already the thread's last one by the time the review's own data resolves", async () => {
    // jsdom has no scrollIntoView; the real DOM always carries one.
    Element.prototype.scrollIntoView ??= () => {};
    const scrollSpy = vi
      .spyOn(Element.prototype, "scrollIntoView")
      .mockImplementation(() => {});
    stubWithSession(
      reviewRoutes(REVIEW_FIELDS, reviewProposal(REVIEW_FIELDS)),
      {},
    );
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const tree = (state: ConversationState) => (
      <QueryClientProvider client={queryClient}>
        <LocaleProvider initial="en">
          <CompanyAct
            state={state}
            dispatch={vi.fn()}
            profile={null}
            persist={vi.fn(async () => true)}
          />
        </LocaleProvider>
      </QueryClientProvider>
    );
    // First render carries the field-naming narration already — the site
    // read and proposal queries are still in flight, so this first commit
    // never finds the row: the effect that pulses/scrolls fires once here,
    // matching nothing.
    const findingEntry = {
      kind: "narration" as const,
      id: "3:field:display_name",
      i18nKey: "ob.conv.read.learnedField" as const,
      findingIds: ["display_name"],
    };
    const { rerender } = render(
      tree({ ...REVIEW_STATE, thread: [findingEntry] }),
    );
    // The finding-highlight effect targets `[data-finding-id]`, which only
    // the editing wall carries: the deck's own digest, and the article the
    // whole record is read as, both state the record as prose with no
    // per-field DOM hook to pulse. The deck is the review's default face and
    // reading is not editing, so the wall is two doors away, and this test
    // walks the same two a reader would.
    fireEvent.click(
      await screen.findByRole("button", { name: "Read full profile" }),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Choose facts to keep" }),
    );
    await screen.findByRole("heading", { level: 2, name: /Correct anything/ });

    // A background poll narrates again, live, while the review is already
    // on screen with the row now actually mounted — a fresh thread array
    // that still ends on a field-naming entry, exactly the shape a
    // narrating background poll produces mid-review.
    rerender(
      tree({
        ...REVIEW_STATE,
        thread: [findingEntry, { ...findingEntry, id: "4:field:display_name" }],
      }),
    );

    expect(document.querySelectorAll(".ob-conv-pulse")).toHaveLength(0);
    expect(document.activeElement === document.body).toBe(true);
    const row = document.getElementById("ob-triage-row-display_name");
    expect(row).not.toBeNull();
    // The thread's own follow-the-bottom behaviour is a separate, legitimate
    // scroll target; the review board's rows are never among its targets.
    const scrolled: readonly unknown[] = scrollSpy.mock.instances;
    expect(scrolled.some((instance) => instance === row)).toBe(false);
  });
});
