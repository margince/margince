/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { ToastProvider } from "../../../design-system/toast";
import { LocaleProvider } from "../../../i18n";
import { ApprovalHost } from "../../../testing/approvalhost";
import { DealApprovals } from "./dealapprovals";

type Approval = components["schemas"]["Approval"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DRAFT: Approval = {
  id: "ap-draft",
  kind: "held_draft",
  status: "pending",
  summary: "Reply to Anna about the quote",
  proposed_by: "system:automation",
  proposed_change: {
    anchor_activity_id: "act-1",
    to: "anna@example.test",
    subject: "Re: the quote for Hanoi",
    body: "Attached, as promised, with the revised terms.",
  },
  created_at: "2026-08-20T09:00:00Z",
  target_entity_type: "deal",
  target_entity_id: "d-1",
};

// The drawer reads the decision by the id the card lists. Every other read
// answers empty, so the draft on screen can only come from that read.
function stubApprovalRead() {
  const fetched = vi.fn(async (input: RequestInfo | URL) => {
    const path =
      input instanceof Request
        ? new URL(input.url).pathname
        : new URL(String(input), "https://test").pathname;
    const body = path.endsWith(`/approvals/${DRAFT.id}`) ? DRAFT : { data: [] };
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fetched);
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ApprovalHost>
            <DealApprovals approvals={[DRAFT]} decide={vi.fn()} />
          </ApprovalHost>
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the deal page's awaiting-approval card", () => {
  it("opens a drafted email so its subject and body can be read before the decision", async () => {
    stubApprovalRead();
    renderCard();

    expect(
      screen.queryByText("Attached, as promised, with the revised terms."),
    ).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Open" }));

    const drawer = await screen.findByRole("dialog");
    expect(
      await within(drawer).findByText("Re: the quote for Hanoi"),
    ).toBeInTheDocument();
    expect(
      within(drawer).getByText(
        "Attached, as promised, with the revised terms.",
      ),
    ).toBeInTheDocument();
  });
});
