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
import { LocaleProvider } from "../i18n";
import { ApprovalRow } from "./approvalrow";
import type { Approval } from "./approvals.queries";

// What a reader sees after approving, and where the button goes.
//
// The toast is the only signpost to a reversal that already exists, so what
// matters is that it APPEARS and that pressing it lands on the record whose
// history holds the change. useToast is local state paired with its own
// ToastRegion — a row showing a toast without rendering the region shows
// nothing at all, and only a render proves it does.

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
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

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
  it("stays silent after a rejection", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ id: "ap1", status: "rejected" })),
    );
    render(<ApprovalRow approval={closeDateApproval()} />);

    await userEvent.click(screen.getByRole("button", { name: "Reject" }));
    const confirm = await screen.findAllByRole("button", { name: "Reject" });
    await userEvent.click(confirm[confirm.length - 1]);

    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Undo on the record" }),
      ).toBeNull(),
    );
  });

  // A step-up names no record, so there is no history to send anyone to.
  it("stays silent when the approval names no record", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ id: "ap1", status: "approved" })),
    );
    render(
      <ApprovalRow
        approval={closeDateApproval({
          kind: "quota_release",
          target_entity_type: null,
          target_entity_id: null,
        })}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Accept" }));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Undo on the record" }),
      ).toBeNull(),
    );
  });
});
