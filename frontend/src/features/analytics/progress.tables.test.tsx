// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { render } from "./analytics.testkit";
import { sellerMarch, teamMarch } from "./progress.fixtures";
import { ProgressMembers, ProgressMonths } from "./progress.tables";

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

describe("ProgressMembers", () => {
  it("lists each member and opens a seller's own view from their row", async () => {
    window.location.hash = "#/progress";
    const user = userEvent.setup();
    render(<ProgressMembers progress={teamMarch} />);
    const table = screen.getByRole("table", { name: "Team" });
    expect(within(table).getByText("Alex Brandt")).toBeTruthy();
    await user.click(within(table).getByText("Sam Okafor"));
    expect(window.location.hash).toContain(
      `scope=owner%3A${teamMarch.members[1]?.owner_id}`,
    );
  });

  it("says members are withheld and lists none it may not show", () => {
    render(
      <ProgressMembers
        progress={{ ...teamMarch, members: [], members_withheld: true }}
      />,
    );
    expect(screen.queryByRole("table")).toBeNull();
    expect(
      screen.getByText(
        "Some members are hidden, so the team goal is hidden too.",
      ),
    ).toBeTruthy();
  });
});

describe("ProgressMonths", () => {
  const sellerYear = {
    ...sellerMarch,
    period: { kind: "year" as const, year: 2026 },
  };

  it("compares each month and sums the year so far against its goals", () => {
    render(<ProgressMonths progress={sellerYear} />);
    const table = screen.getByRole("table", { name: "Months of 2026" });
    expect(within(table).getByText("January")).toBeTruthy();
    // Nine months are still ahead: neither their revenue nor their difference is read.
    expect(within(table).getAllByText("Not started")).toHaveLength(18);
    expect(
      screen.getByText("2026 so far: €227,000 of €1,280,000 (18%)"),
    ).toBeTruthy();
  });

  it("leaves the year line to the year reading, which knows of a changed plan", () => {
    render(<ProgressMonths progress={sellerMarch} />);
    expect(screen.queryByText(/so far:/)).toBeNull();
  });

  it("drops the year line while a month has no goal", () => {
    const months = sellerMarch.months.map((row, at) =>
      at === 5
        ? {
            ...row,
            goal: {
              status: "missing" as const,
              months_set: 0,
              months_needed: 1,
            },
          }
        : row,
    );
    render(<ProgressMonths progress={{ ...sellerYear, months }} />);
    expect(screen.queryByText(/so far:/)).toBeNull();
  });

  it("drops the year line while a plan changed elsewhere awaits review", () => {
    const goal = {
      ...sellerMarch.goal,
      status: "changed_outside_plan" as const,
    };
    render(<ProgressMonths progress={{ ...sellerYear, goal }} />);
    expect(screen.queryByText(/so far:/)).toBeNull();
  });

  it("drops the year line while a month's revenue is withheld", () => {
    const months = sellerMarch.months.map((row, at) =>
      at === 0 ? { ...row, closed: { status: "withheld" as const } } : row,
    );
    render(<ProgressMonths progress={{ ...sellerYear, months }} />);
    expect(screen.queryByText(/so far:/)).toBeNull();
  });
});
