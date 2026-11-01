/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { MeetingSlots, MeetingWeek } from "./meetingslots";

afterEach(cleanup);
it("returns only the chosen interval, keeping display text out of booking requests", async () => {
  const user = userEvent.setup();
  const onSelect = vi.fn();
  const start = "2026-10-05T09:00:00Z";
  const end = "2026-10-05T09:30:00Z";
  render(
    <MeetingSlots
      slots={[{ start, end, label: "Monday morning" }]}
      onSelect={onSelect}
      empty="No times"
    />,
  );
  await user.click(screen.getByRole("button", { name: "Monday morning" }));
  expect(onSelect).toHaveBeenCalledExactlyOnceWith({ start, end });
});

it("groups a week by day, names each slot in full and says when a day is busy", async () => {
  const user = userEvent.setup();
  const onSelect = vi.fn();
  const start = "2026-10-05T07:00:00Z";
  const end = "2026-10-05T07:30:00Z";
  render(
    <MeetingWeek
      days={[
        {
          key: "2026-10-05",
          weekday: "Mon",
          date: "5 Oct",
          slots: [
            { start, end, label: "Monday 5 October, 09:00", time: "09:00" },
          ],
        },
        { key: "2026-10-06", weekday: "Tue", date: "6 Oct", slots: [] },
      ]}
      selected={[start]}
      emptyDay="No free time"
      onSelect={onSelect}
    />,
  );
  const monday = screen.getByRole("group", { name: "Mon 5 Oct" });
  const slot = screen.getByRole("button", { name: "Monday 5 October, 09:00" });
  expect(monday.contains(slot)).toBe(true);
  expect(slot.getAttribute("aria-pressed")).toBe("true");
  expect(
    screen.getByRole("group", { name: "Tue 6 Oct" }).textContent,
  ).toContain("No free time");
  await user.click(slot);
  expect(onSelect).toHaveBeenCalledExactlyOnceWith({ start, end });
});

it("draws a preview week's times without offering them to be pressed", () => {
  render(
    <MeetingWeek
      days={[
        {
          key: "2026-10-05",
          weekday: "Mon",
          date: "5 Oct",
          slots: [
            {
              start: "2026-10-05T07:00:00Z",
              end: "2026-10-05T07:30:00Z",
              label: "Monday 5 October, 09:00",
              time: "09:00",
            },
          ],
        },
      ]}
      selected={[]}
      emptyDay="No free time"
    />,
  );
  const monday = screen.getByRole("group", { name: "Mon 5 Oct" });
  expect(monday.textContent).toContain("Monday 5 October, 09:00");
  expect(screen.queryByRole("button")).toBeNull();
});

it("will not press a blocked time, and keeps a chosen one pressable", () => {
  const chosen = "2026-10-05T07:00:00Z";
  const blocked = "2026-10-05T07:15:00Z";
  const slot = (start: string, time: string) => ({
    start,
    end: new Date(Date.parse(start) + 30 * 60000).toISOString(),
    label: `Monday 5 October, ${time}`,
    time,
  });
  render(
    <MeetingWeek
      days={[
        {
          key: "2026-10-05",
          weekday: "Mon",
          date: "5 Oct",
          slots: [slot(chosen, "09:00"), slot(blocked, "09:15")],
        },
      ]}
      selected={[chosen]}
      blocked={[chosen, blocked]}
      emptyDay="No free time"
      onSelect={() => undefined}
    />,
  );
  expect(
    screen.getByRole("button", { name: "Monday 5 October, 09:15" }),
  ).toBeDisabled();
  expect(
    screen.getByRole("button", { name: "Monday 5 October, 09:00" }),
  ).toBeEnabled();
});
