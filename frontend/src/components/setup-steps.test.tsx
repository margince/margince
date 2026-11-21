// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import type { SetupStep } from "../lib/setup-checklist";
import { SetupSteps } from "./setup-steps";

function calendarStep(fields: Pick<SetupStep, "page" | "blocked" | "reason">) {
  const step: SetupStep = {
    id: "calendar",
    state: "current",
    account: null,
    ...fields,
  };
  return step;
}

afterEach(cleanup);

it("sends the reader who sets up the company app to its page", async () => {
  globalThis.location.hash = "#/home";
  render(
    <LocaleProvider initial="en">
      <SetupSteps
        steps={[
          calendarStep({
            page: "authentication",
            blocked: false,
            reason: "registerApp",
          }),
        ]}
      />
    </LocaleProvider>,
  );

  expect(
    screen.getByText("Needs your company’s Google or Microsoft app."),
  ).toBeTruthy();
  await userEvent.click(screen.getByRole("button", { name: "Set up app" }));
  expect(globalThis.location.hash).toBe("#/settings/authentication/apps");
});

it("offers no action to a reader who waits on an administrator", () => {
  render(
    <LocaleProvider initial="en">
      <SetupSteps
        steps={[
          calendarStep({
            page: "connections",
            blocked: true,
            reason: "waitsAdmin",
          }),
        ]}
      />
    </LocaleProvider>,
  );

  expect(screen.getByText("Waits on an administrator.")).toBeTruthy();
  expect(screen.queryByRole("button")).toBeNull();
});
