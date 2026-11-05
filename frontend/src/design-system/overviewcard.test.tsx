// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { OverviewCard, OverviewList, OverviewRow } from "./overviewcard";

afterEach(cleanup);

describe("Overview card", () => {
  it("is a region named by its title, with its rows as one list", () => {
    render(
      <OverviewCard
        title="Deals and projects"
        action={<a href="#d">All deals</a>}
      >
        <OverviewList>
          <OverviewRow
            title="Fleet retrofit"
            sub="Proposal"
            right="€48,000"
            figure
          />
          <OverviewRow title="Depot rollout" right="€16,500" figure />
        </OverviewList>
      </OverviewCard>,
    );
    const card = screen.getByRole("region", { name: "Deals and projects" });
    expect(card).toContainElement(
      screen.getByRole("link", { name: "All deals" }),
    );
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByText("€48,000")).toHaveClass("overviewrow-figure");
  });

  it("says why it is empty instead of drawing no rows", () => {
    render(
      <OverviewCard
        title="Offers"
        empty="No offer sent yet."
        emptyAction={<button type="button">New offer</button>}
      />,
    );
    expect(screen.getByText("No offer sent yet.")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "New offer" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("closes on one line about the whole card, in warning ink when asked", () => {
    render(
      <OverviewCard
        title="Contacts"
        foot="No economic buyer named yet"
        footTone="warn"
      >
        <OverviewList>
          <OverviewRow title="Dana Buyer" />
        </OverviewList>
      </OverviewCard>,
    );
    expect(screen.getByText("No economic buyer named yet")).toHaveClass(
      "overviewcard-foot-warn",
    );
  });
});
