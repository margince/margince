/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { formatDateAbbrev, formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meetingRow, readingsDay } from "./brief.fixtures";
import { SchedulePanel, scheduleIsEmpty } from "./brief.schedule";
import type { WorklistItem } from "./worklist.queries";

// The two rail panels, and what each of them refuses to claim.
//
// Both are CUTS of the one worklist answer the work column is drawn from, so
// what they are really about is which rows each one owns and what it says on a
// day with none of them.

function taskRow(id: string, title: string): WorklistItem {
  return {
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
  subject: "Weber kickoff",
  linked_deal_id: "deal-1",
  participants: [{ contact_id: "contact-1", full_name: "Anna Weber" }],
};

afterEach(cleanup);

describe("the schedule panel", () => {
  it("lists the day's meetings in the order the server sent them", () => {
    const day = readingsDay({}, [
      meetingRow("m1", true),
      taskRow("t1", "Call Alice back"),
      meetingRow("m2", false),
    ]);
    draw(<SchedulePanel day={day} state="ready" />);

    const panel = screen.getByRole("region");
    // The task belongs to the OTHER panel: a rail that listed everything would
    // be the queue again, in a narrower column.
    expect(within(panel).queryByText("Call Alice back")).toBeNull();
    expect(within(panel).getAllByText(/Weber GmbH/)).toHaveLength(2);
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
    const zone = viewerZone();
    const day = readingsDay({}, [
      meetingRow("m1", true, "2026-09-03T03:40:00Z"),
      meetingRow("m2", true, "2026-09-03T13:15:00Z"),
    ]);
    draw(<SchedulePanel day={day} state="ready" />);

    const times = screen
      .getByRole("region")
      .querySelectorAll(".rail-schedule-when");
    expect(Array.from(times, (cell) => cell.textContent)).toEqual([
      formatTimeOfDay("2026-09-03T03:40:00Z", "en", zone),
      formatTimeOfDay("2026-09-03T13:15:00Z", "en", zone),
    ]);
    // Two DIFFERENT times, so a cell that rendered a constant — or the same
    // row twice — cannot pass the comparison above.
    expect(times[0].textContent).not.toBe(times[1].textContent);
    expect(times[0].textContent).not.toBe("");
  });

  // A CLEAR DAY IS NOT A PANEL. The header band, the hairline and one grey
  // sentence stood in the rail beside the panels that had news, and the news
  // was read last. The rail's quiet panel carries the line instead
  // (brief.rail.tsx), and `scheduleIsEmpty` is the one predicate both turn on.
  it("draws nothing at all on a day with no meeting", () => {
    const { container } = draw(
      <SchedulePanel day={readingsDay({}, [])} state="ready" />,
    );

    expect(container.innerHTML).toBe("");
    expect(scheduleIsEmpty(readingsDay({}, []), "ready")).toBe(true);
    const measured = { ...readingsDay({}, []), calendar: "connected" as const };
    expect(scheduleIsEmpty(measured, "ready")).toBe(true);
  });

  // A quiet day with a meeting booked further out says when, on the day's own
  // line, with a date where today's rows carry a time.
  it("draws the next booked meeting on a day with none left", () => {
    const zone = viewerZone();
    const day = {
      ...readingsDay({}, []),
      calendar: "connected" as const,
      next_meeting: NEXT,
    };
    draw(<SchedulePanel day={day} state="ready" />);

    const panel = screen.getByRole("region");
    expect(panel.querySelector(".rail-schedule-when")?.textContent).toBe(
      formatDateAbbrev(NEXT.starts_at, "en", zone),
    );
    expect(
      within(panel)
        .getByRole("link", { name: "Weber kickoff" })
        .getAttribute("href"),
    ).toContain("deal-1");
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
    expect(scheduleIsEmpty(day, "ready")).toBe(false);
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
      expect(panel.querySelector(".rail-schedule-row")).toBeNull();
      expect(
        within(panel)
          .getByRole("link", { name: en[verb] })
          .getAttribute("href"),
      ).toBe("#/settings/connections");
      expect(scheduleIsEmpty(day, "ready")).toBe(false);
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
    expect(rows[0].textContent).toContain(en["brief.schedule.notConnected"]);
    expect(rows[1].classList.contains("rail-schedule-row")).toBe(true);
    expect(
      within(panel).getByRole("link", { name: en["brief.schedule.connect"] }),
    ).toBeTruthy();
    expect(
      within(panel).getByRole("link", { name: "Weber kickoff" }),
    ).toBeTruthy();
  });

  // A read that has not landed is not a clear day. Collapsing here would send a
  // rep into a morning believing nothing was booked, so the panel keeps its box
  // and draws the pending state its own title names.
  it("keeps its box, and claims nothing, before the read lands", () => {
    draw(<SchedulePanel day={undefined} state="loading" />);

    expect(
      screen.getByRole("region", { name: en["brief.panel.schedule"] }),
    ).toBeTruthy();
    expect(scheduleIsEmpty(undefined, "loading")).toBe(false);
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
