/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { DealScreen } from "./deals";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// WHY THE DEAL'S STAGE LADDER REFUSES, on the reader's screen.
//
// It was the one refused control on the deal page that said nothing: every rung
// disabled, none of them naming a reason. A reader could not tell "you may not
// change this deal" from a page that failed to load, and a screen reader —
// which cannot focus a disabled button and is told nothing by a `title` —
// heard a disabled button and no more. Every other refused control on the same
// page already carried its sentence.
//
// Three causes reach the ladder as one `advanceRefused`, and they do not share
// an answer, so each is asked here for the sentence it actually gives.

type Deal = components["schemas"]["Deal"];

const STAGES = [
  {
    id: "s1",
    pipeline_id: "p1",
    name: "Qualify",
    position: 1,
    semantic: "open",
  },
  {
    id: "s2",
    pipeline_id: "p1",
    name: "Proposal",
    position: 2,
    semantic: "open",
  },
  { id: "s3", pipeline_id: "p1", name: "Won", position: 3, semantic: "won" },
];

function deal(over: Partial<Deal> = {}): Deal {
  return {
    id: "d1",
    name: "Fleet retrofit",
    status: "open",
    writable: true,
    version: 4,
    pipeline_id: "p1",
    stage_id: "s1",
    currency: "EUR",
    source: "manual",
    captured_by: "human:u",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  } as Deal;
}

function mount(current: Deal) {
  installFetchStub({
    "GET /me": meRoute({ deal: ["read", "update"] }, { seat: "full" }),
    "GET /deals/d1": () => jsonResponse(current),
    "GET /pipelines": () =>
      jsonResponse({
        data: [
          {
            id: "p1",
            name: "Sales",
            is_default: true,
            position: 0,
            stages: STAGES,
          },
        ],
        page: { next_cursor: null },
      }),
  });
  return render(
    <StoryProviders>
      <DealScreen id="d1" />
    </StoryProviders>,
  );
}

// The step a reader would press: never the current stage, which is a marker.
function proposalStep() {
  return screen.findByRole("button", { name: "Proposal" });
}

function reasonOf(step: HTMLElement): string | undefined {
  // FROM THE CONTROL, which is the whole point: a disabled button cannot be
  // focused and a `title` on it is announced by nobody, so a sentence the
  // control does not point at reaches no reader who needed it.
  return (
    document.getElementById(step.getAttribute("aria-describedby") ?? "")
      ?.textContent ?? undefined
  );
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The page already prints this sentence in its header band, so the rungs point
// at THAT one rather than drawing a second copy under each of themselves.
it.each([
  [
    "a colleague's deal",
    { owner_id: "u-else", writable: false } as Partial<Deal>,
  ],
  [
    "an archived deal",
    { archived_at: "2026-07-01T00:00:00Z" } as Partial<Deal>,
  ],
])("names the page's own sentence on %s", async (_what, fields) => {
  mount(deal(fields));

  const step = await proposalStep();
  expect(step.hasAttribute("disabled")).toBe(true);
  const reason = reasonOf(step);
  expect(reason).toBeTruthy();
  // The SAME element the page's band draws, not a copy of its words: a second
  // copy would drift, and a ladder of five rungs would print it five times.
  expect(screen.getAllByText(reason ?? "")).toHaveLength(1);
});

// Nothing on the page says this one, so the ladder draws it — once, under the
// ladder, with every rung naming it. It points at the door that DOES take the
// move, which is what makes a refusal usable rather than merely honest.
it("draws its own sentence on a closed deal, and points at the way back", async () => {
  mount(
    deal({ status: "won", stage_id: "s3", closed_at: "2026-07-01T00:00:00Z" }),
  );

  const step = await proposalStep();
  expect(step.hasAttribute("disabled")).toBe(true);
  expect(reasonOf(step)).toBe(
    "This deal is closed. Reopen it to move it to another stage.",
  );
});

// And a live deal this caller may move says nothing at all. Without this the
// cases above are satisfied by a ladder that refuses everybody, and the
// sentence would be drawn over a reader who has nothing to be told.
it("offers the move, unexplained, on a live deal the viewer may change", async () => {
  mount(deal());

  const step = await proposalStep();
  expect(step.hasAttribute("disabled")).toBe(false);
  expect(step.getAttribute("aria-describedby")).toBeNull();
  expect(screen.queryByText(/This deal is closed/)).toBeNull();
});
