// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  LEAD_LIST_KEY,
  leadKey,
  leadManualSignalsKey,
  leadPromotePreviewKey,
  leadScoreKey,
  leadWriteKeys,
} from "./leadkeys";

describe("the keys themselves", () => {
  it("files the detail page as a SIBLING of the list, which is the whole problem", () => {
    // Stated as an assertion rather than a comment: if these two ever share a
    // prefix, the module is solving a problem that no longer exists and the
    // next reader deserves to be told by a failure, not by archaeology.
    expect(leadKey("l-1")).toEqual(["lead", "l-1"]);
    expect(LEAD_LIST_KEY).toEqual(["leads"]);
    expect(leadKey("l-1")[0]).not.toEqual(LEAD_LIST_KEY[0]);
  });

  it("reaches the detail page's children by prefix", () => {
    for (const child of [leadScoreKey("l-1"), leadManualSignalsKey("l-1")]) {
      expect(child.slice(0, 2)).toEqual(leadKey("l-1"));
    }
  });

  it("carries the list, the detail page, the history and its lists on every write", () => {
    expect(leadWriteKeys("l-1")).toEqual([
      ["leads"],
      ["lead", "l-1"],
      ["record-history", "lead", "l-1"],
      ["lists", "record", "lead", "l-1"],
    ]);
  });

  it("leaves the promote preview out, because invalidating it would change nothing", () => {
    // staleTime 0 + enabled-on-open: it refetches every time the dialog opens,
    // and an inactive query is refetched on its next mount either way.
    expect(leadWriteKeys("l-1")).not.toContainEqual(
      leadPromotePreviewKey("l-1"),
    );
  });
});
