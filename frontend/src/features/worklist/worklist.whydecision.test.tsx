// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { row } from "./worklist.fixtures";
import { WorklistRow } from "./worklist.row";
import { WorklistWhy } from "./worklist.why";

// A decision row shows what was proposed where it is read, not only behind the
// Decide drawer. The card shows the proposal; its details carry the decision.

type Approval = components["schemas"]["Approval"];

const ID = "01a1bfc9-66c6-7685-8685-3faa289b0bda";

const followUp: Approval = {
  id: ID,
  kind: "deal_follow_up",
  status: "pending",
  proposed_by: "agent:overnight",
  created_at: "2026-11-09T10:06:30Z",
  expires_at: "2099-11-12T10:06:30Z",
  summary:
    'Draft a follow-up on "Acme Rollout" — the email on 9 Nov 2026 left no next step planned',
  target_entity_type: "deal",
  target_entity_id: "01a08f74-3be3-7523-9d23-7d53f4110e2e",
  target_label: "Acme Rollout",
  proposed_change: {
    deal_id: "01a08f74-3be3-7523-9d23-7d53f4110e2e",
    subject: "Follow up on Acme Rollout",
    due_date: "2026-11-12",
    body: "Follow up on the email “Re: pricing” from 9 Nov 2026. No next step is on the timeline yet.",
    evidence_activity_id: "01a1bf79-0e55-750e-bdb0-7bfb5605261d",
  },
};

const decision = row({
  id: ID,
  source: "approval",
  category: "approvals",
  title: followUp.summary,
  actions: ["decide"],
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Answers the approval read; every other read is empty, which the surfaces
// around the proposal treat as nothing to add.
function serve() {
  const read = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const target = input instanceof Request ? input : new Request(input, init);
    const path = new URL(target.url).pathname;
    const body = path.endsWith(`/approvals/${ID}`) ? followUp : {};
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", read);
  return read;
}

function draw(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <LocaleProvider initial="en">
      <QueryClientProvider client={client}>
        <ToastProvider>{node}</ToastProvider>
      </QueryClientProvider>
    </LocaleProvider>,
  );
}

it("shows the proposed task on the card it is read on", async () => {
  serve();
  draw(<WorklistRow item={decision} owner="" framed />);

  expect(await screen.findByText("Follow up on Acme Rollout")).toBeTruthy();
  expect(screen.getByText("Task")).toBeTruthy();
  expect(screen.getByText("Due")).toBeTruthy();
  expect(screen.getByText(/Re: pricing/)).toBeTruthy();
  // The identifiers the write uses are not facts a reader weighs.
  expect(screen.queryByText(followUp.target_entity_id ?? "")).toBeNull();
});

it("answers the decision in its details instead of opening a second drawer", async () => {
  serve();
  draw(<WorklistWhy item={decision} answer />);

  expect(await screen.findByText("Follow up on Acme Rollout")).toBeTruthy();
  expect(screen.getByRole("button", { name: /reject/i })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Decide" })).toBeNull();
});

it("reads nothing for a row that asks no decision", () => {
  const read = serve();
  draw(<WorklistRow item={row({ title: "Call Acme" })} owner="" framed />);

  const approvalReads = read.mock.calls.filter(([input]) =>
    String(input instanceof Request ? input.url : input).includes(
      "/approvals/",
    ),
  );
  expect(approvalReads).toHaveLength(0);
});
