// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealStatusCardPanel } from "./dealstatus";

type Basis = components["schemas"]["WorkEvidenceBasis"];

const source = "00000000-0000-7000-8000-000000000001";
const asOf = "2026-08-24T12:00:00Z";
const current: Basis = {
  kind: "current_request",
  scope: "deal",
  source_activity_id: source,
  source_at: "2026-02-05T12:00:00Z",
  source_precision: "instant",
  source_version: 1,
  evaluated_at: asOf,
  policy_version: "test",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(basis?: Basis) {
  const review = basis?.kind === "request_review";
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create", "update"] }),
    "GET /deals/demo-deal/status": () =>
      jsonResponse({
        deal_id: "demo-deal",
        story: {
          sentences: [{ text: "The customer asked for prices.", evidence: [] }],
        },
        reply_to: review ? null : source,
        request_basis: review ? undefined : basis,
        next: {
          action: "create_task",
          reason: review
            ? "Review this request"
            : "Take responsibility for this request",
          arguments: {
            subject: "Price request",
            source: "manual",
            request_activity_id: source,
          },
          basis,
          evidence: [
            {
              activity_id: source,
              text: "Price request",
              occurred_precision: "instant",
            },
          ],
        },
        generated_at: asOf,
        generated_by: "deterministic",
      }),
    "GET /deals/demo-deal/coverage": () =>
      jsonResponse({ stakeholders: [], risks: [] }),
  });
  render(
    <StoryProviders>
      <DealStatusCardPanel
        deal={{ id: "demo-deal", name: "Demo deal" }}
        onOpenEmail={vi.fn()}
      />
    </StoryProviders>,
  );
}

it.each([undefined, current, { ...current, kind: "request_review" }])(
  "does not draw source age as lateness without a recorded deadline",
  async (basis) => {
    show(basis);
    expect(
      await screen.findByRole("button", { name: "Open the message" }),
    ).toBeVisible();
    expect(screen.queryByText("200 days")).not.toBeInTheDocument();
  },
);
