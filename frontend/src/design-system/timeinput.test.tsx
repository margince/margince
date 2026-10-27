/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { setDateTimePreferences } from "../format/preferences";
import { type Locale, LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { Field } from "./field";
import { type ClockTime, TimeInput } from "./timeinput";

afterEach(() => {
  cleanup();
  setDateTimePreferences({ dateFormat: "locale", timeFormat: "locale" });
});

function Starts({
  initial = "",
  locale = "en",
  onReport,
}: Readonly<{
  initial?: ClockTime | "";
  locale?: Locale;
  onReport?: (time: string) => void;
}>) {
  const [time, setTime] = useState<ClockTime | "">(initial);
  return (
    <LocaleProvider initial={locale}>
      <Field label="Starts">
        {(control) => (
          <TimeInput
            {...control}
            value={time}
            onChange={(event) => {
              setTime(event.target.value);
              onReport?.(event.target.value);
            }}
          />
        )}
      </Field>
    </LocaleProvider>
  );
}

it("shows the time on the installation's clock, not the browser's", () => {
  setDateTimePreferences({ dateFormat: "locale", timeFormat: "12h" });
  render(<Starts initial="21:05" />);
  const box = screen.getByLabelText("Starts");
  expect(box).toHaveProperty("value", "09:05 pm");
  expect(box.getAttribute("type")).toBe("text");
});

it("reports a typed time once the reader settles it", async () => {
  const onReport = vi.fn();
  const user = userEvent.setup();
  render(<Starts onReport={onReport} />);
  await user.type(screen.getByLabelText("Starts"), "930");
  expect(onReport).not.toHaveBeenCalled();
  await user.keyboard("{Enter}");
  expect(onReport).toHaveBeenCalledWith("09:30");
  expect(screen.getByLabelText("Starts")).toHaveProperty("value", "09:30");
});

it("keeps text it cannot read and says why", async () => {
  const onReport = vi.fn();
  const user = userEvent.setup();
  render(<Starts initial="09:00" onReport={onReport} />);
  const box = screen.getByLabelText("Starts");
  await user.clear(box);
  await user.type(box, "nine{Enter}");
  expect(onReport).not.toHaveBeenCalled();
  expect(
    screen.getByText(en["calendar.invalidTime"].replace("{pattern}", "hh:mm")),
  ).toBeTruthy();
  expect(box.getAttribute("aria-invalid")).toBe("true");
});

it("puts the shown time back on Escape", async () => {
  const user = userEvent.setup();
  render(<Starts initial="09:00" />);
  const box = screen.getByLabelText("Starts");
  await user.type(box, "5{Escape}");
  expect(box).toHaveProperty("value", "09:00");
});

it("reports a cleared time as empty", async () => {
  const onReport = vi.fn();
  const user = userEvent.setup();
  render(<Starts initial="09:00" onReport={onReport} />);
  const box = screen.getByLabelText("Starts");
  await user.clear(box);
  await user.tab();
  expect(onReport).toHaveBeenCalledWith("");
});
