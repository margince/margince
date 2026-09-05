/** @vitest-environment jsdom */

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { ApprovalRow } from "./approvalrow";
import type { Approval } from "./approvals.queries";
import { isPreviewDoor, refusedPreview } from "./sendpermission.testkit";

// What a reader sees after approving, and where the button goes.
//
// The toast is the only signpost to a reversal that already exists, so what
// matters is that it APPEARS and that pressing it lands on the record whose
// history holds the change.
//
// The region is the application's, mounted once in `main.tsx`, so this harness
// mounts it the same way. The row used to render its own when no surface handed
// one down, which is what let a row shown on its own say anything at all; with
// one region for the whole app that fallback — and the prop that fed it — is
// gone, and a suite that renders the row alone has to supply the region the
// shell would.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// How many times the row posted a decision, by verb. The negative tests wait
// on these: an absence asserted before the request went out is an absence that
// survives the interaction being deleted.
function decisionCalls(fetched: ReturnType<typeof vi.fn>, verb: string) {
  // The client hands fetch a Request, not a url string, so the path is read
  // off the object rather than by stringifying the argument.
  return fetched.mock.calls.filter(([input]) =>
    input instanceof Request
      ? new URL(input.url).pathname.endsWith(verb)
      : String(input).endsWith(verb),
  ).length;
}
const approveCalls = (f: ReturnType<typeof vi.fn>) =>
  decisionCalls(f, "/approve");
const rejectCalls = (f: ReturnType<typeof vi.fn>) =>
  decisionCalls(f, "/reject");

function closeDateApproval(overrides: Partial<Approval> = {}): Approval {
  return {
    id: "ap1",
    kind: "close_date_correction",
    status: "pending",
    summary: "Confirm the real close date",
    proposed_by: "system:close-date",
    proposed_change: {
      deal_id: "d1",
      expected_close_date: "2026-11-15",
      basis: "Nobody has answered since 5 August.",
    },
    created_at: "2026-08-20T09:00:00Z",
    target_entity_type: "deal",
    target_entity_id: "d1",
    ...overrides,
  } as Approval;
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
  globalThis.location.hash = "";
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the offer to put an approved change back", () => {
  it("appears after approving, and lands on the record that changed", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ id: "ap1", status: "approved" })),
    );
    render(<ApprovalRow approval={closeDateApproval()} />);

    await userEvent.click(screen.getByRole("button", { name: "Accept" }));

    const undo = await screen.findByRole("button", {
      name: "Undo on the record",
    });
    await userEvent.click(undo);
    expect(globalThis.location.hash).toBe("#/deals/d1");
  });

  // A rejection changed nothing, so there is nothing to put back and no offer.
  //
  // The absence is asserted only AFTER the reject call has actually gone out.
  // Before it, the button is absent for the uninteresting reason that nothing
  // has happened yet — an assertion that would survive the interaction being
  // removed entirely, and therefore proves nothing about rejecting.
  it("stays silent after a rejection", async () => {
    const fetched = vi.fn(async () =>
      jsonResponse({ id: "ap1", status: "rejected" }),
    );
    vi.stubGlobal("fetch", fetched);
    render(<ApprovalRow approval={closeDateApproval()} />);

    // One press rejects — no confirmation step, no reason form.
    await userEvent.click(screen.getByRole("button", { name: "Reject" }));

    await waitFor(() => expect(rejectCalls(fetched)).toBe(1));
    expect(
      screen.queryByRole("button", { name: "Undo on the record" }),
    ).toBeNull();
  });

  // A step-up names no record, so there is no history to send anyone to. Same
  // rule as above: the approve call must have gone out before the absence means
  // anything.
  it("stays silent when the approval names no record", async () => {
    const fetched = vi.fn(async () =>
      jsonResponse({ id: "ap1", status: "approved" }),
    );
    vi.stubGlobal("fetch", fetched);
    render(
      <ApprovalRow
        approval={closeDateApproval({
          kind: "volume_release",
          target_entity_type: null,
          target_entity_id: null,
        })}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Accept" }));
    await waitFor(() => expect(approveCalls(fetched)).toBe(1));
    expect(
      screen.queryByRole("button", { name: "Undo on the record" }),
    ).toBeNull();
  });
});

// The card says whether the engine would let the staged mail go, before the
// approver releases it. An approver decides on somebody else's behalf, and
// used to learn of a refusal the way the author would have: by pressing Accept
// and reading the effect's failure.
describe("whether the staged mail may go", () => {
  function heldDraft(over: Partial<Approval> = {}): Approval {
    return {
      id: "ap2",
      kind: "held_draft",
      status: "pending",
      summary: "Reply to Anna about the quote",
      proposed_by: "system:automation",
      proposed_change: {
        anchor_activity_id: "act-1",
        to: "anna@example.test",
        subject: "Re: the quote",
        body: "Attached, as promised.",
      },
      created_at: "2026-08-20T09:00:00Z",
      target_entity_type: "activity",
      target_entity_id: "act-1",
      ...over,
    };
  }

  // Routes the preview door and nothing else: the card's other reads answer
  // empty, so what appears can only have come from the engine's answer.
  function stubEngine(answer: () => Response) {
    const fetched = vi.fn(async (input: RequestInfo | URL) => {
      const path =
        input instanceof Request
          ? new URL(input.url).pathname
          : new URL(String(input), "https://test").pathname;
      return isPreviewDoor(path) ? answer() : jsonResponse({ data: [] });
    });
    vi.stubGlobal("fetch", fetched);
    return fetched;
  }

  it("names who decided against the message, and that nobody may lift it", async () => {
    const fetched = stubEngine(() =>
      jsonResponse(
        refusedPreview("anna@example.test", {
          reason_code: "marketing_objection",
          decided_by: "subject",
        }),
      ),
    );
    render(<ApprovalRow approval={heldDraft()} />);

    expect(
      await screen.findByText(/asked not to receive marketing/i),
    ).toBeInTheDocument();
    // Asked the way the release will ask: against the thread the draft answers.
    const asked = fetched.mock.calls.find(([input]) =>
      isPreviewDoor(
        input instanceof Request ? new URL(input.url).pathname : String(input),
      ),
    );
    expect(
      asked && asked[0] instanceof Request
        ? new URL(asked[0].url).pathname
        : "",
    ).toBe("/v1/activities/act-1/send-email:preview");
  });

  // A decided row has nothing left to release, so there is nothing to ask.
  it("asks nothing about a proposal already decided", async () => {
    const fetched = stubEngine(() => jsonResponse({}));
    render(
      <ApprovalRow approval={heldDraft({ status: "approved" })} decided />,
    );
    await screen.findByText("Reply to Anna about the quote");
    expect(
      fetched.mock.calls.some(([input]) =>
        isPreviewDoor(
          input instanceof Request
            ? new URL(input.url).pathname
            : String(input),
        ),
      ),
    ).toBe(false);
  });
});
