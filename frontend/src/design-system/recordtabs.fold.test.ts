// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { foldToFit } from "./recordtabs.fold";

const tabs = ["overview", "deals", "history", "files", "finance"] as const;
const width = () => 100;

describe("foldToFit", () => {
  it("folds nothing when every tab fits", () => {
    expect(foldToFit(tabs, "overview", width, 500, 60)).toEqual([]);
  });

  it("keeps a run from the start and folds the rest", () => {
    expect(foldToFit(tabs, "overview", width, 400, 60)).toEqual([
      "files",
      "finance",
    ]);
  });

  it("keeps the open tab on the row even when it comes last", () => {
    expect(foldToFit(tabs, "finance", width, 400, 60)).toEqual([
      "history",
      "files",
    ]);
  });

  it("stops at the first tab that does not fit, so the order holds", () => {
    const wide = (tab: string) => (tab === "deals" ? 300 : 50);
    expect(foldToFit(tabs, "overview", wide, 300, 60)).toEqual([
      "deals",
      "history",
      "files",
      "finance",
    ]);
  });
});
