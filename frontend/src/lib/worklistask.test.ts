// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { askedReason, askKey } from "./worklistask";
import type { WorklistItem } from "./worklisttypes";

function briefItem(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "brief-1",
    source: "brief_item",
    category: "deals_at_risk",
    level: 2,
    consequence: "none",
    because: [],
    actions: ["act", "set_aside", "dismiss"],
    ...over,
  };
}

describe("a brief item's ask", () => {
  it("leads with the most actionable reason the server sent", () => {
    const item = briefItem({
      kind: "stalled",
      because: [{ kind: "no_next_step" }, { kind: "closing_soon" }],
    });
    expect(askKey(item)).toBe("worklist.ask.brief.closing_soon");
    expect(askedReason(item)).toBe("closing_soon");
  });

  it("says why the night picked the deal when no reason names it", () => {
    const item = briefItem({
      kind: "moved",
      because: [{ kind: "quiet_days" }],
    });
    expect(askKey(item)).toBe("worklist.ask.brief.signal.moved");
    expect(askedReason(item)).toBeUndefined();
  });

  it("keeps a sentence for an entry stored before signals existed", () => {
    expect(askKey(briefItem())).toBe("worklist.ask.brief_item");
  });

  it("leaves every other source on its own sentence", () => {
    const task = briefItem({
      source: "task",
      because: [{ kind: "closing_soon" }],
    });
    expect(askKey(task)).toBe("worklist.ask.task");
    expect(askedReason(task)).toBeUndefined();
  });
});
