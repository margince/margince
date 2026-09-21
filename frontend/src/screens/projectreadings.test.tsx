/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { RollupsStrip } from "./projectreadings";
import { project360 } from "./projects.fixtures";
import { StoryProviders } from "./story-utils";

// The project's readings plate states ONE reading per fact. The activity feed
// was answered twice — how much is filed, and when the last of it landed —
// side by side, opening the same anchor and neither saying what the other
// could not. This file holds the merged slot to that.

afterEach(cleanup);

function plate(view: ReturnType<typeof project360>): HTMLElement {
  render(
    <StoryProviders>
      <RollupsStrip view={view} />
    </StoryProviders>,
  );
  return screen.getByTestId("project-rollups");
}

function activity(strip: HTMLElement): HTMLElement {
  const card = within(strip).getByText("Activity").closest(".stat-card");
  if (!(card instanceof HTMLElement)) {
    throw new Error("the activity reading has no card");
  }
  return card;
}

describe("the activity feed is one reading, not two", () => {
  it("draws four slots, with the count as the reading and the date under it", () => {
    const strip = plate(project360());

    expect(strip.childElementCount).toBe(4);
    const card = activity(strip);
    expect(within(card).getByText("142 filed")).toBeTruthy();
    expect(card.textContent).toMatch(/Last · /);
    // The two readings that were merged away left no second slot behind.
    expect(within(strip).queryByText("Last activity")).toBeNull();
    expect(within(strip).queryByText("Activities")).toBeNull();
  });

  // Zero is a READING here, and "None" is the word the whole product uses for
  // it. The slot used to say a lower-case fragment.
  it("says nothing is filed rather than standing a bare zero there", () => {
    const strip = plate(
      project360({
        rollups: {
          open_deal_value: { amount_minor: 0, currency: "EUR" },
          won_deal_value: { amount_minor: 0, currency: "EUR" },
          open_commitments: 0,
          last_activity_at: null,
          activity_count: 0,
        },
      }),
    );
    const card = activity(strip);

    expect(within(card).getByText("None")).toBeTruthy();
    expect(card.querySelector(".stat-card-detail")).toBeNull();
  });

  // The count and the date move on different rules, so a filed count with no
  // date is a shape the payload can really take. The count still stands as
  // the reading; the qualifier simply has nothing to add.
  it("reports the count with no date when the feed names none", () => {
    const strip = plate(
      project360({
        rollups: {
          open_deal_value: { amount_minor: 1_200_000, currency: "EUR" },
          won_deal_value: { amount_minor: 450_000, currency: "EUR" },
          open_commitments: 4,
          last_activity_at: null,
          activity_count: 7,
        },
      }),
    );
    const card = activity(strip);

    expect(within(card).getByText("7 filed")).toBeTruthy();
    expect(card.querySelector(".stat-card-detail")).toBeNull();
  });

  // Both money slots and the commitments count still say their own absences
  // in their own words: "no deal open" and "nothing won yet" are different
  // facts, and a zero count of commitments is a number rather than an absence.
  it("keeps the other three readings saying their own absences", () => {
    const strip = plate(
      project360({
        rollups: {
          open_deal_value: { amount_minor: null, currency: null },
          won_deal_value: { amount_minor: null, currency: null },
          open_commitments: 0,
          last_activity_at: null,
          activity_count: 0,
        },
      }),
    );

    expect(within(strip).getByText("None yet")).toBeTruthy();
    expect(within(strip).getAllByText("None").length).toBe(2);
    expect(within(strip).getByText("0")).toBeTruthy();
  });
});
