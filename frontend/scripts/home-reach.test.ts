// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { spawnSync } from "node:child_process";
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

  it("reaches a stylesheet the shell's stylesheet imports", () => {
    expect(homeReach(["frontend/src/design-system/tokens.css"])).not.toBeNull();
  });

  it("reaches what the browser check imports", () => {
    expect(homeReach(["frontend/src/testing/quantile.ts"])).not.toBeNull();
    expect(homeReach(["frontend/e2e/homejourney.ts"])).not.toBeNull();
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

// The CI job's first step, fed the changed paths directly.
describe("when the home-budget job runs", () => {
  const scope = (paths: string[]) =>
    spawnSync("bash", ["../scripts/home-budget-scope.sh", "-"], {
      input: paths.join("\n"),
      encoding: "utf8",
    }).stdout;

  it.each([
    ["backend/internal/compose/attention/homeanswer.go"],
    [
      "backend/migrations/core/1793594630_home_reads_are_cached_behind_an_access_fence.up.sql",
    ],
    ["frontend/e2e/quantile.ts", "frontend/src/testing/quantile.ts"],
    ["frontend/playwright.config.ts"],
    ["backend/Makefile"],
    ["frontend/src/design-system/tokens.css"],
  ])("runs for %s", (...paths) => {
    expect(scope(paths)).toContain("run=true");
  });

  it.each([
    ["frontend/src/features/deals/dealroompage.tsx"],
    ["docs/reference/make-targets.md"],
    [".github/actions/install-claude-cli/action.yml"],
  ])("skips for %s", (...paths) => {
    expect(scope(paths)).toContain("run=false");
  });
});
