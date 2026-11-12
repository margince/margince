// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { foldToFit } from "./recordtabs.fold";

const tabs = ["overview", "deals", "history", "files", "finance"] as const;
const width = () => 100;
const space = (room: number, inset = 0) => ({ room, more: 60, inset });

describe("foldToFit", () => {
  it("folds nothing when every tab fits", () => {
    expect(foldToFit(tabs, "overview", width, space(500))).toEqual([]);
  });

  it("keeps a run from the start and folds the rest", () => {
    expect(foldToFit(tabs, "overview", width, space(400))).toEqual([
      "files",
      "finance",
    ]);
  });

  it("keeps the open tab on the row even when it comes last", () => {
    expect(foldToFit(tabs, "finance", width, space(400))).toEqual([
      "history",
      "files",
    ]);
  });

  it("stops at the first tab that does not fit, so the order holds", () => {
    const wide = (tab: string) => (tab === "deals" ? 300 : 50);
    expect(foldToFit(tabs, "overview", wide, space(300))).toEqual([
      "deals",
      "history",
      "files",
      "finance",
    ]);
  });

  it("gives the first tab's dropped padding back to the row", () => {
    const three = ["overview", "deals", "files"] as const;
    const stored = { overview: 92, deals: 100, files: 200 };
    const widthOf = (tab: keyof typeof stored) => stored[tab];
    expect(foldToFit(three, "overview", widthOf, space(380, 12))).toEqual([]);
    expect(foldToFit(three, "overview", widthOf, space(379, 12))).toEqual([
      "files",
    ]);
  });
});
