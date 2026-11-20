/** @vitest-environment happy-dom */

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { Approval } from "../../lib/approvals.queries";
import { ApprovalRow } from "./approvalrow";

afterEach(cleanup);

const request = (overrides: Partial<Approval> = {}) =>
  ({
    id: "ap1",
    kind: "legal_review_request",
    status: "pending",
    summary:
      "Legal review requested for Fleet renewal. Answer it under Legal reviews.",
    proposed_by: "system:legal_review_desk",
    proposed_change: {
      review_id: "r1",
      deal_id: "d1",
      deal_name: "Fleet renewal",
    },
    created_at: "2026-11-20T09:00:00Z",
    ...overrides,
  }) as Approval;

function show(approval: Approval) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <ApprovalRow approval={approval} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a legal review request in the inbox", () => {
  it("offers the queue and a dismissal, and no decision", () => {
    show(request());
    expect(
      screen.getByRole("link", { name: en["legalReview.openQueue"] }),
    ).toHaveAttribute("href", "#/legal-reviews");
    expect(
      screen.getByRole("button", { name: en["legalReview.dismissRequest"] }),
    ).toBeInTheDocument();
    for (const verb of [
      en["brief.approval.approve"],
      en["decision.reject"],
      en["decision.status.approved"],
      en["decision.status.rejected"],
    ]) {
      expect(screen.queryByText(verb)).not.toBeInTheDocument();
    }
    expect(screen.getAllByRole("button")).toHaveLength(1);
  });
});
