/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { OutcomeReviewPanel } from "./outcomereview";

// The rule this panel turns on is WHICH closing a review is about, and it is
// the one that fails silently: a review of March's loss rendered as if it were
// about June's win reads as a correct record of the wrong thing.

const CLOSING = "closing-now";
const EARLIER = "closing-before";

const REVIEW = {
  id: "r-1",
  activity_id: "a-1",
  deal_id: "d-1",
  closing_occurrence_id: CLOSING,
  outcome: "won",
  template_key: "win_review",
  template_version: 1,
  questions: [
    { key: "why", label: "Why did we win?", type: "text", required: true },
    { key: "else", label: "Who else?", type: "text", required: false },
  ],
  answers: { why: "Price." },
  revision: 1,
  created_at: "2026-09-12T10:00:00Z",
  updated_at: "2026-09-12T10:00:00Z",
};

const TEMPLATES = [
  {
    id: "t-1",
    key: "win_review",
    label: "Win review",
    outcome: "won",
    questions: REVIEW.questions,
    version: 1,
    active: true,
    system: true,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  },
];

function stubFetch(reviews: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const body = req.url.includes("outcome-reviews")
        ? { data: reviews }
        : { data: TEMPLATES };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function render(ui: ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={qc}>
      <LocaleProvider>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("says nothing at all on an open deal", () => {
  stubFetch([]);
  const { container } = render(
    <OutcomeReviewPanel
      dealId="d-1"
      status="open"
      closingOccurrenceId={null}
      writable
    />,
  );
  // Absent, not empty. A deal still in flight has no outcome to review, and a
  // card inviting one would be asking for a verdict nobody can give.
  expect(container.textContent).toBe("");
});

it("invites a review on a closed deal that has none", async () => {
  stubFetch([]);
  render(
    <OutcomeReviewPanel
      dealId="d-1"
      status="won"
      closingOccurrenceId={CLOSING}
      writable
    />,
  );
  expect(await screen.findByText(/No review written yet/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Add review" })).toBeTruthy();
});

it("offers no way in without write access", async () => {
  stubFetch([]);
  render(
    <OutcomeReviewPanel
      dealId="d-1"
      status="won"
      closingOccurrenceId={CLOSING}
      writable={false}
    />,
  );
  expect(await screen.findByText(/No review written yet/)).toBeTruthy();
  // Absent rather than disabled: a disabled button invites a reader to hunt
  // for the reason it is disabled.
  expect(screen.queryByRole("button", { name: "Add review" })).toBeNull();
});

it("renders the questions frozen onto the review, not today's template", async () => {
  stubFetch([
    {
      ...REVIEW,
      questions: [
        { key: "why", label: "AS ASKED THEN", type: "text", required: true },
      ],
      answers: { why: "Price." },
    },
  ]);
  render(
    <OutcomeReviewPanel
      dealId="d-1"
      status="won"
      closingOccurrenceId={CLOSING}
      writable
    />,
  );
  // The template is editable. Rendering today's wording over an old answer
  // would put words in the author's mouth.
  expect(await screen.findByText("AS ASKED THEN")).toBeTruthy();
  expect(screen.queryByText("Why did we win?")).toBeNull();
});

it("marks a review written about an earlier closing", async () => {
  stubFetch([{ ...REVIEW, id: "r-old", closing_occurrence_id: EARLIER }]);
  render(
    <OutcomeReviewPanel
      dealId="d-1"
      status="won"
      closingOccurrenceId={CLOSING}
      writable
    />,
  );
  // Reopening and reclosing makes a new outcome. The old review stays readable
  // and says which closing it was about, so nobody reads March's loss review
  // as a verdict on June's win.
  expect(await screen.findByText(/About an earlier closing/)).toBeTruthy();
});

it("says which question went unanswered rather than dropping the row", async () => {
  stubFetch([REVIEW]);
  render(
    <OutcomeReviewPanel
      dealId="d-1"
      status="won"
      closingOccurrenceId={CLOSING}
      writable
    />,
  );
  expect(await screen.findByText("Who else?")).toBeTruthy();
  expect(screen.getByText("Not answered")).toBeTruthy();
});
