/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { ToastProvider } from "../../../design-system/toast";
import { type Locale, LocaleProvider } from "../../../i18n";
import { ApprovalHost } from "../../../testing/approvalhost";
import { DealApprovals } from "./dealapprovals";

type Approval = components["schemas"]["Approval"];

afterEach(cleanup);

const QUESTION: Approval = {
  id: "ap-proposal-stage",
  kind: "deal_proposal_stage",
  status: "pending",
  summary: "A proposal went out on Hanoi depot. Move the deal to “Proposal”?",
  proposed_by: "system:proposal-stage",
  proposed_change: {
    deal_id: "d-1",
    to_stage_id: "st-proposal",
    to_stage_name: "Proposal",
    skipped_stage_ids: ["st-qualified"],
    skipped_stage_names: ["Qualified"],
  },
  created_at: "2026-11-20T09:00:00Z",
  target_entity_type: "deal",
  target_entity_id: "d-1",
};

// The admin named the stage "Proposal" and gave it a German and a Vietnamese
// name. The card has to show the one in the reader's language.
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
          name_translations: { de: "Angebotsphase", vi: "Giai đoạn báo giá" },
        },
      ],
    },
  ],
};

function renderCard(locale: Locale) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(PIPELINES))),
  );
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <ToastProvider>
          <ApprovalHost>
            <DealApprovals approvals={[QUESTION]} decide={vi.fn()} />
          </ApprovalHost>
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the deal page card for a proposal that went out", () => {
  it.each([
    ["en", "Move to Proposal?"],
    ["de", "In die Phase „Angebotsphase“ verschieben?"],
    ["vi", "Chuyển sang giai đoạn “Giai đoạn báo giá”?"],
  ] as const)(
    "asks in %s which stage the deal would move to",
    async (locale, words) => {
      renderCard(locale);
      expect(await screen.findByText(words)).toBeInTheDocument();
    },
  );
});
