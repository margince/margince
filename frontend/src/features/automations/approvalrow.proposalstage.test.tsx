/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import type { Approval } from "../../lib/approvals.queries";
import { ApprovalHost } from "../../testing/approvalhost";
import { ApprovalDetailModal } from "./approvaldetail";
import { ApprovalRow } from "./approvalrow";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The server sentence is frozen with the stage name stored when the question
// was raised, so a German reader has to see the stage's German name instead.
const QUESTION: Approval = {
  id: "ap-proposal-stage",
  kind: "deal_proposal_stage",
  status: "pending",
  summary: "A proposal went out on Hanoi depot. Move the deal to “Proposal”?",
  proposed_by: "system:proposal-stage",
  proposed_change: {
    deal_id: "d-1",
    deal_name: "Hanoi depot",
    to_stage_id: "st-proposal",
    to_stage_name: "Proposal",
    skipped_stage_ids: ["st-qualified"],
    skipped_stage_names: ["Qualified"],
  },
  created_at: "2026-11-20T09:00:00Z",
  target_entity_type: "deal",
  target_entity_id: "d-1",
};

const PIPELINES = {
  data: [
    {
      id: "pl-1",
      name: "Sales",
      is_default: true,
      stages: [
        {
          id: "st-proposal",
          name: "Proposal",
          name_translations: { de: "Angebotsphase" },
        },
        {
          id: "st-qualified",
          name: "Qualified",
          name_translations: { de: "Qualifiziert" },
        },
      ],
    },
  ],
};

function stubFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes("/approvals/") ? QUESTION : PIPELINES;
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function shown(ui: React.ReactNode) {
  stubFetch();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="de">
        <ToastProvider>
          <ApprovalHost>{ui}</ApprovalHost>
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a proposal-stage question read in German", () => {
  it("names the stage in German on the inbox row, with the stored name nowhere", async () => {
    const { container } = shown(<ApprovalRow approval={QUESTION} />);
    expect(
      await screen.findByText(/Den Deal in die Phase „Angebotsphase“/),
    ).toBeInTheDocument();
    expect(container.textContent).not.toContain("Proposal");
    expect(container.textContent).not.toContain("Qualified");
  });

  it("names the stage in German in the detail, with the stored name nowhere", async () => {
    shown(
      <ApprovalDetailModal approvalId={QUESTION.id} open onClose={() => {}} />,
    );
    expect(
      await screen.findByText(/Den Deal in die Phase „Angebotsphase“/),
    ).toBeInTheDocument();
    expect(screen.getByText("Qualifiziert")).toBeInTheDocument();
    expect(document.body.textContent).not.toContain("Proposal");
    expect(document.body.textContent).not.toContain("Qualified");
  });
});
