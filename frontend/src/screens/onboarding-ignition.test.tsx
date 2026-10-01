// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { OnboardingStage } from "../design-system/onboarding-stage";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { Ignition } from "./onboarding-ignition";

afterEach(cleanup);

function renderOnStage() {
  return render(
    <LocaleProvider initial="en">
      <OnboardingStage flow="Setup" lit title="Choose a model">
        <Ignition vendor="Google Gemini" onDone={() => {}} />
      </OnboardingStage>
    </LocaleProvider>,
  );
}

describe("the ignition", () => {
  // The control that started it is gone, so focus is handed somewhere a screen
  // reader reads the whole scene from, in order.
  it("takes the reader to the stage title the moment it starts", () => {
    renderOnStage();

    expect(document.activeElement).toBe(
      screen.getByRole("heading", { name: "Choose a model" }),
    );
  });

  // A live region that mounts already full is announced by no screen reader
  // reliably, and one that is never changed is never announced at all.
  it("draws its capability lines as one plain list of three, not a live region", () => {
    renderOnStage();

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
