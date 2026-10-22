/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { PROJECT_PHASES } from "../../format/projectphase";
import { LocaleProvider } from "../../i18n";
import { PhaseBadge } from "./projects";

afterEach(cleanup);

function badgeFor(phase: (typeof PROJECT_PHASES)[number]): Element {
  const { container } = render(
    <LocaleProvider>
      <PhaseBadge phase={phase} />
    </LocaleProvider>,
  );
  const badge = container.querySelector(".badge");
  if (badge === null) throw new Error(`PhaseBadge drew no badge for ${phase}`);
  return badge;
}

it("draws a phase under way in the tone for work still running", () => {
  for (const phase of PROJECT_PHASES.filter((one) => one !== "closed")) {
    expect(badgeFor(phase).classList.contains("badge-info"), phase).toBe(true);
    cleanup();
  }
});

it("draws a closed project in the neutral tone", () => {
  expect([...badgeFor("closed").classList]).toEqual(["badge"]);
});

// `success` is a good end, and the phase ladder has no phase that means one.
it("draws no phase in the success tone", () => {
  for (const phase of PROJECT_PHASES) {
    expect(badgeFor(phase).classList.contains("badge-success"), phase).toBe(
      false,
    );
    cleanup();
  }
});
