// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// The suite unmounts a case's tree without the case asking.
//
// React Testing Library's own auto-cleanup does not arm here — it registers on
// a global `afterEach`, and this suite runs on vitest's `globals: false` — so
// the hook is `vitest.setup.ts`'s to register. Nothing else would notice if it
// stopped: 119 of the suites that render never call `cleanup` themselves, and
// what a live tree costs is not a failing assertion but React's scheduler
// waking after jsdom is gone, in whichever file happens to run last.
//
// So the claim is checked the only way it can be — across two cases, the second
// asserting on what the first left behind. The order dependence IS the subject.

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

const TREE_TEXT = "a tree this case never unmounts";

describe("a rendered tree does not outlive its case", () => {
  it("renders one and leaves it mounted", () => {
    render(<p>{TREE_TEXT}</p>);
    expect(screen.getByText(TREE_TEXT)).toBeTruthy();
  });

  it("finds the previous case's tree already gone", () => {
    expect(document.body.innerHTML).toBe("");
  });
});
