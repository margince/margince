// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type {
  LegalReview,
  LegalReviewQueueItem,
  LegalReviewSummary,
} from "../lib/legalreviews.queries";
import { jsonResponse } from "./story-utils";

type Deal = components["schemas"]["Deal"];

export const DEAL_ID = "019f7e65-0000-7000-8000-00000000d001";
export const REVIEW_ID = "019f7e65-0000-7000-8000-00000000a001";
export const EARLIER_ID = "019f7e65-0000-7000-8000-00000000a000";
export const REQUESTER_ID = "019f7e65-0000-7000-8000-0000000000c1";
/** The user meFixture signs in as. */
export const VIEWER_ID = "00000000-0000-4000-8000-000000000001";

export const legalDeal: Deal = {
  id: DEAL_ID,
  name: "Warehouse rollout",
  pipeline_id: "019f7e65-0000-7000-8000-0000000005a0",
  stage_id: "019f7e65-0000-7000-8000-0000000005a1",
  status: "open",
  source: "manual",
  captured_by: `human:${REQUESTER_ID}`,
  writable: true,
  version: 4,
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
};

export function summary(
  over: Partial<LegalReviewSummary> = {},
): LegalReviewSummary {
  return {
    id: REVIEW_ID,
    deal_id: DEAL_ID,
    status: "requested",
    note: "Check the liability cap.",
    requested_by: REQUESTER_ID,
    requested_by_name: "Rosa Rep",
    created_at: "2026-10-01T09:00:00Z",
    version: 1,
    ...over,
  };
}

export const changesNeeded = summary({
  id: EARLIER_ID,
  status: "changes_needed",
  comments: "Cap the liability at the contract value.",
  decided_by_name: "Lena Legal",
  decided_at: "2026-09-20T09:00:00Z",
});

export function queueItem(
  name: string,
  value: number | null,
  over: Partial<LegalReviewSummary> = {},
): LegalReviewQueueItem {
  return {
    review: summary({ id: `${REVIEW_ID}-${name}`, ...over }),
    deal_name: name,
    company_name: "Acme",
    stage_name: "Negotiation",
    value_minor: value,
    value_currency: value === null ? null : "EUR",
    expected_close_date: "2026-12-01",
  };
}

export function review(over: Partial<LegalReviewSummary> = {}): LegalReview {
  return {
    review: summary(over),
    earlier: [changesNeeded],
    packet: {
      deal: {
        id: DEAL_ID,
        name: "Warehouse rollout",
        company_name: "Acme",
        amount_minor: 9_000_000,
        currency: "EUR",
        stage_id: "019f7e65-0000-7000-8000-0000000005a1",
        stage_name: "Negotiation",
        expected_close_date: "2026-12-01",
        owner_name: "Rosa Rep",
      },
      value_minor: 9_000_000,
      value_currency: "EUR",
      offer: {
        id: "019f7e65-0000-7000-8000-0000000000f1",
        offer_number: "OF-2026-014",
        revision: 2,
        status: "sent",
        net_minor: 7_563_025,
        tax_minor: 1_436_975,
        gross_minor: 9_000_000,
        currency: "EUR",
        valid_until: "2026-11-15",
        intro_text: "Thank you for the meeting.",
        terms_text: "Payment within 30 days.",
        has_pdf: true,
        lines: [
          {
            position: 0,
            description: "Implementation",
            quantity: "2.000",
            unit_price_minor: 3_781_513,
            tax_rate: "19.00",
          },
          {
            position: 1,
            description: "Support",
            quantity: "1.000",
            unit_price_minor: 100_000,
            tax_rate: "19.00",
            billing_model: "recurring",
            billing_interval_months: 1,
            interval_count: 36,
            staged: true,
          },
        ],
      },
      binding: [
        {
          id: "019f7e65-0000-7000-8000-0000000000f2",
          offer_number: "OF-2026-015",
          revision: 1,
          status: "sent",
          currency: "EUR",
          has_pdf: false,
          lines: [],
        },
      ],
      history: [
        { stage_name: "Discovery", entered_at: "2026-08-01T09:00:00Z" },
        { stage_name: "Negotiation", entered_at: "2026-09-10T09:00:00Z" },
      ],
      documents: [
        {
          attachment_id: "019f7e65-0000-7000-8000-0000000000e1",
          category: "contract",
          doc_state: "draft",
          added_at: "2026-09-15T09:00:00Z",
          filename: "msa-draft.pdf",
          byte_size: 120_000,
          available: true,
        },
        {
          attachment_id: "019f7e65-0000-7000-8000-0000000000e2",
          category: "legal",
          doc_state: "final",
          added_at: "2026-09-16T09:00:00Z",
          available: false,
        },
      ],
    },
  };
}

export const legalOn = () => jsonResponse({ enabled: true });
export const legalOff = () => jsonResponse({ enabled: false });
