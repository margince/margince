/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { ScheduleMenu } from "./composeschedule";
import { ScheduleDialog } from "./composescheduledialog";

const ZONE = "Europe/Berlin";
// Monday 14 September 2026, 14:30 in Berlin: 08:00, 09:00 and 13:00 are past.
const AFTERNOON = new Date("2026-09-14T12:30:00Z");
// 18:00 in Berlin: every hour today is past.
const EVENING = new Date("2026-09-14T16:00:00Z");

function Dialog({
  now,
  sendAt = "",
  onChoose = () => {},
}: Readonly<{
  now: Date;
  sendAt?: string;
  onChoose?: (next: string) => void;
}>) {
  const [chosen, setChosen] = useState(sendAt);
  return (
    <LocaleProvider initial="en">
      <ScheduleDialog
        open
        onClose={() => {}}
        sendAt={chosen}
        timezone={ZONE}
        onChoose={(next) => {
          setChosen(next);
          onChoose(next);
        }}
        now={now}
      />
    </LocaleProvider>
  );
}

async function openPicker(now: Date, props: { sendAt?: string } = {}) {
  vi.useFakeTimers({ now, toFake: ["Date", "setInterval", "clearInterval"] });
  const user = userEvent.setup();
  const onChoose = vi.fn();
  render(<Dialog now={now} onChoose={onChoose} {...props} />);
  await user.click(
    screen.getByRole("button", { name: "Select date and time" }),
  );
  return { user, onChoose };
}

function hour(name: string): HTMLButtonElement {
  const chip = screen.getByRole("button", { name });
  if (!(chip instanceof HTMLButtonElement))
    throw new Error(`${name} is no button`);
  return chip;
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("choosing a date and time to send", () => {
  it("refuses the days before today and keeps the day chosen", async () => {
    const { user } = await openPicker(AFTERNOON);

    const yesterday = screen.getByRole("button", {
      name: /^Sunday, 13 September 2026, Earliest date: /,
    });
    expect(yesterday.getAttribute("aria-disabled")).toBe("true");
    expect(
      screen
        .getByRole("button", { name: "Monday, 14 September 2026" })
        .getAttribute("aria-disabled"),
    ).toBeNull();

    await user.click(yesterday);
    expect(screen.getByText(/^Sends 14\/09\/2026, 17:00/)).toBeTruthy();
  });

  it("refuses the hours already past today and says why", async () => {
    await openPicker(AFTERNOON);

    const reason = screen.getByText("Times that have passed cannot be chosen.");
    for (const past of ["08:00", "09:00", "13:00"]) {
      expect(hour(past).disabled).toBe(true);
      expect(hour(past).getAttribute("aria-describedby")).toBe(reason.id);
    }
    expect(hour("17:00").disabled).toBe(false);
  });

  it("opens on the next hour left today", async () => {
    await openPicker(AFTERNOON);

    expect(hour("17:00").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText(/^Sends 14\/09\/2026, 17:00/)).toBeTruthy();
  });

  it("opens on tomorrow's first hour once today has none left", async () => {
    await openPicker(EVENING);

    expect(hour("08:00").getAttribute("aria-pressed")).toBe("true");
    expect(hour("08:00").disabled).toBe(false);
    expect(screen.getByText(/^Sends 15\/09\/2026, 08:00/)).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: /^Monday, 14 September 2026, / })
        .getAttribute("aria-disabled"),
    ).toBe("true");
  });

  it("moves to the next hour left when today is picked over a past hour", async () => {
    const { user } = await openPicker(AFTERNOON, {
      sendAt: "2026-09-16T09:00",
    });
    expect(hour("09:00").getAttribute("aria-pressed")).toBe("true");

    await user.click(
      screen.getByRole("button", { name: "Monday, 14 September 2026" }),
    );

    expect(hour("17:00").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText(/^Sends 14\/09\/2026, 17:00/)).toBeTruthy();
  });

  it("refuses to schedule a time that passed while the dialog stayed open", async () => {
    const lateAfternoon = new Date("2026-09-14T14:30:00Z");
    const { user, onChoose } = await openPicker(lateAfternoon);
    expect(hour("17:00").getAttribute("aria-pressed")).toBe("true");

    act(() => {
      vi.advanceTimersByTime(60 * 60_000);
    });

    const confirm = screen.getByRole("button", { name: "Schedule send" });
    expect(confirm).toHaveProperty("disabled", true);
    const line = screen.getByText("Choose a time in the future.");
    expect(confirm.getAttribute("aria-describedby")).toContain(line.id);
    await user.click(confirm);
    expect(onChoose).not.toHaveBeenCalled();
  });

  it("refuses a saved time that is already past", async () => {
    await openPicker(AFTERNOON, { sendAt: "2026-09-14T09:00" });

    expect(
      screen.getByRole("button", { name: "Schedule send" }),
    ).toHaveProperty("disabled", true);
    expect(screen.getByText("Choose a time in the future.")).toBeTruthy();
  });

  it("schedules the hour it opened on", async () => {
    const { user, onChoose } = await openPicker(AFTERNOON);

    await user.click(screen.getByRole("button", { name: "Schedule send" }));
    expect(onChoose).toHaveBeenCalledWith("2026-09-14T17:00");
  });
});

