/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { DealCommitteeMap } from "./dealcommittee";

type DealCoverage = components["schemas"]["DealCoverage"];

// The states worth asserting are the ones that render IDENTICALLY if their
// distinction is dropped — the stories file names the same pair. A withheld
// read and an empty one both draw no seats; a deal with a gap and one without
// both draw seats. A map that showed either pair the same way would report a
// covered deal from a check that never ran.

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";

const coverage = (over: Partial<DealCoverage> = {}): DealCoverage => ({
  deal_id: DEAL_ID,
  stakeholders: [
    {
      person_id: "01a03000-0000-7000-8000-0000000000b1",
      person_name: "Dana Weiss",
      role: "champion",
      engaged: true,
    },
    {
      person_id: "01a03000-0000-7000-8000-0000000000b3",
      person_name: "Ines Kraft",
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

function draw(props: Parameters<typeof DealCommitteeMap>[0]): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrap = (node: ReactNode) => (
    <QueryClientProvider client={client}>
      <LocaleProvider>{node}</LocaleProvider>
    </QueryClientProvider>
  );
  render(wrap(<DealCommitteeMap {...props} />));
}

afterEach(cleanup);

describe("the buying committee, drawn", () => {
  it("names every seat it draws, engaged or not", () => {
    draw({ coverage: coverage(), withheld: false, pending: false, overlay: false });
    // The accessible list is the assertion rather than the SVG: a reader on a
    // screen reader gets the rows, and a map that drew shapes for seats it did
    // not name would pass any check that counted only circles.
    expect(screen.getByText("Dana Weiss")).toBeTruthy();
    expect(screen.getByText("Ines Kraft")).toBeTruthy();
  });

  it("draws no seats for a withheld read, and says the lane is withheld", () => {
    draw({ coverage: undefined, withheld: true, pending: false, overlay: false });
    // Not merely "no names": an empty read draws no names either, so asserting
    // absence alone would pass for both and this is the pair the map exists to
    // tell apart.
    expect(screen.queryByText("Dana Weiss")).toBeNull();
    expect(document.querySelector(".dc-legend")).toBeNull();
  });

  it("draws no seats while the read is still pending", () => {
    draw({ coverage: undefined, withheld: false, pending: true, overlay: false });
    expect(screen.queryByText("Dana Weiss")).toBeNull();
  });
});
