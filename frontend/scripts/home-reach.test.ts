// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { homeReach } from "./home-reach.ts";

// The real tree, so a scope that stops seeing Home's imports fails here.
describe("which changes can reach Home's first load", () => {
  it("reaches the Home screen itself", () => {
    expect(homeReach(["frontend/src/features/brief/brief.tsx"])).not.toBeNull();
  });

  it("reaches a stylesheet the Home screen imports", () => {
    expect(homeReach(["frontend/src/features/brief/brief.css"])).not.toBeNull();
  });

  it("reaches the shell, which loads before any screen", () => {
    expect(homeReach(["frontend/src/App.tsx"])).not.toBeNull();
  });

  it("does not reach a screen Home never loads", () => {
    expect(
      homeReach(["frontend/src/features/deals/dealroompage.tsx"]),
    ).toBeNull();
  });

  it("does not reach a test or a file outside the frontend", () => {
    expect(
      homeReach([
        "frontend/src/features/brief/brief.dials.test.tsx",
        "docs/reference/make-targets.md",
      ]),
    ).toBeNull();
  });
});
