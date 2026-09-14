/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../../app/mefixture";
import { LocaleProvider } from "../../i18n";
import { type ClosedDeal, CloseReviewOffer } from "./closereviewoffer";

// The point of this offer is that the reason typed into the close dialog
// answers the review's first question, so nobody types it twice. What it must
// never do is hold the close open: the deal is closed whether this is answered
// or dismissed.

const TEMPLATES = [
  {
    id: "t-1",
    key: "loss_review",
    label: "Loss review",
    outcome: "lost",
    questions: [
      {
        key: "why_we_lost",
        label: "Why did we lose?",
        type: "text",
        required: true,
      },
      { key: "lost_to", label: "Who instead?", type: "text", required: false },
    ],
    version: 1,
    active: true,
    system: true,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  },
];

const CLOSED: ClosedDeal = {
  dealId: "d-1",
  outcome: "lost",
  closingOccurrenceId: "closing-1",
  reason: "They went with the incumbent.",
};

function stubFetch(canWrite = true) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const body = req.url.endsWith("/v1/me")
        ? meFixture({
            allow: { activity: canWrite ? ["read", "create"] : ["read"] },
          })
        : { data: TEMPLATES };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function render(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
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

it("offers the review with the close's own reason already answered", async () => {
  stubFetch();
  render(<CloseReviewOffer closed={CLOSED} onDismiss={() => {}} />);
  const first = await screen.findByLabelText(/Why did we lose/);
  expect((first as HTMLTextAreaElement).value).toBe(
    "They went with the incumbent.",
  );
});

it("leaves the rest of the questions empty", async () => {
  stubFetch();
  render(<CloseReviewOffer closed={CLOSED} onDismiss={() => {}} />);
  const second = await screen.findByLabelText(/Who instead/);
  expect((second as HTMLTextAreaElement).value).toBe("");
});

it("offers nothing when no deal just closed", () => {
  stubFetch();
  const { container } = render(
    <CloseReviewOffer closed={null} onDismiss={() => {}} />,
  );
  expect(container.textContent).toBe("");
});

it("offers nothing to a reader who may not write an activity", async () => {
  // A review is written as an activity. Without that grant the offer would be
  // a form the server refuses, which is worse than no offer at all.
  stubFetch(false);
  const { container } = render(
    <CloseReviewOffer closed={CLOSED} onDismiss={() => {}} />,
  );
  await new Promise((r) => setTimeout(r, 0));
  expect(container.textContent).toBe("");
});

it("prefills nothing when the close stated no reason", async () => {
  // A win closed against a signed contract is one click and names no reason.
  // Seeding an empty string would make a required field look answered.
  stubFetch();
  render(
    <CloseReviewOffer
      closed={{ ...CLOSED, reason: "" }}
      onDismiss={() => {}}
    />,
  );
  const first = await screen.findByLabelText(/Why did we lose/);
  expect((first as HTMLTextAreaElement).value).toBe("");
});
