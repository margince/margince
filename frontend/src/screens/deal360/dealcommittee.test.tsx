/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { CommitteeReading } from "./dealcommittee";

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

function draw(props: Parameters<typeof CommitteeReading>[0]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrap = (node: ReactNode) => (
    <QueryClientProvider client={client}>
      <LocaleProvider>{node}</LocaleProvider>
    </QueryClientProvider>
  );
  return render(wrap(<CommitteeReading {...props} />));
}

afterEach(cleanup);

describe("the buying committee, drawn", () => {
  it("draws one seat per stakeholder and names none of them", () => {
    draw({
      coverage: coverage(),
      withheld: false,
      pending: false,
    });
    // The reading is the PICTURE only. Naming the seats here is what made the
    // page carry three lists of one committee, so the names belong to the
    // table this map stands on (dealcommitteecard.tsx) and the count is what proves
    // the map did not quietly drop a seat it was given.
    expect(document.querySelectorAll(".dc-seat").length).toBe(
      // Our own node is a seat circle too, so the buyer's seats plus ours.
      coverage().stakeholders.length + 1,
    );
    expect(screen.queryByText("Dana Weiss")).toBeNull();
    expect(screen.queryByText("Ines Kraft")).toBeNull();
  });

  it("says how many of ours carry the deal, which the picture can only size", () => {
    draw({ coverage: coverage(), withheld: false, pending: false });
    expect(screen.getByText("1 colleague on this deal")).toBeTruthy();
    cleanup();
    const [lena] = coverage().our_side;
    const twoOfUs = coverage({ our_side: [lena, { ...lena, user_id: "u2" }] });
    draw({ coverage: twoOfUs, withheld: false, pending: false });
    expect(screen.getByText("2 colleagues on this deal")).toBeTruthy();
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
    });
    expect(screen.getByText("Hidden for your role")).toBeTruthy();
    expect(screen.queryByText("No stakeholders on this deal")).toBeNull();
    expect(screen.queryByText("Dana Weiss")).toBeNull();
  });

  it("says the read is still loading rather than showing it as empty", () => {
    draw({
      coverage: undefined,
      withheld: false,
      pending: true,
    });
    // The busy region rather than its label: the label lands in an sr-only
    // span or a visible note depending on the caller, and asserting the one
    // this caller happens to choose would break on a presentational change
    // that leaves the state correct.
    expect(
      document.querySelector('[role="status"][aria-busy="true"]'),
    ).toBeTruthy();
    expect(screen.queryByText("No stakeholders on this deal")).toBeNull();
    expect(screen.queryByText("Dana Weiss")).toBeNull();
  });

  // The empty case is the ONE state this reading stays silent for, and the
  // silence is only safe because the stakeholder rows beneath it say the
  // sentence instead. It is asserted against the withheld case deliberately:
  // "nobody is on this deal" and "you may not see who is" are opposite claims,
  // so a regression that let withheld fall through to silence would leave a
  // reader with no seats and no reason, and an absence-only check would stay
  // green through it.
  it("draws nothing when the read is simply empty, leaving the rows to say so", () => {
    const { container } = draw({
      coverage: coverage({ stakeholders: [] }),
      withheld: false,
      pending: false,
    });
    expect(container.firstChild).toBeNull();
    expect(screen.queryByText("No stakeholders on this deal")).toBeNull();
  });

  it("still names the withheld case rather than falling silent with it", () => {
    const { container } = draw({
      coverage: undefined,
      withheld: true,
      pending: false,
    });
    expect(container.firstChild).not.toBeNull();
    expect(screen.getByText("Hidden for your role")).toBeTruthy();
  });
});
