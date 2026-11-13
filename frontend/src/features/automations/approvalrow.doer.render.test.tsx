/** @vitest-environment happy-dom */

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import type { Approval } from "../../lib/approvals.queries";
import { ApprovalRow } from "./approvalrow";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Names Priya and nobody else; every other read answers empty.
function stubNames() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url =
        input instanceof Request
          ? new URL(input.url)
          : new URL(String(input), "https://test");
      const body = url.pathname.endsWith("/users/names")
        ? {
            data: url.searchParams.getAll("id").includes("seat-priya")
              ? [{ id: "seat-priya", display_name: "Priya Raman" }]
              : [],
          }
        : { data: [] };
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function renderDecided(approval: Approval) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ApprovalRow approval={approval} decided />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const decided = {
  status: "approved",
  proposed_by: "system:overnight",
  created_at: "2026-11-10T09:00:00Z",
  decided_at: "2026-11-11T09:00:00Z",
} as const;

it("names the colleague chosen to do a decided follow-up", async () => {
  stubNames();
  renderDecided({
    ...decided,
    id: "ap-follow",
    kind: "deal_follow_up",
    summary: "Follow up on Northwind",
    proposed_change: {
      deal_id: "deal-1",
      subject: "Follow up on Northwind",
      body: "Ask about the pilot",
      due_date: "2026-11-20",
      doer_id: "seat-priya",
    },
  });
  expect(await screen.findByText("Who does this")).toBeVisible();
  expect(await screen.findByText("Priya Raman")).toBeVisible();
});

it("names the colleague chosen to keep a decided promise, not its promiser", async () => {
  stubNames();
  renderDecided({
    ...decided,
    id: "ap-promise",
    kind: "commitment_task",
    summary: "Send the pricing",
    proposed_change: {
      summary: "Send the pricing",
      party: "Tom",
      seat_id: "seat-tom",
      promiser_seat_id: "seat-tom",
      doer_id: "seat-priya",
    },
  });
  expect(
    await screen.findByText("Creates a task assigned to Priya Raman."),
  ).toBeVisible();
});

it("shows no doer row on a follow-up that kept its own rule", async () => {
  stubNames();
  renderDecided({
    ...decided,
    id: "ap-own",
    kind: "deal_follow_up",
    summary: "Follow up on Northwind",
    proposed_change: {
      deal_id: "deal-1",
      subject: "Follow up on Northwind",
      body: "Ask about the pilot",
      due_date: "2026-11-20",
      doer_id: "",
    },
  });
  expect(
    (await screen.findAllByText("Follow up on Northwind")).length,
  ).toBeGreaterThan(0);
  expect(screen.queryByText("Who does this")).toBeNull();
});