describe("the schedule send menu", () => {
  it("closes once it has opened the dialog", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    render(
      <LocaleProvider initial="en">
        <ScheduleMenu onOpen={onOpen} />
      </LocaleProvider>,
    );

    await user.click(
      screen.getByRole("button", { name: "Other ways to send" }),
    );
    await user.click(screen.getByRole("button", { name: "Schedule send" }));

    expect(onOpen).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("button", { name: "Schedule send" })).toBeNull();
    expect(
      screen
        .getByRole("button", { name: "Other ways to send" })
        .getAttribute("aria-expanded"),
    ).toBe("false");
  });

  it("hands focus back to its trigger when the dialog closes", async () => {
    vi.useFakeTimers({
      now: AFTERNOON,
      toFake: ["Date", "setInterval", "clearInterval"],
    });
    const user = userEvent.setup();
    render(<MenuAndDialog />);

    const trigger = screen.getByRole("button", { name: "Other ways to send" });
    await user.click(trigger);
    await user.click(screen.getByRole("button", { name: "Schedule send" }));
    await screen.findByRole("dialog", { name: "Schedule send" });
    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(trigger);
  });
});

describe("opening the dialog again", () => {
  it("starts from the next hour, not the day picked last time", async () => {
    vi.useFakeTimers({
      now: AFTERNOON,
      toFake: ["Date", "setInterval", "clearInterval"],
    });
    const user = userEvent.setup();
    render(<MenuAndDialog />);
    const pickAgain = async () => {
      await user.click(
        screen.getByRole("button", { name: "Other ways to send" }),
      );
      await user.click(screen.getByRole("button", { name: "Schedule send" }));
      await user.click(
        await screen.findByRole("button", { name: "Select date and time" }),
      );
    };

    await pickAgain();
    await user.click(
      screen.getByRole("button", { name: "Tuesday, 15 September 2026" }),
    );
    expect(screen.getByText(/^Sends 15\/09\/2026, 17:00/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await pickAgain();
    expect(screen.getByText(/^Sends 14\/09\/2026, 17:00/)).toBeTruthy();
  });
});

// The dialog renders before the menu, as the composer renders it.
function MenuAndDialog() {
  const [open, setOpen] = useState(false);
  return (
    <LocaleProvider initial="en">
      <ScheduleDialog
        open={open}
        onClose={() => setOpen(false)}
        sendAt=""
        timezone={ZONE}
        onChoose={() => {}}
        now={AFTERNOON}
      />
      <ScheduleMenu onOpen={() => setOpen(true)} />
    </LocaleProvider>
  );
}
