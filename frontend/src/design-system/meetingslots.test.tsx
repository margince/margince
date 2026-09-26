/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { MeetingSlots } from "./meetingslots";

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
