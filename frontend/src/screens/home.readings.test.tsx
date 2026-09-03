/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import {
  boundedLeads,
  boundedMeetings,
  leadRow,
  meetingRow,
  readingsDay,
  wholeLeads,
} from "./home.fixtures";
import { HomeReadingsStrip } from "./home.readings";

// The Brief's readings strip, and the one claim it makes that the Worklist's
// own strip does not: the row is FIVE slots on every morning, including the two
// whose questions the product cannot answer yet.
//
// The interesting cases are all about what the strip refuses to say. A zero
// under "Promises due" would tell a reader they owe nobody anything, which
// nothing in the estate has standing to claim; a four-slot row on a quiet day
// would let yesterday's five be compared against today's four.

function draw(...args: Parameters<typeof readingsDay>) {
  return render(
    <LocaleProvider initial="en">
      <HomeReadingsStrip day={readingsDay(...args)} />
    </LocaleProvider>,
  );
}

// Five READINGS, not five DOM children: a strip that wrapped its slots in a
// container would satisfy a child count while drawing one card, and a strip of
// five empty boxes would satisfy it while drawing none.
function labels(): string[] {
  return [...screen.getByTestId("home-readings").querySelectorAll(".stat-card")]
    .map((card) => card.querySelector(".stat-card-label")?.textContent ?? "")
    .filter((text) => text !== "");
}

function meetingsCard(): HTMLElement {
  const card = screen
    .getByText(en["home.readings.meetings"])
    .closest(".stat-card");
  if (!(card instanceof HTMLElement)) {
    throw new Error("the meetings reading is not on the page");
  }
  return card;
}

afterEach(cleanup);

describe("the brief readings strip", () => {
  it("draws five slots on a full morning", () => {
    draw();

    expect(labels()).toHaveLength(5);
    expect(screen.getByText(en["home.readings.waiting"])).toBeTruthy();
    expect(screen.getByText(en["home.readings.meetings"])).toBeTruthy();
    expect(screen.getByText(en["home.readings.promises"])).toBeTruthy();
    expect(screen.getByText(en["home.readings.leads"])).toBeTruthy();
    expect(screen.getByText(en["home.readings.quota"])).toBeTruthy();
  });

  // The whole reason the slots are fixed. A row that shrank on a quiet day
  // would be compared against a fuller one and read as fewer questions asked.
  it("still draws five slots when nothing is waiting", () => {
    draw({ buyer_replies: 0, prospecting: 0 }, []);

    expect(labels()).toHaveLength(5);
    // A zero already reads as "none". What the line under it adds is the basis
    // — what the figure was taken over — which is the same on a quiet day as on
    // a busy one, so the row keeps its shape as well as its slot count.
    expect(screen.getByText(en["home.readings.waitingBasis"])).toBeTruthy();
    expect(screen.getByText(en["home.readings.leadsBasis"])).toBeTruthy();
    expect(screen.getByText(en["home.readings.meetingsBasis"])).toBeTruthy();
  });

  it("counts the meetings nothing is prepared for", () => {
    // Three meetings, two of them unprepared. The count of meetings comes from
    // `counts` and the readiness figure from the ROWS, so a fixture where they
    // agree is the one that proves both are read.
    draw({ buyer_replies: 0 }, [
      meetingRow("m1", false),
      meetingRow("m2", false),
      meetingRow("m3", true),
    ]);

    expect(meetingsCard().textContent).toContain("3");
    expect(meetingsCard().textContent).toContain(
      en["home.readings.needsPrep_other"].replace("{count}", "2"),
    );
  });

  // The absence of a warning has to read as an answer rather than as a gap: a
  // blank line under the meetings figure looks exactly like a line that failed
  // to render.
  it("says so when every meeting is prepared", () => {
    draw({}, [meetingRow("m1", true), meetingRow("m2", true)]);

    expect(screen.getByText(en["home.readings.prepared"])).toBeTruthy();
    expect(screen.queryByText(/needs prep/)).toBeNull();
  });

  // The defect this slot is shaped around. `considered` counts every meeting
  // read and ranked; readiness can only be counted off the rows the page
  // carries. Ten considered, three on the page, two of those unprepared would
  // read "10 · 2 need prep" — telling a rep eight meetings are ready when
  // nothing checked them.
  it("refuses a readiness figure when the page carries fewer than it counted", () => {
    draw({}, [meetingRow("m1", false)], [boundedMeetings(10, 1)]);

    expect(meetingsCard().textContent).toContain("10");
    expect(screen.getByText(en["home.readings.prepUnknown"])).toBeTruthy();
    // Not "1 needs prep", and not "all prepared": both are claims about ten
    // meetings made by looking at one.
    expect(screen.queryByText(/needs prep/)).toBeNull();
    expect(screen.queryByText(en["home.readings.prepared"])).toBeNull();
  });

  // A day whose counts carry no meetings entry at all. That is not an
  // unanswerable question — it is zero meetings, read whole — and saying "not
  // all could be checked" tells a rep the page failed at something it finished.
  it("reads a missing meetings count as a day with none, not an unknown", () => {
    draw({}, [], []);

    expect(meetingsCard().textContent).toContain("0");
    expect(screen.getByText(en["home.readings.meetingsBasis"])).toBeTruthy();
    expect(screen.queryByText(en["home.readings.prepUnknown"])).toBeNull();
  });

  // Two questions with no source. Drawing a zero would be a false answer, and
  // dropping the slot would be the page losing a question without saying so.
  it("names what it cannot measure instead of drawing a nought", () => {
    draw();

    expect(screen.getByText(en["home.readings.promisesBasis"])).toBeTruthy();
    expect(screen.getByText(en["home.readings.quotaBasis"])).toBeTruthy();
    expect(screen.getAllByText("—").length).toBe(2);
  });

  // The caveat belongs under the whole strip, the way the Worklist's strip
  // states it: put on one figure, it invites the reading where the rest are
  // exact.
  it("qualifies the whole row when a source was read to its limit", () => {
    draw({ more_available: true });

    const caveat = screen.getByText(en["home.readings.truncated"]);
    expect(caveat).toBeTruthy();
    // Outside the strip, not in a slot. Inside one, it reads as a caveat on that
    // figure alone and invites the reading where the other four are exact.
    expect(screen.getByTestId("home-readings").contains(caveat)).toBe(false);
  });

  it("says nothing about limits on a day it read whole", () => {
    draw();

    expect(screen.queryByText(en["home.readings.truncated"])).toBeNull();
  });
});

