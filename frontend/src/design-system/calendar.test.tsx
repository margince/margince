/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Calendar, isoDay, monthGrid } from "./calendar";

afterEach(cleanup);

const AUGUST_2026 = new Date(2026, 7, 1);
const TODAY = new Date(2026, 7, 24);

// Six weeks whatever the month: a grid that shrank to five would move every
// control under it as the reader pages.
it("draws six weeks for a month that opens on a Saturday and one that does not", () => {
  expect(monthGrid(new Date(2026, 7, 1))).toHaveLength(42);
  expect(monthGrid(new Date(2026, 1, 1))).toHaveLength(42);
});

it("opens the grid on the Sunday before the first of the month", () => {
  // 1 August 2026 is a Saturday, so the grid opens on 26 July.
  expect(isoDay(monthGrid(AUGUST_2026)[0])).toBe("2026-07-26");
});

it("hands back the day the reader pressed", async () => {
  const chose = vi.fn();
  render(
    <Calendar
      month={AUGUST_2026}
      onMonthChange={() => {}}
      selected=""
      onSelect={chose}
      today={TODAY}
      locale="en"
    />,
  );

  await userEvent.click(
    screen.getByRole("button", { name: "Tuesday, 25 August 2026" }),
  );

  expect(chose).toHaveBeenCalledWith("2026-08-25");
});

// The month is the caller's, so paging asks rather than moves. Stepping back
// from a 31-day month must not land in the month it started in.
it("steps a month at a time from a date no shorter month holds", async () => {
  const paged = vi.fn();
  render(
    <Calendar
      month={new Date(2026, 6, 31)}
      onMonthChange={paged}
      selected=""
      onSelect={() => {}}
      today={TODAY}
      locale="en"
    />,
  );

  await userEvent.click(screen.getByRole("button", { name: "Previous month" }));

  expect(isoDay(paged.mock.calls[0][0])).toBe("2026-06-01");
});

it("marks the chosen day and nothing else", () => {
  render(
    <Calendar
      month={AUGUST_2026}
      onMonthChange={() => {}}
      selected="2026-08-25"
      onSelect={() => {}}
      today={TODAY}
      locale="en"
    />,
  );

  const pressed = screen
    .getAllByRole("button")
    .filter((button) => button.getAttribute("aria-pressed") === "true");
  expect(pressed).toHaveLength(1);
  expect(pressed[0].textContent).toBe("25");
});
