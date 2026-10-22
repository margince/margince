// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider, translate } from "../../i18n";
import { en } from "../../i18n/en";
import type { Worklist } from "./worklist.queries";
import { reachNames } from "./worklist.sources";
import { DayUnread, dayPartlyRead } from "./worklist.unread";

// An unread teammate's plan is the same claim as an unread source: the day may
// not read as clear, and the notice says whose plan it was.

afterEach(cleanup);

function aDay(planCoverage?: Worklist["plan_coverage"]): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "team",
    scope_options: ["mine", "team"],
    summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    reach: [],
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
    counts: [],
    queue: [],
    plan_coverage: planCoverage,
  };
}

function draw(day: Worklist) {
  return render(
    <LocaleProvider initial="en">
      <DayUnread day={day} />
    </LocaleProvider>,
  );
}

const ana = { user_id: "id-ana", display_name: "Ana", read: false };
const ben = { user_id: "id-ben", display_name: "Ben", read: true };

describe("what a team day did not read", () => {
  it("warns under the partial heading and names the unread teammate", () => {
    const day = aDay({ members: [ana, ben], truncated: false });
    draw(day);

    expect(screen.getByText(en["worklist.partialTitle"])).toBeTruthy();
    expect(
      screen.getByText(
        "Weekly plans read for 1 of 2 teammates. Not read: Ana.",
      ),
    ).toBeTruthy();
    expect(dayPartlyRead(day)).toBe(true);
  });

  it("says quietly whose plans it covered when every plan was read", () => {
    const day = aDay({ members: [ben], truncated: false });
    draw(day);

    expect(
      screen.getByText("Weekly plan read for the team’s one member."),
    ).toBeTruthy();
    expect(screen.queryByText(en["worklist.partialTitle"])).toBeNull();
    expect(dayPartlyRead(day)).toBe(false);
  });

  it("draws nothing when the day made no team plan read", () => {
    const { container } = draw(aDay());

    expect(container.innerHTML).toBe("");
  });
});

describe("source read limits remain visible after page continuation ends", () => {
  it("names each bounded source and preserves personal scope within a team day", () => {
    const day = aDay();
    day.reach = [
      {
        source: "task",
        considered: 12,
        shown: 12,
        more_available: true,
        personal: false,
      },
      {
        source: "notice",
        considered: 30,
        shown: 30,
        more_available: true,
        personal: true,
      },
    ];
    draw(day);
    expect(dayPartlyRead(day)).toBe(true);
    expect(
      screen.getByText(/read limit reached · the selected record scope/),
    ).toBeTruthy();
    expect(
      screen.getByText(/read limit reached · your personal source/),
    ).toBeTruthy();
    expect(screen.getByText(en["worklist.limits.boundary"])).toBeTruthy();
    expect(screen.getByText(en["worklist.limits.remedy"])).toBeTruthy();
  });
  it("retains an explicit boundary for an older response without source reach", () => {
    const day = aDay();
    Reflect.deleteProperty(day, "reach");
    day.counts = [
      { category: "tasks", considered: 12, shown: 12, more_available: true },
    ];
    draw(day);
    expect(dayPartlyRead(day)).toBe(true);
    expect(screen.getByText(en["worklist.limits.boundary"])).toBeTruthy();
  });
});

it("gives every registered source an explicit boundary and remedy when bounded", () => {
  const day = aDay();
  // The label catalog must cover the contract's complete source enum.
  for (const [source, label] of Object.entries(reachNames)) {
    if (!isReachSource(source)) continue;
    day.reach = [{ source, considered: 1, shown: 1, more_available: true }];
    const { unmount } = draw(day);
    expect(screen.getByText(en["worklist.limits.boundary"])).toBeTruthy();
    expect(screen.getByText(en["worklist.limits.remedy"])).toBeTruthy();
    expect(
      screen.getByText(
        `${translate("en", label)}: read limit reached · the selected record scope`,
      ),
    ).toBeTruthy();
    unmount();
  }
});

function isReachSource(
  source: string,
): source is NonNullable<Worklist["reach"]>[number]["source"] {
  return source in reachNames;
}

it("retains full plan coverage beside a routine source limit", () => {
  const day = aDay({
    members: [
      { ...ana, read: true },
      { ...ben, read: true },
    ],
    truncated: false,
  });
  day.reach = [
    { source: "task", considered: 2, shown: 2, more_available: true },
  ];
  draw(day);
  expect(
    screen.getByText("Weekly plans read for all 2 teammates."),
  ).toBeTruthy();
  expect(screen.getByText(en["worklist.limits.title"])).toBeTruthy();
  expect(screen.queryByText(en["worklist.partialTitle"])).toBeNull();
});