// The leads slot carries a second fact for the same reason the meetings slot
// does: how many are owed an answer does not tell a rep whether one is due
// before lunch.
describe("the leads reading", () => {
  function leadsCard(): HTMLElement {
    const card = screen
      .getByText(en["home.readings.leads"])
      .closest(".stat-card");
    if (!(card instanceof HTMLElement)) {
      throw new Error("the leads reading is not on the page");
    }
    return card;
  }

  it("names when the nearest answer is due", () => {
    draw(
      { prospecting: 2 },
      [leadRow("l1", "2026-08-31T15:00:00Z"), leadRow("l2")],
      [wholeLeads(2)],
    );

    expect(leadsCard().textContent).toContain("2");
    // The formatted moment, not the ISO string: the slot renders it in the
    // reader's own zone and this file does not own that format.
    expect(leadsCard().textContent).not.toContain("2026-08-31T15:00:00Z");
    expect(screen.queryByText(en["home.readings.leadsBasis"])).toBeNull();
  });

  it("names the earliest of several, not the first it meets", () => {
    draw(
      { prospecting: 2 },
      [
        leadRow("late", "2026-08-31T18:00:00Z"),
        leadRow("soon", "2026-08-31T09:00:00Z"),
      ],
      [wholeLeads(2)],
    );

    // Asserted by DIFFERENCE rather than by a literal clock reading: the two
    // moments are nine hours apart, so whichever zone the runtime is in, the
    // card must carry the earlier one's rendering and not the later one's. A
    // hard-coded hour would pin the test to the runner's own zone — the first
    // version of this line did, and read 09:00 as 16:00 under Asia/Ho_Chi_Minh.
    const earlier = formatDateTime("2026-08-31T09:00:00Z", "en", viewerZone());
    const later = formatDateTime("2026-08-31T18:00:00Z", "en", viewerZone());
    expect(leadsCard().textContent).toContain(earlier);
    expect(leadsCard().textContent).not.toContain(later);
  });

  // The defect this slot is shaped around, and the same one the meetings slot
  // guards: a cut read means an unshown lead could be due sooner than every one
  // the reader can see, so naming the earliest visible deadline states a "next"
  // that is not next.
  it("refuses a deadline when the page carries fewer leads than it counted", () => {
    draw(
      { prospecting: 9 },
      [leadRow("l1", "2026-08-31T15:00:00Z")],
      [boundedLeads(9, 1)],
    );

    expect(leadsCard().textContent).toContain("9");
    expect(screen.getByText(en["home.readings.leadsBasis"])).toBeTruthy();
  });

  // Every lead already overdue. There is no NEXT moment to name — they have all
  // passed — so the slot says what it counts rather than a date behind it.
  it("names no deadline when every lead is already overdue", () => {
    draw({ prospecting: 2 }, [leadRow("l1"), leadRow("l2")], [wholeLeads(2)]);

    expect(leadsCard().textContent).toContain("2");
    expect(screen.getByText(en["home.readings.leadsBasis"])).toBeTruthy();
  });
});
