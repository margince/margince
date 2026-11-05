/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { render } from "../../testing/brief.testkit";
import { ActivityTotal, WeekActivity } from "./brief.weekly.activity";

afterEach(cleanup);

const quiet = { emails_sent: 0, calls_logged: 0, meetings_held: 0 };
const week = [
  { day: "2026-06-01", ...quiet, emails_sent: 4 },
  { day: "2026-06-02", emails_sent: 6, calls_logged: 2, meetings_held: 1 },
  { day: "2026-06-03", ...quiet },
  { day: "2026-06-04", ...quiet, calls_logged: 1 },
  { day: "2026-06-05", ...quiet },
  { day: "2026-06-06", ...quiet },
  { day: "2026-06-07", ...quiet, emails_sent: 1 },
];

function keyTotal(label: string): string | null | undefined {
  return screen.getByText(label).closest("li")?.querySelector("b")
    ?.textContent;
}

describe("the week's activity chart", () => {
  it("totals each kind over the whole week in the key", () => {
    render(<WeekActivity days={week} />);
    expect(keyTotal(en["brief.weekly.activity.emails"])).toBe("11");
    expect(keyTotal(en["brief.weekly.activity.calls"])).toBe("3");
    expect(keyTotal(en["brief.weekly.activity.meetings"])).toBe("1");
  });

  it("draws the working days and only a weekend day with work on it", () => {
    render(<WeekActivity days={week} />);
    const days = screen.getAllByRole("listitem", { name: /2026|June/ });
    expect(days).toHaveLength(6);
    expect(days[5]?.getAttribute("aria-label")).toContain("Sunday");
  });

  it("names the busiest day, and none for a week with no work", () => {
    render(<WeekActivity days={week} />);
    expect(screen.getByText(/Your busiest day was Tuesday/)).toBeTruthy();
    cleanup();
    render(<WeekActivity days={week.map((day) => ({ ...day, ...quiet }))} />);
    expect(screen.queryByText(/busiest day/)).toBeNull();
  });

  it("counts what the rep did in the card's head", () => {
    render(<ActivityTotal days={week} />);
    expect(screen.getByText("15 things you did")).toBeTruthy();
  });
});
