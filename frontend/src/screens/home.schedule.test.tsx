/** @vitest-environment jsdom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meetingRow, readingsDay } from "./home.fixtures";
import { PromisesPanel, SchedulePanel } from "./home.schedule";
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

function draw(node: React.ReactNode) {
  return render(<LocaleProvider initial="en">{node}</LocaleProvider>);
}

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

  it("says the day is clear rather than drawing an empty panel", () => {
    draw(<SchedulePanel day={readingsDay({}, [])} state="ready" />);

    expect(screen.getByText(en["home.schedule.clear"])).toBeTruthy();
  });

  // A read that has not landed is not a clear day. Saying so would send a rep
  // into a morning believing nothing was booked.
  it("says nothing about the day before the read lands", () => {
    draw(<SchedulePanel day={undefined} state="loading" />);

    expect(screen.queryByText(en["home.schedule.clear"])).toBeNull();
  });
});

describe("the promises panel", () => {
  it("lists open tasks and leaves the meetings to the panel above", () => {
    const day = readingsDay({}, [
      meetingRow("m1", true),
      taskRow("t1", "Call Alice back"),
    ]);
    draw(<PromisesPanel day={day} state="ready" />);

    const panel = screen.getByRole("region");
    expect(within(panel).getByText("Call Alice back")).toBeTruthy();
    expect(within(panel).queryByText(/Weber GmbH/)).toBeNull();
  });

  // The panel's heading names two things and the product tracks one. Without
  // this line an empty panel reads as "no promises outstanding", which is the
  // one claim nothing behind it can make.
  it("says promises are untracked on a busy day and on a quiet one", () => {
    const busy = readingsDay({}, [taskRow("t1", "Call Alice back")]);
    const { unmount } = draw(<PromisesPanel day={busy} state="ready" />);
    expect(screen.getByText(en["home.promises.untracked"])).toBeTruthy();
    unmount();

    draw(<PromisesPanel day={readingsDay({}, [])} state="ready" />);
    expect(screen.getByText(en["home.promises.untracked"])).toBeTruthy();
    expect(screen.getByText(en["home.promises.clear"])).toBeTruthy();
  });

  it("does not claim a clear slate before the read lands", () => {
    draw(<PromisesPanel day={undefined} state="loading" />);

    expect(screen.queryByText(en["home.promises.clear"])).toBeNull();
  });
});
