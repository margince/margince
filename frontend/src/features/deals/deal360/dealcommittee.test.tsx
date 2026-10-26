/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { CommitteeReading } from "./dealcommittee";

type DealCoverage = components["schemas"]["DealCoverage"];

// The states worth asserting render alike if their distinction is
// dropped. They are a withheld read and an empty one, and a deal with a gap
// and one without. A map that showed either pair the same way would report a
// covered deal from a check that never ran.

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";

const coverage = (over: Partial<DealCoverage> = {}): DealCoverage => ({
  deal_id: DEAL_ID,
  stakeholders: [
    {
      contact_id: "01a03000-0000-7000-8000-0000000000b1",
      contact_name: "Dana Weiss",
      role: "champion",
      engaged: true,
    },
    {
      contact_id: "01a03000-0000-7000-8000-0000000000b3",
      contact_name: "Ines Kraft",
      role: "evaluator",
      engaged: false,
    },
  ],
  our_side: [
    {
      user_id: "01a03000-0000-7000-8000-0000000000c1",
      display_name: "Lena Fischer",
      strength_bucket: "strong",
      interactions_90d: 24,
      last_at: "2026-08-20T09:00:00Z",
    },
  ],
  risks: [],
  sections_omitted: [],
  ...over,
});

function draw(
  props: Omit<Parameters<typeof CommitteeReading>[0], "roleLabel">,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrap = (node: ReactNode) => (
    <QueryClientProvider client={client}>
      <LocaleProvider>{node}</LocaleProvider>
    </QueryClientProvider>
  );
  return render(
    wrap(<CommitteeReading {...props} roleLabel={(role) => role} />),
  );
}

afterEach(cleanup);

describe("the buying committee, drawn", () => {
  it("draws one row per seat, named, and a curve only to the engaged ones", () => {
    draw({ coverage: coverage(), withheld: false, pending: false });
    expect(document.querySelectorAll(".dc-seat").length).toBe(
      coverage().stakeholders.length,
    );
    expect(screen.getByText("Dana Weiss")).toBeTruthy();
    expect(screen.getByText("Ines Kraft")).toBeTruthy();
    // A line to a seat nobody has exchanged with would draw a conversation
    // that never happened.
    expect(document.querySelectorAll(".dc-link").length).toBe(1);
  });

  it("draws a dashed row and curve for every seat the rules find missing", () => {
    draw({
      coverage: coverage({
        risks: [
          { kind: "coverage_gap", summary: "no engaged champion" },
          { kind: "going_cold", summary: "quiet", days_since_touch: 30 },
        ],
      }),
      withheld: false,
      pending: false,
    });
    expect(document.querySelectorAll(".dc-seat-gap").length).toBe(1);
    expect(document.querySelectorAll(".dc-link-gap").length).toBe(1);
  });

  // Each no-seat state asserts its own sentence, not merely the absence of
  // seats. Absence is what they share, so a withheld lane that regressed into
  // an empty one would stay green on an absence-only check.
  it("says the lane is withheld rather than showing it as empty", () => {
    draw({ coverage: undefined, withheld: true, pending: false });
    expect(screen.getByText("Hidden for your role")).toBeTruthy();
    expect(screen.queryByText("Nobody named")).toBeNull();
  });

  it("says the read is still loading rather than showing it as empty", () => {
    draw({ coverage: undefined, withheld: false, pending: true });
    expect(
      document.querySelector('[role="status"][aria-busy="true"]'),
    ).toBeTruthy();
    expect(screen.queryByText("Nobody named")).toBeNull();
  });

  it("says nobody is named when the read answers with no seat and no gap", () => {
    draw({
      coverage: coverage({ stakeholders: [] }),
      withheld: false,
      pending: false,
    });
    expect(screen.getByText("Nobody named")).toBeTruthy();
  });
});
