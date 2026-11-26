/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDateAbbrev, formatTimeOfDay } from "../../format/format";
import { browserZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meetingRow, readingsDay } from "../../testing/brief.fixtures";
import type { WorklistItem } from "../worklist/worklist.queries";
import { SchedulePanel } from "./brief.schedule";

// External meetings is a cut of the one worklist answer the work column is
// drawn from. It lists the meetings by start, each saying who it is with.

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
// The mailbox roster says whether this reader may connect a calendar at all,
// and an unanswered one has not said yet.
type Roster = "captures" | "refused" | "unanswered";

function draw(node: React.ReactNode, roster: Roster = "captures") {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      roster === "unanswered"
        ? new Promise<Response>(() => {})
        : Promise.resolve(
            Response.json({
              data: [],
              providers: [],
              seat_capture: { allowed: roster === "captures" },
            }),
          ),
    ),
  );
  const client = new QueryClient();
  return render(
    <QueryClientProvider client={client}>
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

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

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
    expect(what).toHaveLength(2);
    expect(what[0]).toContain("Sooner");
    expect(what[1]).toContain("Later");
  });

  // No row claims the reader is free: an internal meeting is not on this
  // panel, so a gap between two rows is not free time. Only the next meeting
  // stands on its plate, says how soon it starts and opens its brief.
  it("sets the next meeting on its plate", () => {
    const day = readingsDay({}, [
      { ...meetingRow("m1", true, "2026-09-03T08:00:00Z"), title: "Started" },
      { ...meetingRow("m2", true, "2026-09-03T10:00:00Z"), title: "Next up" },
    ]);
    const prepared: string[] = [];
    draw(
      <SchedulePanel
        day={day}
        state="ready"
        now={Date.parse("2026-09-03T09:40:00Z")}
        onPrepareMeeting={(id) => prepared.push(id)}
      />,
    );

    const rows = screen
      .getByRole("region")
      .querySelectorAll(".rail-schedule-row");
    expect(rows).toHaveLength(2);
    expect(rows[0].textContent).toContain("Started");
    expect(rows[1].textContent).toContain("in 20 minutes");
    expect(rows[0].classList.contains("rail-schedule-next")).toBe(false);
    expect(rows[1].classList.contains("rail-schedule-next")).toBe(true);
    // Only the next meeting offers a brief, and pressing it asks for that one.
    const prepare = en["worklist.verb.open_meeting_brief"];
    expect(within(rows[0] as HTMLElement).queryByText(prepare)).toBeNull();
    fireEvent.click(
      within(rows[1] as HTMLElement).getByRole("button", { name: prepare }),
    );
    expect(prepared).toEqual(["m2"]);
  });

  // A meeting's title is what the calendar called it. The row adds the account
  // and the outside contacts, each a link, and how many more there are.
  it("names the account and the contacts each meeting is with", () => {
    const meeting: WorklistItem = {
      ...meetingRow("m1", true),
      contact: {
        id: "c-1",
        label: "Anna Weber",
        employer: { company_id: "co-1", company_name: "Weber GmbH" },
      },
      room: {
        participants: [
          { contact_id: "c-1", full_name: "Anna Weber" },
          { contact_id: "c-2", full_name: "Ben Ortiz" },
        ],
        other_participants: [
          { kind: "colleague", name: "Rita Rep" },
          { kind: "address", name: "cfo@weber.example" },
        ],
        participants_more: 3,
      },
    };
    draw(
      <SchedulePanel
        day={readingsDay({}, [meeting])}
        state="ready"
        now={EARLY}
      />,
    );

    const panel = screen.getByRole("region");
    // Colleagues are on every meeting here, so the row leaves them out.
    const links = within(panel).getAllByRole("link");
    expect(links.map((link) => link.textContent)).toEqual([
      "WGWeber GmbH",
      "AWAnna Weber",
      "BOBen Ortiz",
    ]);
    expect(
      panel.querySelector(".rail-schedule-attendees")?.textContent,
    ).toContain("cfo@weber.example and 3 more");
    expect(panel.textContent).not.toContain("Rita Rep");
    expect(
      within(panel)
        .getByRole("link", { name: "Weber GmbH" })
        .getAttribute("href"),
    ).toContain("co-1");
    expect(
      within(panel)
        .getByRole("link", { name: "Ben Ortiz" })
        .getAttribute("href"),
    ).toContain("c-2");
  });

  it("lets every attendee's name wrap inside the panel rather than run past it", () => {
    draw(
      <SchedulePanel
        day={readingsDay({}, [
          {
            ...meetingRow("m1", true),
            contact: {
              id: "c-1",
              label: "Anna Weber",
              employer: { company_id: "co-1", company_name: "Weber GmbH" },
            },
          },
        ])}
        state="ready"
        now={EARLY}
      />,
    );

    const chips = within(screen.getByRole("region")).getAllByRole("link");
    expect(chips.map((chip) => chip.classList.contains("chip-wrap"))).toEqual([
      true,
      true,
    ]);
  });

  // A row whose room did not come still names the counterparty the lane chose.
  it("names the meeting's contact when the row carries no room", () => {
    const meeting: WorklistItem = {
      ...meetingRow("m1", true),
      contact: { id: "c-1", label: "Anna Weber" },
    };
    draw(
      <SchedulePanel
        day={readingsDay({}, [meeting])}
        state="ready"
        now={EARLY}
      />,
    );

    expect(
      within(screen.getByRole("region"))
        .getByRole("link", { name: "Anna Weber" })
        .getAttribute("href"),
    ).toContain("c-1");
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
      .querySelectorAll(".rail-schedule-row .rail-schedule-when");
    expect(Array.from(times, (cell) => cell.textContent)).toEqual([
      formatTimeOfDay("2026-09-03T03:40:00Z", "en", zone),
      formatTimeOfDay("2026-09-03T13:15:00Z", "en", zone),
    ]);
    // Two DIFFERENT times, so a cell that rendered a constant — or the same
    // row twice — cannot pass the comparison above.
    expect(times[0].textContent).not.toBe(times[1].textContent);
    expect(times[0].textContent).not.toBe("");
  });

  // A clear day keeps the box and says no external meeting is left, so a
  // reader never wonders whether the meetings failed to load.
  it("says the rest of a day with no external meeting is clear", () => {
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
      panel.querySelector(".rail-schedule-row .rail-schedule-when")
        ?.textContent,
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
    async (calendar, sentence, verb) => {
      const day = { ...readingsDay({}, []), calendar };
      draw(<SchedulePanel day={day} state="ready" />);

      const panel = screen.getByRole("region");
      expect(panel.textContent).toContain(en[sentence]);
      // No meeting row, and no claim about the rest of the day.
      expect(panel.querySelectorAll(".rail-schedule-row")).toHaveLength(0);
      expect(panel.textContent).not.toContain(en["brief.schedule.clearRest"]);
      const link = await within(panel).findByRole("link", { name: en[verb] });
      expect(link.getAttribute("href")).toBe("/#/settings/connections");
    },
  );

  // Before the roster answers, nothing has said this reader may connect. A
  // member whose capture will be refused is not sent to the setting meanwhile.
  it.each([
    ["not_connected", "brief.schedule.notConnected"],
    ["unreadable", "brief.schedule.unreadable"],
  ] as const)(
    "offers no link for a %s calendar before the roster answers",
    (calendar, sentence) => {
      const day = { ...readingsDay({}, []), calendar };
      draw(<SchedulePanel day={day} state="ready" />, "unanswered");

      const panel = screen.getByRole("region");
      expect(panel.textContent).toContain(en[sentence]);
      expect(within(panel).queryByRole("link")).toBeNull();
    },
  );

  // A seat or role that may not capture (Read-only, say) hears why the panel
  // is empty. It is not sent to a setting it cannot change.
  it.each([
    ["not_connected", "brief.schedule.notConnected"],
    ["unreadable", "brief.schedule.unreadable"],
  ] as const)(
    "offers a reader who may not connect no link for a %s calendar",
    async (calendar, sentence) => {
      const day = { ...readingsDay({}, []), calendar };
      draw(<SchedulePanel day={day} state="ready" />, "refused");

      const panel = screen.getByRole("region");
      expect(panel.textContent).toContain(en[sentence]);
      await within(panel).findByText(en["connectors.roleCannotCapture"], {
        exact: false,
      });
      expect(within(panel).queryByRole("link")).toBeNull();
    },
  );

  // Today's meeting known to Margince shows under the sentence that says where
  // the panel's meetings come from when no calendar is connected.
  it("heads a no-calendar day's meetings with where they come from", () => {
    const day = {
      ...readingsDay({}, [{ ...meetingRow("m1", true), title: "Site visit" }]),
      calendar: "not_connected" as const,
    };
    draw(<SchedulePanel day={day} state="ready" />);

    const panel = screen.getByRole("region");
    const rows = panel.querySelectorAll(".panel-row");
    expect(rows[0].textContent).toContain(en["brief.schedule.notConnected"]);
    expect(rows[1].textContent).toContain("Site visit");
    expect(panel.querySelectorAll(".rail-schedule-row")).toHaveLength(1);
  });

  // A meeting booked by hand is a fact the CRM holds whatever the calendar
  // says, so it follows the sentence instead of hiding behind it.
  it("names a hand-booked next meeting under a calendar that cannot count", async () => {
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
      await within(panel).findByRole("link", {
        name: en["brief.schedule.connect"],
      }),
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

it("lists the next meeting's outside attendees as one sentence", () => {
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
  // One element holds names and commas, so they wrap as text, not as items.
  const attendees = screen
    .getByRole("region")
    .querySelector(".rail-schedule-attendees");
  expect(
    within(attendees as HTMLElement)
      .getAllByRole("link")
      .map((link) => link.getAttribute("href")),
  ).toEqual(["/#/contacts/c-1", "/#/contacts/c-2"]);
  expect(attendees?.textContent).not.toContain("Rita Rep");
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
