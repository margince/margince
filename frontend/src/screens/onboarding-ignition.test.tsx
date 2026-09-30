// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { Ignition } from "./onboarding-ignition";

afterEach(cleanup);

describe("the ignition's capability lines", () => {
  it("reach a screen reader as one list of three items inside a polite, atomic live region", () => {
    render(
      <LocaleProvider initial="en">
        <Ignition vendor="Google Gemini" onDone={() => {}} />
      </LocaleProvider>,
    );

    const list = screen.getByRole("list");
    const items = within(list).getAllByRole("listitem");
    expect(items.map((item) => item.textContent)).toEqual([
      `${en["firstRun.ignite.canNow"]}${en["firstRun.ignite.read"]}`,
      `${en["firstRun.ignite.canNow"]}${en["firstRun.ignite.draft"]}`,
      `${en["firstRun.ignite.cannot"]}${en["firstRun.ignite.act"]}`,
    ]);
    expect(list.getAttribute("aria-live")).toBe("polite");
    expect(list.getAttribute("aria-atomic")).toBe("true");
  });
});
