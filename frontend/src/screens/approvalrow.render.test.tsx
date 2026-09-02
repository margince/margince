/** @vitest-environment jsdom */

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
