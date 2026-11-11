/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { formatDateAbbrev, formatTimeOfDay } from "../../format/format";
import { browserZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meetingRow, readingsDay } from "../../testing/brief.fixtures";
import type { WorklistItem } from "../worklist/worklist.queries";
import { SchedulePanel } from "./brief.schedule";

// Your day is a cut of the one worklist answer the work column is drawn from.
// It lays the meetings on one line with a Now stop, and says so on a clear day.

function taskRow(id: string, title: string): WorklistItem {
  return {
    occurred_precision: "instant",
    id,
    source: "task",
    level: 2,
    category: "tasks",
    title,
    because: [],
    consequence: "task_slips",
    actions: ["complete", "open"],
  };
}

// A QueryClient because the next meeting names its records through
// `EntityRef`, which reads a name it is not handed.
function draw(node: React.ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const NEXT = {
  activity_id: "a-next",
  starts_at: "2026-09-04T09:30:00Z",
  occurred_precision: "instant" as const,
  subject: "Weber kickoff",
  linked_deal_id: "deal-1",
  participants: [{ contact_id: "contact-1", full_name: "Anna Weber" }],
};

// Before the fixtures' default meeting start, 2026-09-03T09:30Z.
const EARLY = Date.parse("2026-09-03T03:00:00Z");

afterEach(cleanup);

describe("the schedule panel", () => {
  it("lays the day's meetings on the line by start, whatever the rank", () => {
    const day = readingsDay({}, [
      { ...meetingRow("m1", true, "2026-09-03T13:00:00Z"), title: "Later" },
      taskRow("t1", "Call Alice back"),
      { ...meetingRow("m2", false, "2026-09-03T09:00:00Z"), title: "Sooner" },
    ]);
    draw(<SchedulePanel day={day} state="ready" now={EARLY} />);

    const panel = screen.getByRole("region");
    // A task is the queue's: a rail that listed everything would be the queue
    // again, in a narrower column.
    expect(within(panel).queryByText("Call Alice back")).toBeNull();
    const what = Array.from(
      panel.querySelectorAll(".rail-schedule-what"),
      (cell) => cell.textContent ?? "",
    );
    expect(what[0]).toContain(en["brief.schedule.now"]);
    expect(what[1]).toContain("Sooner");
    expect(what[2]).toContain("Later");
  });

  // The Now stop sits where the clock is: after a meeting under way, before
  // the next one. Only the next one says how soon it starts and opens its brief.
  it("stands Now between the started meeting and the next one", () => {
    const day = readingsDay({}, [
      { ...meetingRow("m1", true, "2026-09-03T08:00:00Z"), title: "Started" },
      {
        ...meetingRow("m2", true, "2026-09-03T10:00:00Z"),
        title: "Next up",
        with_contact: "contact-1",
        move: { action: "open_meeting_brief", activity_id: "m2" },
      },
    ]);
    draw(
      <SchedulePanel
        day={day}
        state="ready"
        now={Date.parse("2026-09-03T09:40:00Z")}
      />,
    );

    const rows = screen
      .getByRole("region")
      .querySelectorAll(".rail-schedule-row");
    expect(rows[0].textContent).toContain("Started");
    expect(rows[1].classList.contains("rail-schedule-now")).toBe(true);
    // A meeting already under way has no known end, so no free time is claimed.
    expect(rows[1].textContent).not.toContain("Free until");
    expect(rows[2].classList.contains("rail-schedule-next")).toBe(true);
    expect(rows[2].textContent).toContain("in 20 minutes");
    expect(
      within(rows[2] as HTMLElement)
        .getByRole("link", { name: en["worklist.verb.open_meeting_brief"] })
        .getAttribute("href"),
    ).toContain("contact-1");
  });

  it("says the reader is free until the next meeting", () => {
    const zone = browserZone();
    draw(
      <SchedulePanel
        day={readingsDay({}, [meetingRow("m1", true)])}
        state="ready"
        now={EARLY}
      />,
    );

    expect(
      screen.getByText(
        en["brief.schedule.freeUntil"].replace(
          "{time}",
          formatTimeOfDay("2026-09-03T09:30:00Z", "en", zone),
        ),
      ),
    ).toBeTruthy();
  });

  // The badge is the panel's reason to exist: a rep scanning the rail for the
  // meeting to open before it starts must see it without reading further.
  it("badges the meeting nothing is prepared for, and only that one", () => {
    const day = readingsDay({}, [
      meetingRow("m1", false),
      meetingRow("m2", true),
    ]);
    draw(<SchedulePanel day={day} state="ready" />);

    expect(screen.getAllByText(en["worklist.needsPrep"])).toHaveLength(1);
  });

  // The panel is a list of TIMES. Without them it is the meetings count from
  // the readings strip again, in a wider column — and a rep is told they have a
  // meeting today and never told when.
  //
  // The start rides `due_at`, which is where the meeting lane puts it; the rows
  // carry no `occurred_at` at all, because a meeting on today's schedule has
  // not occurred yet. Reading that field drew every row with a blank gutter,
  // and no fixture gave a meeting a time, so the panel's own suite rendered the
  // empty branch and asserted around it.
  it("draws each meeting's start time", () => {
    const zone = browserZone();
    const day = readingsDay({}, [
      meetingRow("m1", true, "2026-09-03T03:40:00Z"),
      meetingRow("m2", true, "2026-09-03T13:15:00Z"),
    ]);
    draw(<SchedulePanel day={day} state="ready" now={EARLY} />);

    const times = screen
      .getByRole("region")
      .querySelectorAll(
        ".rail-schedule-row:not(.rail-schedule-now) .rail-schedule-when",
      );
    expect(Array.from(times, (cell) => cell.textContent)).toEqual([
      formatTimeOfDay("2026-09-03T03:40:00Z", "en", zone),
      formatTimeOfDay("2026-09-03T13:15:00Z", "en", zone),
    ]);
    // Two DIFFERENT times, so a cell that rendered a constant — or the same
    // row twice — cannot pass the comparison above.
    expect(times[0].textContent).not.toBe(times[1].textContent);
    expect(times[0].textContent).not.toBe("");
  });

  // A clear day keeps the box. The Now stop says the rest of the day is
  // clear, so a reader never wonders whether the meetings failed to load.
  it("says the rest of a day with no meeting is clear", () => {
    const measured = { ...readingsDay({}, []), calendar: "connected" as const };
    for (const day of [readingsDay({}, []), measured]) {
      draw(<SchedulePanel day={day} state="ready" now={EARLY} />);
      const panel = screen.getByRole("region", {
        name: en["brief.panel.schedule"],
      });
      expect(panel.textContent).toContain(en["brief.schedule.clearRest"]);
      expect(panel.textContent).not.toContain(en["brief.schedule.more"]);
      cleanup();
    }
  });

  // A quiet day with a meeting booked further out says when, on the day's own
  // line, with a date where today's rows carry a time.
  it("draws the next booked meeting on a day with none left", () => {
    const zone = browserZone();
    const day = {
      ...readingsDay({}, []),
      calendar: "connected" as const,
      next_meeting: NEXT,
    };
    const opened: string[] = [];
    draw(
      <SchedulePanel
        day={day}
        state="ready"
        onOpenMeeting={(id) => opened.push(id)}
      />,
    );

    const panel = screen.getByRole("region");
    expect(
      panel.querySelector(
        ".rail-schedule-row:not(.rail-schedule-now) .rail-schedule-when",
      )?.textContent,
    ).toBe(formatDateAbbrev(NEXT.starts_at, "en", zone));
    // The title opens the meeting itself, never the deal it is filed on.
    within(panel).getByRole("button", { name: "Weber kickoff" }).click();
    expect(opened).toEqual([NEXT.activity_id]);
    expect(
      within(panel)
        .getByRole("link", { name: "Anna Weber" })
        .getAttribute("href"),
    ).toContain("contact-1");
    expect(
      within(panel).getByText(
        en["brief.schedule.nextAt"].replace(
          "{time}",
          formatTimeOfDay(NEXT.starts_at, "en", zone),
        ),
      ),
    ).toBeTruthy();
  });

  // A calendar that could not count keeps the panel, says why, and leads to
  // where it is connected. Collapsing it would read as a clear morning.
  it.each([
    ["not_connected", "brief.schedule.notConnected", "brief.schedule.connect"],
    ["unreadable", "brief.schedule.unreadable", "brief.schedule.reconnect"],
  ] as const)(
    "says a %s calendar counted nothing",
    (calendar, sentence, verb) => {
      const day = { ...readingsDay({}, []), calendar };
      draw(<SchedulePanel day={day} state="ready" />);

      const panel = screen.getByRole("region");
      expect(panel.textContent).toContain(en[sentence]);
      // Only the Now stop, which claims nothing about the rest of the day.
      const rows = panel.querySelectorAll(".rail-schedule-row");
      expect(rows).toHaveLength(1);
      expect(rows[0].textContent).not.toContain(en["brief.schedule.clearRest"]);
      expect(
        within(panel)
          .getByRole("link", { name: en[verb] })
          .getAttribute("href"),
      ).toBe("/#/settings/connections");
    },
  );

  // A meeting booked by hand is a fact the CRM holds whatever the calendar
  // says, so it follows the sentence instead of hiding behind it.
  it("names a hand-booked next meeting under a calendar that cannot count", () => {
    const day = {
      ...readingsDay({}, []),
      calendar: "not_connected" as const,
      next_meeting: NEXT,
    };
    draw(<SchedulePanel day={day} state="ready" />);

    const panel = screen.getByRole("region");
    const rows = panel.querySelectorAll(".panel-row");
    expect(rows[0].classList.contains("rail-schedule-now")).toBe(true);
    expect(rows[1].textContent).toContain(en["brief.schedule.notConnected"]);
    expect(rows[2].classList.contains("rail-schedule-row")).toBe(true);
    expect(
      within(panel).getByRole("link", { name: en["brief.schedule.connect"] }),
    ).toBeTruthy();
    expect(within(panel).getByText("Weber kickoff")).toBeTruthy();
  });

  // A read that has not landed is not a clear day. Collapsing here would send a
  // rep into a morning believing nothing was booked, so the panel keeps its box
  // and draws the pending state its own title names.
  it("keeps its box, and claims nothing, before the read lands", () => {
    draw(<SchedulePanel day={undefined} state="loading" />);

    expect(
      screen.getByRole("region", { name: en["brief.panel.schedule"] }),
    ).toBeTruthy();
  });

  // Nor is a read that failed. Same box, and the failure said out loud.
  it("keeps its box when the read failed", () => {
    draw(<SchedulePanel day={undefined} state="failed" />);

    expect(
      screen.getByRole("region", { name: en["brief.panel.schedule"] }),
    ).toBeTruthy();
    expect(screen.getByText(en["state.failed"])).toBeTruthy();
  });
});

it("separates the next meeting's attendees with one comma each", () => {
  const day = {
    ...readingsDay({}, []),
    calendar: "connected" as const,
    next_meeting: {
      ...NEXT,
      participants: [
        { contact_id: "c-1", full_name: "Anna Weber" },
        { contact_id: "c-2", full_name: "Ben Ortiz" },
      ],
      other_participants: [{ kind: "colleague" as const, name: "Rita Rep" }],
    },
  };
  draw(<SchedulePanel day={day} state="ready" />);
  const row = screen
    .getByRole("region")
    .querySelector(
      ".rail-schedule-row:not(.rail-schedule-now) .rail-schedule-what",
    );
  expect(row?.textContent).toContain("Anna Weber, Ben Ortiz, Rita Rep");
  expect(row?.textContent).not.toMatch(/,\s*,|,\s*$/);
});

it("opens today's meeting from its title, not the record it is about", () => {
  const opened: string[] = [];
  const day = readingsDay({}, [meetingRow("m-today", true)]);
  draw(
    <SchedulePanel
      day={day}
      state="ready"
      onOpenMeeting={(id) => opened.push(id)}
    />,
  );
  screen.getByRole("button", { name: /Weber GmbH · quarterly review/ }).click();
  expect(opened).toEqual(["m-today"]);
});
