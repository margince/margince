/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { boundedMeetings, meetingRow, readingsDay } from "./home.fixtures";
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
