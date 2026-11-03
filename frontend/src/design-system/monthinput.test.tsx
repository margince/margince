/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider } from "../i18n";
import { Field } from "./field";
import { MonthInput, type YearMonth } from "./monthinput";
import { pickOption } from "./select-testing";

afterEach(cleanup);

function Period({
  locale = "en",
  initial = "",
  onReport,
}: Readonly<{
  locale?: Locale;
  initial?: YearMonth | "";
  onReport?: (month: string) => void;
}>) {
  const [month, setMonth] = useState<YearMonth | "">(initial);
  return (
    <LocaleProvider initial={locale}>
      <Field label="Month starting">
        {(control) => (
          <MonthInput
            {...control}
            value={month}
            onChange={(next) => {
              setMonth(next);
              onReport?.(next);
            }}
          />
        )}
      </Field>
    </LocaleProvider>
  );
}

it("names the month in the UI language", () => {
  render(<Period locale="de" initial="2026-03" />);
  expect(screen.getByLabelText("Month starting").textContent).toContain("März");
  expect(screen.getByDisplayValue("2026")).toBeTruthy();
});

it("reports the month once a month and a year are set", async () => {
  const onReport = vi.fn();
  const user = userEvent.setup();
  render(<Period onReport={onReport} />);
  await user.type(screen.getByLabelText("Year"), "2025");
  expect(onReport).not.toHaveBeenCalled();
  await pickOption(user, screen.getByLabelText("Month starting"), "November");
  expect(onReport).toHaveBeenLastCalledWith("2025-11");
  await user.clear(screen.getByLabelText("Year"));
  await user.type(screen.getByLabelText("Year"), "2024");
  expect(onReport).toHaveBeenLastCalledWith("2024-11");
});

it("reports no month once the month is cleared", async () => {
  const onReport = vi.fn();
  const user = userEvent.setup();
  render(<Period initial="2025-11" onReport={onReport} />);
  await pickOption(user, screen.getByLabelText("Month starting"), "Any month");
  expect(onReport).toHaveBeenLastCalledWith("");
});
