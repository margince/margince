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
  // Not a live region: one that mounts already full and never changes is
  // announced by no screen reader reliably. The reader reaches the lines by
  // focus, which the screen hosting the ignition hands to the stage title.
  it("are one plain list of three items", () => {
    render(
      <LocaleProvider initial="en">
        <Ignition vendor="Google Gemini" onDone={async () => {}} />
      </LocaleProvider>,
    );

    const list = screen.getByRole("list");
    const items = within(list).getAllByRole("listitem");
    expect(items.map((item) => item.textContent)).toEqual([
      `${en["firstRun.ignite.canNow"]}${en["firstRun.ignite.read"]}`,
      `${en["firstRun.ignite.canNow"]}${en["firstRun.ignite.draft"]}`,
      `${en["firstRun.ignite.cannot"]}${en["firstRun.ignite.act"]}`,
    ]);
    expect(list.getAttribute("aria-live")).toBeNull();
  });
});
