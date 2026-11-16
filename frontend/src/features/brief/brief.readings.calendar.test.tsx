/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { formatDateAbbrev } from "../../format/format";
import { browserZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import {
  meetingRow,
  nextMeeting,
  readingsDay,
} from "../../testing/brief.fixtures";
import { BriefReadingsStrip } from "./brief.readings";

// The meetings reading under each state of the reader's calendar: what its
// figure counts, and where its door leads when the calendar left meetings out.

function drawDay(day: ReturnType<typeof readingsDay>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <BriefReadingsStrip day={day} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function meetingsCard(): HTMLElement {
  const card = screen
    .getByText(en["brief.readings.meetings"])
    .closest(".stat-card");
  if (!(card instanceof HTMLElement)) {
    throw new Error("the meetings reading is not on the page");
  }
  return card;
}

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

describe("the meetings reading and the calendar", () => {
  const NEXT_AT = "2026-09-04T09:30:00Z";

  // A zero from a calendar that cannot be read measured nothing, and drawing
  // it tells a rep their day is clear. The door goes where it reconnects.
  it("reads an unreadable calendar's zero as no measurement", async () => {
    const user = userEvent.setup();
    const next_meeting = nextMeeting("a-n", NEXT_AT);
    drawDay({
      ...readingsDay({}, [], []),
      calendar: "unreadable",
      next_meeting,
    });

    const value = en["brief.readings.calendarUnreadable"];
    // The why stays the line; the hand-booked meeting is the panel's to show.
    expect(meetingsCard().textContent).toContain(
      en["brief.readings.calendarUnreadableWhy"],
    );
    expect(meetingsCard().textContent).not.toContain("Next:");
    expect(meetingsCard().querySelector(".stat-card-value")?.textContent).toBe(
      value,
    );
    await user.click(
      screen.getByRole("button", {
        name: en["stat.open"],
        description: en["brief.readings.meetings"],
      }),
    );
    expect(window.location.hash).toContain("/settings/connections");
  });

  // Without a calendar Margince still knows its own meetings. The zero is a
  // count, and the hint leads to where a calendar connects.
  it("counts no meetings as 0 without a calendar, and keeps the connect hint", async () => {
    const user = userEvent.setup();
    const next_meeting = nextMeeting("a-n", NEXT_AT);
    drawDay({
      ...readingsDay({}, [], []),
      calendar: "not_connected",
      next_meeting,
    });

    expect(meetingsCard().querySelector(".stat-card-value")?.textContent).toBe(
      "0",
    );
    expect(meetingsCard().textContent).toContain(
      en["brief.readings.calendarNotConnectedWhy"],
    );
    await user.click(
      screen.getByRole("button", {
        name: en["stat.open"],
        description: en["brief.readings.meetings"],
      }),
    );
    expect(window.location.hash).toContain("/settings/connections");
  });

  it("counts today's meetings Margince knows without a calendar", () => {
    drawDay({
      ...readingsDay({}, [meetingRow("m1", true)]),
      calendar: "not_connected",
    });

    expect(meetingsCard().querySelector(".stat-card-value")?.textContent).toBe(
      "1",
    );
  });

  // The meetings booked by hand still count while the calendar cannot be
  // read. The figure is a minimum, and the line says how to make it whole.
  it("counts hand-booked meetings as a minimum while the calendar is unreadable", () => {
    drawDay({
      ...readingsDay({}, [meetingRow("m1", true)]),
      calendar: "unreadable",
    });

    expect(meetingsCard().querySelector(".stat-card-value")?.textContent).toBe(
      "1+",
    );
    expect(meetingsCard().textContent).toContain(
      en["brief.readings.calendarUnreadableWhy"],
    );
  });

  // A lane that never answered says so, whatever the calendar reports.
  it("keeps the unavailable reading when the meetings lane failed", () => {
    const day = {
      ...readingsDay({}, [], []),
      calendar: "not_connected" as const,
    };
    day.sources_unavailable = [{ source: "meeting", reason: "failed" }];
    drawDay(day);

    expect(meetingsCard().textContent).toContain(
      en["brief.readings.unavailable"],
    );
    expect(meetingsCard().textContent).not.toContain(
      en["brief.readings.calendarNotConnectedWhy"],
    );
  });

  // A failed calendar side read leaves `calendar` unstated: unknown, so the
  // slot keeps the wording it had before the server could say.
  it("keeps today's reading when only the calendar read failed", () => {
    const day = readingsDay({}, [], []);
    day.sources_unavailable = [
      { source: "calendar", reason: "failed", category: "meetings" },
    ];
    drawDay(day);

    expect(meetingsCard().querySelector(".stat-card-value")?.textContent).toBe(
      "0",
    );
    expect(meetingsCard().textContent).toContain(
      en["brief.readings.meetingsBasis"],
    );
  });

  // A measured quiet day says when the next conversation is.
  it("names the next meeting under a connected calendar's zero", () => {
    const startsAt = "2026-09-04T09:30:00Z";
    drawDay({
      ...readingsDay({}, [], []),
      calendar: "connected",
      next_meeting: nextMeeting("a-next", startsAt, "Weber kickoff"),
    });

    expect(meetingsCard().querySelector(".stat-card-value")?.textContent).toBe(
      "0",
    );
    expect(meetingsCard().textContent).toContain(
      en["brief.readings.nextMeeting"]
        .replace("{date}", formatDateAbbrev(startsAt, "en", browserZone()))
        .replace("{subject}", "Weber kickoff"),
    );
  });
});
