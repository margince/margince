/** @vitest-environment happy-dom */

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { commercialApproval } from "../../testing/commercial-approval";
import { ApprovalDetailModal } from "./approvaleditor";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("does not revive withheld evidence when opening the full decision", async () => {
  const approval = commercialApproval({
    summary: "Review the account lifecycle",
    requested_decider_id: undefined,
    evidence: [{ evidence_snippet: "Historical private source" }],
    preview: {
      version: 1,
      state: "unavailable",
      reviewer_reason: "record_authority",
      changes: [],
      evidence: [],
    },
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(approval)),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ApprovalDetailModal approvalId="ap-1" open onClose={() => undefined} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(await screen.findByText("Review the account lifecycle")).toBeVisible();
  expect(
    screen.queryByText("Historical private source"),
  ).not.toBeInTheDocument();
});
