/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { isImeCommit } from "./imecommit";

function enter(init: KeyboardEventInit) {
  return {
    nativeEvent: new KeyboardEvent("keydown", { key: "Enter", ...init }),
  };
}

describe("isImeCommit", () => {
  it("claims an Enter while a composition is open", () => {
    expect(isImeCommit(enter({ isComposing: true, keyCode: 13 }))).toBe(true);
  });

  it("claims the Enter Safari sends after the composition ended", () => {
    expect(isImeCommit(enter({ keyCode: 229 }))).toBe(true);
  });

  it("leaves a plain Enter to the field", () => {
    expect(isImeCommit(enter({ keyCode: 13 }))).toBe(false);
  });
});
