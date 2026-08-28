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
    draw({
      coverage: coverage(),
      withheld: false,
      pending: false,
      overlay: false,
    });
    // The accessible list is the assertion rather than the SVG: a reader on a
    // screen reader gets the rows, and a map that drew shapes for seats it did
    // not name would pass any check that counted only circles.
    expect(screen.getByText("Dana Weiss")).toBeTruthy();
    expect(screen.getByText("Ines Kraft")).toBeTruthy();
  });

  // Each of the three no-seat states asserts ITS OWN sentence, not merely the
  // absence of seats. Absence is what all three share, so a check built on it
  // passes for every one of them — a withheld lane that regressed into an empty
  // one would say "no stakeholder is recorded" to a reader who is simply not
  // allowed to know, and every absence-only assertion would stay green.
  it("says the lane is withheld rather than showing it as empty", () => {
    draw({
      coverage: undefined,
      withheld: true,
      pending: false,
      overlay: false,
    });
    expect(
      screen.getByText("Hidden — your role cannot read this"),
    ).toBeTruthy();
    expect(
      screen.queryByText("No stakeholder is recorded on this deal"),
    ).toBeNull();
    expect(screen.queryByText("Dana Weiss")).toBeNull();
  });

  it("says the read is still loading rather than showing it as empty", () => {
    draw({
      coverage: undefined,
      withheld: false,
      pending: true,
      overlay: false,
    });
    // The busy region rather than its label: the label lands in an sr-only
    // span or a visible note depending on the caller, and asserting the one
    // this caller happens to choose would break on a presentational change
    // that leaves the state correct.
    expect(
      document.querySelector('[role="status"][aria-busy="true"]'),
    ).toBeTruthy();
    expect(
      screen.queryByText("No stakeholder is recorded on this deal"),
    ).toBeNull();
    expect(screen.queryByText("Dana Weiss")).toBeNull();
  });

  it("says the deal has no stakeholders when the read is simply empty", () => {
    draw({
      coverage: coverage({ stakeholders: [] }),
      withheld: false,
      pending: false,
      overlay: false,
    });
    expect(
      screen.getByText("No stakeholder is recorded on this deal"),
    ).toBeTruthy();
    expect(
      screen.queryByText("Hidden — your role cannot read this"),
    ).toBeNull();
  });
});
