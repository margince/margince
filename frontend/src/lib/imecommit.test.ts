// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { isImeCommit } from "./imecommit";

// The two fields a browser reads an input method's key from, and nothing else.
function enter(nativeEvent: { isComposing: boolean; keyCode: number }) {
  return { nativeEvent: nativeEvent as KeyboardEvent };
}

describe("isImeCommit", () => {
  it("claims an Enter while a composition is open", () => {
    expect(isImeCommit(enter({ isComposing: true, keyCode: 13 }))).toBe(true);
  });

  it("claims the Enter Safari sends after the composition ended", () => {
    expect(isImeCommit(enter({ isComposing: false, keyCode: 229 }))).toBe(true);
  });

  it("leaves a plain Enter to the field", () => {
    expect(isImeCommit(enter({ isComposing: false, keyCode: 13 }))).toBe(false);
  });
});
